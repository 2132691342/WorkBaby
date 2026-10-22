// 进程监督：一个工作区一个 PI 进程，按需拉起、复用、死亡后重建。
// cwd 是进程级属性，跨工作区的会话不能共用一个进程。
package pi

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"WorkBaby/backend/pkg"
)

// Supervisor 管理一组按工作区划分的 PI 进程。
type Supervisor struct {
	mu    sync.Mutex
	procs map[string]*Proc
	// factory 生成进程配置（注入环境与可执行路径），由 env 层提供。
	factory func(workspace string) (Config, error)
	handler func(workspace string) Handler
}

// Proc 是一个工作区上的 PI 进程与其客户端。
// mu 是拉起闸门：同一工作区的并发首启要串行，否则会同时 spawn 两个进程。
type Proc struct {
	Workspace string
	Client    *Client

	mu sync.Mutex
}

// NewSupervisor 构造监督器。
func NewSupervisor(factory func(workspace string) (Config, error), handler func(workspace string) Handler) *Supervisor {
	return &Supervisor{
		procs:   map[string]*Proc{},
		factory: factory,
		handler: handler,
	}
}

// keyOf 归一并校验工作区键；空工作区视为无效。
func keyOf(workspace string) string {
	w := strings.TrimSpace(workspace)
	if w == "" {
		return ""
	}
	return filepath.Clean(w)
}

// Ensure 取（必要时拉起）某工作区的进程；ctx 只用于闸门判定，
// 拉起本身是同步的 cmd.Start——用户已取消时不再白起进程。
func (s *Supervisor) Ensure(ctx context.Context, workspace string) (*Client, error) {
	key := keyOf(workspace)
	if key == "" {
		return nil, pkg.New("9706", "这个会话没有设置工作目录", "")
	}

	s.mu.Lock()
	p := s.procs[key]
	if p == nil {
		p = &Proc{Workspace: key}
		s.procs[key] = p
	}
	s.mu.Unlock()

	// 每个工作区一把闸门：并发首启只拉起一次，不做重复 spawn。
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Client != nil && p.Client.Alive() {
		return p.Client, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, pkg.Wrap("9703", "启动助手被取消", err)
	}

	cfg, err := s.factory(key)
	if err != nil {
		return nil, err
	}
	cfg.Workspace = key
	client, err := Start(cfg, s.handler(key))
	if err != nil {
		return nil, err
	}
	p.Client = client
	return client, nil
}

// Get 只取已有进程，不拉起。
func (s *Supervisor) Get(workspace string) *Client {
	s.mu.Lock()
	p := s.procs[keyOf(workspace)]
	s.mu.Unlock()
	if p == nil {
		return nil
	}
	if p.Client != nil && p.Client.Alive() {
		return p.Client
	}
	return nil
}

// Restart 结束并重建某工作区的进程（改 mcp.json 或换模型后需要）。
func (s *Supervisor) Restart(ctx context.Context, workspace string) (*Client, error) {
	s.Stop(workspace)
	return s.Ensure(ctx, workspace)
}

// Stop 有序结束一个工作区的进程。
func (s *Supervisor) Stop(workspace string) {
	key := keyOf(workspace)
	s.mu.Lock()
	p := s.procs[key]
	delete(s.procs, key)
	s.mu.Unlock()
	if p != nil && p.Client != nil {
		p.Client.Stop()
	}
}

// StopAll 结束全部进程（应用退出）。
func (s *Supervisor) StopAll() {
	s.mu.Lock()
	procs := make([]*Proc, 0, len(s.procs))
	for _, p := range s.procs {
		procs = append(procs, p)
	}
	s.procs = map[string]*Proc{}
	s.mu.Unlock()

	// 并行关闭：串行等待多个进程各自的 5s 宽限期会拖长退出。
	var wg sync.WaitGroup
	for _, p := range procs {
		if p.Client == nil {
			continue
		}
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			c.Stop()
		}(p.Client)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
	}
}

// Running 报告某工作区是否有活进程。
func (s *Supervisor) Running(workspace string) bool {
	return s.Get(workspace) != nil
}

// RunningCount 数一数当前活着的进程：引擎页用它说明「升级后需要重启几个」。
func (s *Supervisor) RunningCount() int {
	s.mu.Lock()
	procs := make([]*Proc, 0, len(s.procs))
	for _, p := range s.procs {
		procs = append(procs, p)
	}
	s.mu.Unlock()

	n := 0
	for _, p := range procs {
		if p.Client != nil && p.Client.Alive() {
			n++
		}
	}
	return n
}

// RestartIdle 重启进程。这一层看不出「哪个工作区有会话在跑」，所以保守地全部重启。
func (s *Supervisor) RestartIdle(ctx context.Context) error {
	return s.RestartAll()
}

// RestartAll 重启全部进程（改 mcp.json / 改工具集后需要）。
func (s *Supervisor) RestartAll() error {
	s.mu.Lock()
	keys := make([]string, 0, len(s.procs))
	for k := range s.procs {
		keys = append(keys, k)
	}
	s.mu.Unlock()

	ctx := context.Background()
	for _, k := range keys {
		s.Stop(k)
		if _, err := s.Ensure(ctx, k); err != nil {
			return err
		}
	}
	return nil
}
