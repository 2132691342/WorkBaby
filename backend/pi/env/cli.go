// PI 自带的包管理命令：装 / 卸插件包走 PI 自己的 CLI，WorkBaby 不重写一遍解析规则。
package env

import (
	"bufio"
	"io"
	"os/exec"

	"WorkBaby/backend/pkg"
)

// RunCLI 跑一条 PI CLI 命令（`pi install` / `pi remove` / …），输出逐行回调。
// 它复用与子进程完全相同的那份环境：内置 Node、agent 目录、npm 前缀。
func (e *Env) RunCLI(args []string, onLine func(string)) error {
	exe, _ := e.PiExe()
	if exe == "" {
		return pkg.New("9709", "还没有安装助手引擎（PI）", "先去环境向导装它")
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = e.Env()
	cmd.Dir = e.AgentDir()
	hideWindow(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return pkg.Wrap("9719", "无法读取插件命令输出", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return pkg.Wrap("9719", "无法读取插件命令输出", err)
	}
	if err := cmd.Start(); err != nil {
		return pkg.Wrap("9719", "插件命令起不来", err)
	}
	pumpLines(stdout, onLine)
	pumpLines(stderr, onLine)
	if err := cmd.Wait(); err != nil {
		return pkg.Wrap("9719", "插件命令失败", err)
	}
	return nil
}

// pumpLines 逐行读管道并回调；没有回调也要排空，否则子进程写满管道会卡住。
func pumpLines(r io.Reader, onLine func(string)) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if onLine != nil {
			onLine(scanner.Text())
		}
	}
}
