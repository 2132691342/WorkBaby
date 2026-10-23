// PI 自带的包管理命令：装 / 卸 / 升级插件包走 PI 自己的 CLI，WorkBaby 不重写一遍解析规则。
package env

import (
	"bufio"
	"io"
	"os/exec"
	"strings"
	"sync"

	"WorkBaby/backend/pkg"
)

// RunCLI 跑一条 PI CLI 命令（`pi install` / `pi remove` / `pi update` / …），输出逐行回调。
// 它复用与子进程完全相同的那份环境：内置 Node、agent 目录、npm 前缀。
func (e *Env) RunCLI(args []string, onLine func(string)) error {
	return e.runCLI(args, e.Env(), onLine)
}

// runCLI 是带环境覆盖的 CLI 调用；升级 PI 时要多带一层 npm 前缀，见 UpdatePi。
func (e *Env) runCLI(args []string, env []string, onLine func(string)) error {
	exe, extra, err := e.PiCommand()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, append(append([]string{}, extra...), args...)...)
	cmd.Env = env
	cmd.Dir = e.AgentDir()
	hideWindow(cmd)

	stdout, perr := cmd.StdoutPipe()
	if perr != nil {
		return pkg.Wrap("9719", "无法读取插件命令输出", perr)
	}
	stderr, perr := cmd.StderrPipe()
	if perr != nil {
		return pkg.Wrap("9719", "无法读取插件命令输出", perr)
	}
	if perr := cmd.Start(); perr != nil {
		return pkg.Wrap("9719", "插件命令起不来", perr)
	}
	pumpBoth(stdout, stderr, onLine)
	if err := cmd.Wait(); err != nil {
		return pkg.Wrap("9719", "插件命令失败", err)
	}
	return nil
}

// RunCLIOutput 跑一条 PI CLI 命令并返回完整 stdout（用于 `pi list --json` 这类结构化输出）。
func (e *Env) RunCLIOutput(args ...string) (string, error) {
	exe, extra, err := e.PiCommand()
	if err != nil {
		return "", err
	}
	argv := append(append([]string{}, extra...), args...)
	cmd := exec.Command(exe, argv...)
	cmd.Env = e.Env()
	cmd.Dir = e.AgentDir()
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		// 退出码非零时 stdout 仍可能有内容（例如某个 MCP 服务连不上），一并交给调用方判断。
		if len(out) > 0 {
			return string(out), nil
		}
		return "", pkg.Wrap("9719", "插件命令失败", err)
	}
	return string(out), nil
}

// pumpBoth 并行排空 stdout 与 stderr：串行读会让写满 stderr 的 npm 卡住，
// 而 stdout 要等进程退出才 EOF，两边互等就是「更新一直没动静」。
func pumpBoth(stdout, stderr io.Reader, onLine func(string)) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); pumpLines(stdout, onLine) }()
	go func() { defer wg.Done(); pumpLines(stderr, onLine) }()
	wg.Wait()
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

// lineTail 保留最后 N 行：命令失败时这几行就是给人看的唯一线索。
type lineTail struct {
	lines []string
	max   int
}

func newLineTail(max int) *lineTail { return &lineTail{lines: make([]string, 0, max), max: max} }

func (t *lineTail) add(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	t.lines = append(t.lines, line)
	if len(t.lines) > t.max {
		t.lines = t.lines[len(t.lines)-t.max:]
	}
}

// last 取最后一个非空行作为摘要；空时给一句兜底。
func (t *lineTail) last() string {
	for i := len(t.lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(t.lines[i]); s != "" {
			return clipLine(s, 300)
		}
	}
	return "命令没有输出，检查网络与 npm 注册表"
}

// clipLine 截断过长的一行，保留可读前缀。
func clipLine(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
