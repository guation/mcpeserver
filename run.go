package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"time"
	"strconv"
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

var autoRestart int = 0
var autoBackup  string = "false"

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

func execCommand(command string)string{
	cmd := exec.Command("/bin/bash", "-c", command)
	output, err := cmd.Output()
	if err != nil {
		fmt.Printf("启动器出现异常，这将会影响启动器部分功能: ", err.Error(),"\n")
		return "false"
	}else{
		return string(output)
	}
}

func run(datapath, profile string, prompt *fasttemplate.Template) bool {
	GT := make(chan string,8)
	_, err1 := os.Stat("./AutoRestart.GT")
	if err1 == nil {
		a_int, err := strconv.Atoi(execCommand(`tail -n 1 ./AutoRestart.GT`))
		if err != nil {
			fmt.Println("读取AutoRestart.GT出现错误，请检查内容是否符合要求。",err,"\n")
		}else{
			autoRestart = a_int
		}
	}
	_, err := os.Stat("./AutoBackup.GT")
	if err == nil {
		autoBackup = execCommand(`tail -n 1 ./AutoBackup.GT`)
	}
	if autoBackup == "true" {
		go func() {
			if execCommand(`zip -r ./worlds.zip ./worlds/*`) == "false" {
				fmt.Println("自动备份出现错误。\n")
			}else{
				fmt.Println("自动备份执行成功。\n")
			}
		}()
	}else{
		fmt.Println("自动备份未开启。\n")
	}
	
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
	if autoRestart == 0 {
		fmt.Println("自动重启未开启。\n")
	}else if autoRestart > 0 {
		fmt.Println("自动重启参数设置为:",autoRestart,"\n")
		autoRestartTime := time.Second * time.Duration(autoRestart)
		autoRestartMsgTime := time.Second * time.Duration(autoRestart - 15)
		if autoRestart < 900 {
			timer :=time.NewTimer(autoRestartTime) 
			go func(){
				for {
				<-timer.C
				if strings.Contains(execCommand(`tail -n 3 default.log`),"HYB") {
					execFn("console", "挂神自动崩服重启！")
					GT <- "true"
				}else{
					fmt.Println("自动检测完成，未发现崩服。",autoRestart,"秒之后将再次检测。\n")
					timer.Reset(autoRestartTime)
				}}
			}()
		}else{
			timer1 := time.NewTimer(autoRestartMsgTime) 
			timer2 := time.NewTimer(autoRestartTime) 
			go func() { 
				<-timer1.C 
				execFn("console", "say 15秒后服务器将自动重启！")
			}() 
			go func() {
				<-timer2.C 
				execFn("console", "say 服务器正在自动重启！")
				GT <- "true"
			}()
		}
	}else{
		fmt.Println("自动重启参数不合格，0不自动重启，1-899自动崩服检测，900+自动重启。\n")
	}
	
	go func() {
		for {
			line,err:=rl.Readline()
			if err == readline.ErrInterrupt {
				if len(line) == 0 {
					GT <- "break"
					break
				} else {
					continue
				}
			} else if err == io.EOF {
				GT <- "break"
				break
			}
			line = strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(line, ":restart"):
				GT <- "true"
			case strings.HasPrefix(line, ":quit"):
				GT <- "status"
			default:
				cache++
				execFn("console", line)
			}
		}
	}()
	for {
		select{
		case G := <-GT:
			if G == "true" {
				return true
			}else if G == "status" {
				return status
			}else if G == "break" {
				return false
			}
		}
	}
}
