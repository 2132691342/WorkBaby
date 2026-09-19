package event

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// RunEvent 一条带序号的 run 事件（SSE 断线重放单元）。Data 是已序列化的事件载荷。
type RunEvent struct {
	Seq  int64  `json:"seq"`
	Name string `json:"name"`
	Data string `json:"data"`
}

// maxEventBytes 单条事件入缓冲的上限；超出的事件只分配序号不存内容（重放时判定为不覆盖）。
const maxEventBytes = 32 << 10

// runBuffer 单个 run 的环形事件缓冲；只覆盖秒级断线窗口，不做长期存储。
type runBuffer struct {
	seq    int64
	buf    []RunEvent
	next   int
	usedAt int64 // 逻辑时钟，LRU 淘汰依据（不用墙钟，避免同毫秒顺序不稳）
}

func (b *runBuffer) add(e RunEvent, usedAt int64) {
	b.usedAt = usedAt
	if len(b.buf) < cap(b.buf) {
		b.buf = append(b.buf, e)
		b.next = len(b.buf) % cap(b.buf)
		return
	}
	b.buf[b.next] = e
	b.next = (b.next + 1) % cap(b.buf)
}

// oldestSeq 缓冲中最老的序号；空缓冲返回 0。
func (b *runBuffer) oldestSeq() int64 {
	if len(b.buf) == 0 {
		return 0
	}
	if len(b.buf) < cap(b.buf) {
		return b.buf[0].Seq
	}
	return b.buf[b.next].Seq
}

// RunEventLog 按 run 分配单调序号并保留最近事件，供 SSE 断线重放。
// 序号是进程内语义：桌面单进程，跨重启的 run 不再继续，故不落库。
type RunEventLog struct {
	mu       sync.Mutex
	size     int   // 每 run 保留条数
	maxRuns  int   // 最多跟踪的 run 数（LRU 淘汰，防泄漏）
	clock    int64 // 单调递增逻辑时钟
	runs     map[string]*runBuffer
	fileRoot string // JSONL 无头导出根目录（空 = 不导出）
}

// NewRunEventLog 构造事件日志；size/maxRuns 非正时取默认值。
func NewRunEventLog(size, maxRuns int) *RunEventLog {
	if size <= 0 {
		size = 128
	}
	if maxRuns <= 0 {
		maxRuns = 16
	}
	return &RunEventLog{size: size, maxRuns: maxRuns, runs: make(map[string]*runBuffer)}
}

// Append 分配序号、把 seq 注入 payload、序列化并入缓冲；返回序号与 JSON 串。
//
// runID 为空（非 run 作用域事件）时只序列化不入缓冲；单条事件超过 maxEventBytes
// 时只记占位（Data 为空），重放遇到占位即判定为不覆盖，避免超大工具结果撑爆内存。
func (l *RunEventLog) Append(runID, name string, payload map[string]any) (int64, string) {
	if runID == "" {
		data, _ := json.Marshal(payload)
		return 0, string(data)
	}
	l.mu.Lock()
	l.clock++
	usedAt := l.clock
	b, ok := l.runs[runID]
	if !ok {
		if len(l.runs) >= l.maxRuns {
			l.evictLocked()
		}
		b = &runBuffer{buf: make([]RunEvent, 0, l.size)}
		l.runs[runID] = b
	}
	b.seq++
	seq := b.seq
	if payload != nil {
		payload["seq"] = seq
	}
	data, err := json.Marshal(payload)
	if err != nil {
		l.mu.Unlock()
		return seq, "{}"
	}
	e := RunEvent{Seq: seq, Name: name, Data: string(data)}
	if len(e.Data) > maxEventBytes {
		e.Data = ""
	}
	b.add(e, usedAt)
	root := l.fileRoot
	l.mu.Unlock()
	// 无头导出：每条事件一行 JSONL，best-effort
	if root != "" {
		l.appendFile(runID, name, seq, string(data))
		if !ok {
			l.pruneFiles()
		}
	}
	return seq, string(data)
}

// Replay 返回 afterSeq 之后的事件；covered=false 表示缓冲已滚动覆盖或存在超大占位事件，
// 无法完整重放（调用方应推一条 gap 事件让前端转全量回补）。
func (l *RunEventLog) Replay(runID string, afterSeq int64) ([]RunEvent, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.runs[runID]
	if b == nil || len(b.buf) == 0 {
		return nil, true
	}
	if afterSeq > 0 && b.oldestSeq() > afterSeq+1 {
		return nil, false
	}
	out := make([]RunEvent, 0, len(b.buf))
	start := b.next % len(b.buf)
	for i := 0; i < len(b.buf); i++ {
		e := b.buf[(start+i)%len(b.buf)]
		if e.Seq <= afterSeq {
			continue
		}
		if e.Data == "" {
			return nil, false
		}
		out = append(out, e)
	}
	return out, true
}

// Drop 释放 run 的缓冲（run 终态且前端已消费完成后调用）。
func (l *RunEventLog) Drop(runID string) {
	l.mu.Lock()
	delete(l.runs, runID)
	l.mu.Unlock()
}

// evictLocked 淘汰最久未写入的 run；调用方持锁。
func (l *RunEventLog) evictLocked() {
	oldest, at := "", int64(1<<63-1)
	for id, b := range l.runs {
		if b.usedAt < at {
			at, oldest = b.usedAt, id
		}
	}
	if oldest != "" {
		delete(l.runs, oldest)
	}
}

// fileSinkKeep JSONL 导出保留的 run 文件上限，更早的删除。
const fileSinkKeep = 50

// WithFileSink 启用 JSONL 无头导出：每条事件追加一行 {seq,name,ts,data} 到 {root}/{runID}.jsonl。
func (l *RunEventLog) WithFileSink(root string) *RunEventLog {
	l.mu.Lock()
	l.fileRoot = root
	l.mu.Unlock()
	return l
}

// appendFile best-effort 追加一行 JSONL；任何失败静默（导出不影响主路径）。
func (l *RunEventLog) appendFile(runID, name string, seq int64, data string) {
	if runID == "" || filepath.Base(runID) != runID {
		return
	}
	l.mu.Lock()
	root := l.fileRoot
	l.mu.Unlock()
	if root == "" {
		return
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return
	}
	line := struct {
		Seq  int64           `json:"seq"`
		Name string          `json:"name"`
		TS   int64           `json:"ts"`
		Data json.RawMessage `json:"data"`
	}{seq, name, time.Now().UnixMilli(), json.RawMessage(data)}
	bs, err := json.Marshal(line)
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(root, runID+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(bs, '\n'))
}

// pruneFiles 保留最近 fileSinkKeep 个 JSONL（按 mtime），更早删除。
func (l *RunEventLog) pruneFiles() {
	l.mu.Lock()
	root := l.fileRoot
	l.mu.Unlock()
	if root == "" {
		return
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	type fe struct {
		name string
		mod  int64
	}
	list := make([]fe, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		if info, ierr := e.Info(); ierr == nil {
			list = append(list, fe{e.Name(), info.ModTime().Unix()})
		}
	}
	if len(list) <= fileSinkKeep {
		return
	}
	sort.Slice(list, func(i, j int) bool { return list[i].mod > list[j].mod })
	for _, f := range list[fileSinkKeep:] {
		_ = os.Remove(filepath.Join(root, f.name))
	}
}

// Export 返回 run 的 JSONL 字节流（无头导出/回放）；未启用文件 sink 或文件不存在返回错误。
func (l *RunEventLog) Export(runID string) ([]byte, error) {
	if runID == "" || filepath.Base(runID) != runID {
		return nil, fmt.Errorf("invalid run id: %s", runID)
	}
	l.mu.Lock()
	root := l.fileRoot
	l.mu.Unlock()
	if root == "" {
		return nil, fmt.Errorf("run event file sink not enabled")
	}
	bs, err := os.ReadFile(filepath.Join(root, runID+".jsonl"))
	if err != nil {
		return nil, err
	}
	return bs, nil
}
