package pkg

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	logMu   sync.Mutex
	logDir  string
	logFile *os.File
)

// SetLogDir 指定日志目录；装配时调一次，之后按天滚动。
func SetLogDir(dir string) {
	logMu.Lock()
	defer logMu.Unlock()
	logDir = dir
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
}

// Info 记一条信息日志。
func Info(format string, args ...any) { write("INFO", format, args...) }

// Warn 记一条告警日志：不影响业务继续，但值得回头看一眼。
func Warn(format string, args ...any) { write("WARN", format, args...) }

// Error 记一条错误日志。
func Error(format string, args ...any) { write("ERROR", format, args...) }

func write(level, format string, args ...any) {
	line := fmt.Sprintf("%s [%s] %s\n", time.Now().Format("15:04:05.000"), level, fmt.Sprintf(format, args...))
	logMu.Lock()
	defer logMu.Unlock()
	// 发布版没有控制台：日志文件是排查的唯一入口。
	if f := openDaily(); f != nil {
		f.WriteString(line)
	}
	if level == "ERROR" {
		fmt.Fprint(os.Stderr, line)
	}
}

// openDaily 打开当天的日志文件；跨天时自动换一个。调用方需持有 logMu。
func openDaily() *os.File {
	if logDir == "" {
		return nil
	}
	name := filepath.Join(logDir, "wb-"+time.Now().Format("2006-01-02")+".log")
	if logFile != nil && logFile.Name() == name {
		return logFile
	}
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
	if err := EnsureDir(logDir); err != nil {
		return nil
	}
	f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil
	}
	logFile = f
	return f
}

// LogPath 给「打开日志」类入口用：返回今天的日志文件绝对路径。
func LogPath() string {
	if logDir == "" {
		return ""
	}
	return filepath.Join(logDir, "wb-"+time.Now().Format("2006-01-02")+".log")
}

// TailLog 读当日日志尾部若干行；读不到返回 nil。
func TailLog(maxLines int) []string {
	path := LogPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := splitLines(string(raw))
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return lines
}
