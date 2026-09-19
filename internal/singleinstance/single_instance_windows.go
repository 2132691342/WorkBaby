//go:build windows

// Package singleinstance 单实例保护：Windows 命名互斥 + 本地 TCP IPC。
//
// 第二次启动时把命令行里的文件路径（文件关联打开）经 IPC 转交给主实例后退出，
// 避免多实例竞争托盘图标 / DB 锁。互斥名按可执行文件名派生，避免不同 WorkBaby 构建互相冲突。
package singleinstance

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modkernel32      = windows.NewLazySystemDLL("kernel32.dll")
	procCreateMutexW = modkernel32.NewProc("CreateMutexW")
	procGetLastError = modkernel32.NewProc("GetLastError")
	procReleaseMutex = modkernel32.NewProc("ReleaseMutex")
	procCloseHandle  = modkernel32.NewProc("CloseHandle")
)

// mutex 名称前缀：Local\ 仅当前用户可见，避免污染全局名空间。
const mutexPrefix = "Local\\WorkBaby-"

// IPC 监听端口与令牌：仅绑定回环；令牌校验防本机其他进程伪造路径。
const (
	ipcPort  = 27619
	ipcToken = "WorkBaby-IPC-v1"
)

// ErrInstanceAlreadyRunning 已有主实例在运行（main 据此转交路径后退出）。
var ErrInstanceAlreadyRunning = errors.New("workbaby: another instance is already running")

// SingleInstance 单实例守卫：成功时由调用方保留句柄，进程退出时 Release。
type SingleInstance struct {
	handle windows.Handle

	fileCh   chan string
	listener net.Listener
	stopCh   chan struct{}
	wg       sync.WaitGroup
	mu       sync.Mutex
	started  bool
}

// Acquire 尝试获取命名互斥；已被占用时返回 ErrInstanceAlreadyRunning。
func Acquire() (*SingleInstance, error) {
	name, err := mutexName()
	if err != nil {
		return nil, fmt.Errorf("derive mutex name: %w", err)
	}
	ptr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	// CreateMutexW 第二参 bInitialOwner=true 即「获得所有权」语义；
	// 若已存在同名互斥，调用失败且 GetLastError 返回 ERROR_ALREADY_EXISTS。
	h, _, errno := syscall.SyscallN(procCreateMutexW.Addr(),
		0, // lpMutexAttributes（默认安全描述符）
		1, // bInitialOwner
		uintptr(unsafe.Pointer(ptr)),
	)
	if h == 0 {
		return nil, fmt.Errorf("CreateMutexW failed: errno=%v", errno)
	}
	le, _, _ := syscall.SyscallN(procGetLastError.Addr())
	if le == 0 {
		return &SingleInstance{
			handle: windows.Handle(h),
			fileCh: make(chan string, 10),
			stopCh: make(chan struct{}),
		}, nil
	}
	const errorAlreadyExists = 183
	if uint32(le) == errorAlreadyExists {
		// 立刻关闭本进程拿到的句柄（句柄由 CreateMutex 返回，不是真正的所有权）。
		_, _, _ = syscall.SyscallN(procCloseHandle.Addr(), h)
		return nil, ErrInstanceAlreadyRunning
	}
	return nil, fmt.Errorf("CreateMutexW: GetLastError=%v", le)
}

// Release 释放互斥并停掉 IPC 监听（正常退出时调用；panic 路径 OS 自动回收）。
func (s *SingleInstance) Release() {
	if s == nil {
		return
	}
	s.StopListener()
	if s.handle != 0 {
		_, _, _ = syscall.SyscallN(procReleaseMutex.Addr(), uintptr(s.handle))
		_, _, _ = syscall.SyscallN(procCloseHandle.Addr(), uintptr(s.handle))
		s.handle = 0
	}
}

// FileChannel 返回从二次启动实例收到的文件路径流。
func (s *SingleInstance) FileChannel() <-chan string {
	if s == nil {
		return nil
	}
	return s.fileCh
}

// StartListener 启动本地 IPC 监听（回环 + 固定端口），接收二次启动实例转交的文件路径。
// 端口被占用（理论上只可能是残留进程）时静默降级：文件关联退化为「唤不起主实例」。
func (s *SingleInstance) StartListener() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return nil
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", ipcPort))
	if err != nil {
		return fmt.Errorf("ipc listen: %w", err)
	}
	s.listener = ln
	s.started = true
	s.wg.Add(1)
	go s.acceptLoop()
	return nil
}

// StopListener 关闭监听并等待接收循环退出。
func (s *SingleInstance) StopListener() {
	if s == nil {
		return
	}
	s.mu.Lock()
	ln := s.listener
	s.listener = nil
	started := s.started
	s.started = false
	if s.stopCh != nil {
		select {
		case <-s.stopCh:
		default:
			close(s.stopCh)
		}
	}
	s.mu.Unlock()
	if ln != nil {
		_ = ln.Close()
	}
	if started {
		s.wg.Wait()
	}
}

// acceptLoop 接受 IPC 连接；超时轮询以便响应 stopCh。
func (s *SingleInstance) acceptLoop() {
	defer s.wg.Done()
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}
		if s.listener == nil {
			return
		}
		_ = s.listener.(*net.TCPListener).SetDeadline(time.Now().Add(500 * time.Millisecond))
		conn, err := s.listener.Accept()
		if err != nil {
			var opErr *net.OpError
			if errors.As(err, &opErr) && opErr.Timeout() {
				continue
			}
			return // 监听器已关闭
		}
		s.handleConn(conn)
	}
}

// handleConn 读取并校验一条「token\npath\n」消息，路径入队（满则丢弃）。
func (s *SingleInstance) handleConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(conn)
	token, err := reader.ReadString('\n')
	if err != nil || token != ipcToken+"\n" {
		return
	}
	path, err := reader.ReadString('\n')
	if err != nil {
		return
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	select {
	case s.fileCh <- path:
	case <-time.After(time.Second):
	}
}

// SendPathToRunningInstance 把文件路径转交给主实例（二次启动进程调用）。
func SendPathToRunningInstance(path string) error {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", ipcPort), 2*time.Second)
	if err != nil {
		return fmt.Errorf("dial main instance: %w", err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, err = fmt.Fprintf(conn, "%s\n%s\n", ipcToken, path)
	return err
}

// mutexName 从可执行文件名派生互斥名：Local\WorkBaby-{baseName}。
func mutexName() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	base := filepath.Base(exe)
	// 去掉扩展名与可能的空格（互斥名不接受空格）
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.ReplaceAll(base, " ", "_")
	return mutexPrefix + base, nil
}
