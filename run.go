package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"strings"

	"github.com/chzyer/readline"
	"github.com/kr/pty"
	"github.com/valyala/fasttemplate"
)

var replacer = strings.NewReplacer(
	"§0", "\033[30m", // black
	"§1", "\033[34m", // blue
	"§2", "\033[32m", // green
	"§3", "\033[36m", // aqua
	"§4", "\033[31m", // red
	"§5", "\033[35m", // purple
	"§6", "\033[33m", // gold
	"§7", "\033[37m", // gray
	"§8", "\033[90m", // dark gray
	"§9", "\033[94m", // light blue
	"§a", "\033[92m", // light green
	"§b", "\033[96m", // light aque
	"§c", "\033[91m", // light red
	"§d", "\033[95m", // light purple
	"§e", "\033[93m", // light yellow
	"§f", "\033[97m", // light white
	"§k", "\033[5m", // Obfuscated
	"§l", "\033[1m", // Bold
	"§m", "\033[2m", // Strikethrough
	"§n", "\033[4m", // Underline
	"§o", "\033[3m", // Italic
	"§r", "\033[0m", // Reset
	"[", "\033[1m[",
	"]", "]\033[22m",
	"(", "(\033[4m",
	")", "\033[24m)",
	"<", "\033[1m<",
	">", ">\033[22m",
)

func packOutput(input io.Reader, output func(string)) {
	reader := bufio.NewReader(input)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		output(strings.TrimRight(line, "\n"))
	}
}

func runImpl(datapath string, done chan bool) (*os.File, func()) {
	cmd := exec.Command("./bin/bedrockserver")
	cmd.Env = append(os.Environ(), "LD_LIBRARY_PATH=.")
	cmd.Dir, _ = os.Getwd()
	f, err := pty.Start(cmd)
	if err != nil {
		panic(err)
	}
	status := true
	selfLock := make(chan struct{}, 1)
	go func() {
		cmd.Wait()
		selfLock <- struct{}{}
		done <- status
	}()
	return f, func() {
		status = false
		cmd.Process.Signal(os.Interrupt)
		<-selfLock
	}
}

func run(datapath, profile string, prompt *fasttemplate.Template) bool {
	log, err := os.OpenFile(profile+".log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		printWarn("Log File load failed")
		return false
	}
	defer log.Close()
	proc := make(chan bool, 1)
	f, stop := runImpl(datapath, proc)
	defer f.Close()
	defer stop()
	username := "nobody"
	hostname := "bedrockserver"
	{
		u, err := user.Current()
		if err == nil {
			username = u.Username
		}
		hn, err := os.Hostname()
		if err == nil {
			hostname = hn
		}
	}
	rl, _ := readline.NewEx(&readline.Config{
		Prompt: prompt.ExecuteString(map[string]interface{}{
			"username": username,
			"hostname": hostname,
			"esc":      "\033",
		}),
		HistoryFile:     ".readline-history",
		AutoComplete:    completer,
		InterruptPrompt: "^C",
		EOFPrompt:       "quit",

		HistorySearchFold: true,
		FuncFilterInputRune: func(r rune) (rune, bool) {
			if r == readline.CharCtrlZ {
				return r, false
			}
			return r, true
		},
	})
	defer rl.Close()
	lw := io.MultiWriter(rl.Stdout(), log)
	status := false
	execFn := func(src, cmd string) {
		fmt.Fprintf(f, "%s\n", cmd)
		fmt.Fprintf(log, "%s>%s\n", src, cmd)
		switch {
		case strings.HasPrefix(cmd, ":restart"):
			status = true
			rl.Close()
		case strings.HasPrefix(cmd, ":quit"):
			status = true
			rl.Close()
		}
	}
	cache := 0
	go packOutput(f, func(text string) {
		if strings.HasPrefix(text, "\x07") {
			execFn("mod", text[1:len(text)-1])
			cache++
		} else {
			if cache == 0 {
				fmt.Fprintf(lw, "\033[0m%s\033[0m\n", replacer.Replace(text))
			} else {
				cache--
			}
		}
	})
	for {
		line, err := rl.Readline()
		AutoRestart := false
		if err == readline.ErrInterrupt {
			if len(line) == 0 {
				command := `tail -n 1 default.log`
				cmd := exec.Command("/bin/bash", "-c", command)
				output, err := cmd.Output()
				if err != nil {
					fmt.Printf("挂神自动重启检测失败:%s", err.Error())
					break
				}else{
					GT := string(output)
					if strings.ContainsAny(GT,"F&HYB") {
						cache++
						execFn("console", "挂神自动崩服重启！")
						AutoRestart = true
						continue
					}
				}
				
			} else {
				continue
			}
		} else if err == io.EOF {
			break
		}
		line = strings.TrimSpace(line)
		if AutoRestart {
			line = ":restart"
		}
		switch {
		case strings.HasPrefix(line, ":restart"):
			return true
		case strings.HasPrefix(line, ":quit"):
			return status
		default:
			cache++
			execFn("console", line)
		}
	}
	return false
}
