// Package singleinstance 保证只跑一个实例：二次启动把这一趟的意图转交主实例后退出。
package singleinstance

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"WorkBaby/backend/pkg"
)

// ipcReadTimeout 是单次 IPC 报文的读取上限：本地回环上的一行，
// 给到 5s 已经比任何真实调用慢一个数量级。
const ipcReadTimeout = 5 * time.Second

// ErrInstanceAlreadyRunning 表示已有实例在运行。
var ErrInstanceAlreadyRunning = pkg.New(2301, "已经有一个 WorkBaby 在跑了", "")

// ipcPortFile 记录主实例的 IPC 端口。
const ipcPortFile = "ipc.port"

// Instance 是单实例句柄。
type Instance struct {
	dir     string
	ln      net.Listener
	files   chan string
	release func()
	once    sync.Once
}

// Acquire 抢占单实例；已被占用时返回 ErrInstanceAlreadyRunning。
func Acquire(dir string) (*Instance, error) {
	if err := pkg.EnsureDir(dir); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(dir, "app.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, pkg.Wrap(2301, "创建实例锁失败", err)
	}
	// Windows 上以独占方式锁住首字节；已被锁说明另一个实例在跑。
	if err := lockExclusive(f); err != nil {
		_ = f.Close()
		return nil, ErrInstanceAlreadyRunning
	}
	return &Instance{
		dir:     dir,
		files:   make(chan string, 8),
		release: func() { unlockFile(f); _ = f.Close() },
	}, nil
}

// StartListener 监听本地 IPC，接收二次启动转交的文件路径。
func (i *Instance) StartListener() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return pkg.Wrap(2302, "启动实例通信失败", err)
	}
	i.ln = ln
	port := ln.Addr().(*net.TCPAddr).Port
	if err := pkg.WriteText(filepath.Join(i.dir, ipcPortFile), itoa(port)); err != nil {
		return err
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go i.handle(conn)
		}
	}()
	return nil
}

func (i *Instance) handle(conn net.Conn) {
	defer conn.Close()
	// 对端发一半就卡住时没有 deadline 会把这条 goroutine 和 conn 永久挂着：
	// 第二个实例启动几次就积几条，锁释放后它们还在等一段永远不会到的换行。
	if err := conn.SetReadDeadline(time.Now().Add(ipcReadTimeout)); err != nil {
		return
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return
	}
	select {
	case i.files <- trimNewline(line):
	default:
	}
}

// FileChannel 返回被转交的启动意图通道：要打开的文件路径，或空串（只唤起窗口）。
func (i *Instance) FileChannel() <-chan string { return i.files }

// Release 释放实例锁与监听。
func (i *Instance) Release() {
	i.once.Do(func() {
		if i.ln != nil {
			_ = i.ln.Close()
		}
		if i.release != nil {
			i.release()
		}
	})
}

// SendPathToRunningInstance 把这次启动的意图发给主实例：path 为空表示只想唤起窗口。
func SendPathToRunningInstance(dir, path string) error {
	raw, err := os.ReadFile(filepath.Join(dir, ipcPortFile))
	if err != nil {
		return pkg.Wrap(2303, "找不到正在运行的实例", err)
	}
	port := 0
	for _, r := range string(raw) {
		if r < '0' || r > '9' {
			break
		}
		port = port*10 + int(r-'0')
	}
	if port == 0 {
		return pkg.New(2304, "实例端口记录无效", "")
	}
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return pkg.Wrap(2303, "连接正在运行的实例失败", err)
	}
	defer conn.Close()
	_, err = conn.Write([]byte(path + "\n"))
	return err
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
