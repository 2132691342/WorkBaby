// 索引生命周期：Frecency 并发记账、检索出口与重建并发、Build 的可取消性。
// 这一组的共同点是「索引是共享可变状态」：界面检索、模型工具、后台重建
// 三路并发读写同一份账与同一份文档集，任何一路裸奔都是进程级事故。
package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/index"
	"WorkBaby/backend/pkg"
)

func TestIndexLifecycle(t *testing.T) {
	// Frecency 的三份操作（Record/Score/Hits）并发打同一份账。
	// 裸 map 并发写是进程级 fatal，不带 -race 也会当场崩掉整个测试进程；
	// 「活下来 + 计数分毫不差」就是锁在位的证据。
	t.Run("Frecency 并发记账不丢不炸", func(t *testing.T) {
		f := index.NewFrecency()
		const workers = 8
		const iterations = 300
		now := pkg.NowMs()
		var wg sync.WaitGroup
		for w := range workers {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				docID := fmt.Sprintf("F:doc_%d.go", worker%4)
				for range iterations {
					f.Record(docID, now)
					// Score 与 Hits 读同一份账：读写交错才是真实的并发形状。
					if score := f.Score(docID, now); score < 0 {
						t.Errorf("分数不该为负：%f", score)
						return
					}
					_ = f.Hits(docID)
				}
			}(w)
		}
		wg.Wait()

		// 每个文档由两个协程各记 300 次：锁丢了会计数短缺或直接崩。
		total := 0
		for d := range 4 {
			docID := fmt.Sprintf("F:doc_%d.go", d)
			if got := f.Hits(docID); got != 2*iterations {
				t.Fatalf("%s 命中数应为 %d，实际 %d（并发记账丢了）", docID, 2*iterations, got)
			}
			total += f.Hits(docID)
			// 刚记过的账时近满分：分数应为正。
			if score := f.Score(docID, now); score <= 0 {
				t.Fatalf("记过 %d 次的文档分数应为正，实际 %f", 2*iterations, score)
			}
		}
		if total != workers*iterations {
			t.Fatalf("总命中应为 %d，实际 %d", workers*iterations, total)
		}
		// 没记过的文档：零分、零命中，不该被并发写污染。
		if got := f.Hits("F:never.go"); got != 0 {
			t.Fatalf("没记过的文档命中应为 0，实际 %d", got)
		}
		if score := f.Score("F:never.go", now); score != 0 {
			t.Fatalf("没记过的文档分数应为 0，实际 %f", score)
		}
	})

	// 检索出口并发：HTTP /index/search 与引擎 Kit().Search 同时打，
	// 检索里的「读 docs → 打分 → 记账」与重建的整份替换必须互斥。
	// 先建好索引再放并发：所有请求都该命中且 code=0，不出现半新半旧的崩溃。
	t.Run("检索出口并发命中", func(t *testing.T) {
		h := newHarness(t)
		workspace := t.TempDir()
		writeFile(t, filepath.Join(workspace, "alpha.go"), "package main\n\nfunc NeedleAlpha() {}\n")
		writeFile(t, filepath.Join(workspace, "beta.go"), "package main\n\nfunc NeedleBeta() {}\n")
		mustSetWorkspace(t, h, workspace)

		env, code := h.call("POST", "/index/rebuild", nil)
		h.mustOK(env, code, "触发重建", nil)
		status := waitIndexBuilt(t, h)
		if status.IndexDocCount == 0 {
			t.Fatalf("建完索引文档数应大于 0：%+v", status)
		}

		kit := h.app.Searcher.Kit()
		const perLane = 25
		var wg sync.WaitGroup
		// HTTP 一路。
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range perLane {
				env, code := h.call("POST", "/index/search", domain.IndexSearchREQ{SearchQuery: "needlealpha", SearchLimit: 5})
				if code != 200 || env.Code != pkg.CodeOK {
					t.Errorf("HTTP 检索第 %d 次失败：http=%d code=%q %s", i, code, env.Code, env.Message)
					return
				}
				var resp domain.IndexSearchRESP
				if err := json.Unmarshal(env.Data, &resp); err != nil {
					t.Errorf("解析检索结果失败：%v", err)
					return
				}
				if len(resp.SearchHits) == 0 {
					t.Errorf("HTTP 检索第 %d 次应有命中", i)
					return
				}
			}
		}()
		// 引擎 Kit 一路：与 HTTP 走同一个 Searcher，锁必须两边都罩得住。
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range perLane {
				hits, err := kit.Search(context.Background(), "needlebeta", 5)
				if err != nil {
					t.Errorf("Kit 检索第 %d 次失败：%v", i, err)
					return
				}
				if len(hits) == 0 {
					t.Errorf("Kit 检索第 %d 次应有命中", i)
					return
				}
			}
		}()
		wg.Wait()

		// 并发检索之后索引仍完好：状态可读、文档数没被写坏。
		after := h.app.Searcher.Status()
		if after.IndexDocCount != status.IndexDocCount {
			t.Fatalf("并发检索不该改变文档数：之前 %d，之后 %d", status.IndexDocCount, after.IndexDocCount)
		}
	})

	// building 且无索引时 Search 回 5002（引擎忙），而不是空结果装没事。
	// 用一个大工作区把「第一次 EnsureIndexed 触发的后台构建」窗口拉长，
	// 窗口内的检索只允许 5002 或（构建恰好收尾后的）成功；收尾后必须成功。
	t.Run("构建中检索回引擎忙", func(t *testing.T) {
		h := newHarness(t)
		workspace := t.TempDir()
		makeBigTree(t, workspace, 400)
		mustSetWorkspace(t, h, workspace)

		kit := h.app.Searcher.Kit()
		const lanes = 8
		const perLane = 10
		var wg sync.WaitGroup
		var mu sync.Mutex
		busySeen := 0
		okSeen := 0
		for lane := range lanes {
			wg.Add(1)
			go func(lane int) {
				defer wg.Done()
				for range perLane {
					var code string
					if lane%2 == 0 {
						env, _ := h.call("POST", "/index/search", domain.IndexSearchREQ{SearchQuery: "symbol0001", SearchLimit: 5})
						code = env.Code
					} else {
						code = pkg.CodeOK
						if _, err := kit.Search(context.Background(), "symbol0001", 5); err != nil {
							code = pkg.As(err).Code
						}
					}
					switch code {
					case pkg.CodeEngineBusy:
						mu.Lock()
						busySeen++
						mu.Unlock()
					case pkg.CodeOK:
						mu.Lock()
						okSeen++
						mu.Unlock()
					default:
						t.Errorf("构建窗口内检索只该回 %q 或 %q，实际 %q", pkg.CodeEngineBusy, pkg.CodeOK, code)
						return
					}
				}
			}(lane)
		}
		wg.Wait()
		if busySeen == 0 {
			t.Fatal("构建中的检索应至少见到一次 5002（building 且无索引）")
		}
		if busySeen+okSeen != lanes*perLane {
			t.Fatalf("每次检索都该被记账：应 %d 次，实际 busy=%d ok=%d", lanes*perLane, busySeen, okSeen)
		}

		status := waitIndexBuilt(t, h)
		if status.IndexDocCount == 0 {
			t.Fatalf("收尾后文档数应大于 0：%+v", status)
		}
		// 收尾后同样的检索必须成功且有命中：5002 只是暂态，不是终态。
		env, code := h.call("POST", "/index/search", domain.IndexSearchREQ{SearchQuery: "symbol0001", SearchLimit: 5})
		var resp domain.IndexSearchRESP
		h.mustOK(env, code, "收尾后检索", &resp)
		if len(resp.SearchHits) == 0 {
			t.Fatal("收尾后检索应有命中")
		}
	})

	// ctx 取消必须真的停下扫描：预取消透传 context.Canceled；
	// 大目录树中途 cancel 要在 2 秒内带回错误且产出小于全量，而不是扫完全程。
	t.Run("构建可取消", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "main.go"), "package main\n\nfunc Hello() {}\n")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		docs, err := index.Build(ctx, root)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("预取消的 Build 应回 context.Canceled（透传不包装），实际 %v", err)
		}
		if len(docs) != 0 {
			t.Fatalf("预取消的 Build 不该产出文档，实际 %d 条", len(docs))
		}

		// Rebuild 同步语义保留：扫描失败归到 1003（文件 IO），ctx 原话进 Detail。
		// 注意 WrapErr 只把 err.Error() 塞进 Detail、不保留错误链，
		// 所以这里断言 Detail 带原话，而不是 errors.Is（链已被掐断）。
		h := newHarness(t)
		mustSetWorkspace(t, h, root)
		_, rebuildErr := h.app.Searcher.Rebuild(ctx)
		if rebuildErr == nil {
			t.Fatal("预取消的 Rebuild 应失败")
		}
		ae := pkg.As(rebuildErr)
		if ae.Code != pkg.CodeFileIO {
			t.Fatalf("Rebuild 应把扫描失败包成 %q，实际 %q（%v）", pkg.CodeFileIO, ae.Code, rebuildErr)
		}
		if !strings.Contains(ae.Detail, context.Canceled.Error()) {
			t.Fatalf("Detail 应带 ctx 取消原话，实际 %q", ae.Detail)
		}
		// 失败也要收尾：building 不能悬挂成 true，否则界面永远显示正在建。
		if st := h.app.Searcher.Status(); st.IndexBuilding {
			t.Fatalf("失败的 Rebuild 应清掉 building：%+v", st)
		}

		// 大目录树中途 cancel：2 秒内带 ctx 错误回来，产出小于全量——扫描没有硬扛到底。
		big := t.TempDir()
		const files = 600
		makeBigTree(t, big, files)
		full, err := index.Build(context.Background(), big)
		if err != nil {
			t.Fatalf("全量构建失败：%v", err)
		}
		if len(full) < files {
			t.Fatalf("全量构建应至少产出 %d 条文档，实际 %d", files, len(full))
		}
		scanCtx, scanCancel := context.WithCancel(context.Background())
		type result struct {
			docs []index.Doc
			err  error
		}
		done := make(chan result, 1)
		go func() {
			scanDocs, scanErr := index.Build(scanCtx, big)
			done <- result{docs: scanDocs, err: scanErr}
		}()
		// 让它真扫起来一点再取消：既不是预取消，也不给扫完的机会。
		time.Sleep(10 * time.Millisecond)
		scanCancel()

		select {
		case got := <-done:
			if !errors.Is(got.err, context.Canceled) {
				t.Fatalf("中途取消应回 context.Canceled，实际 %v", got.err)
			}
			if len(got.docs) >= len(full) {
				t.Fatalf("取消后产出应小于全量（%d），实际 %d——扫描没有停下", len(full), len(got.docs))
			}
		case <-time.After(2 * time.Second):
			t.Fatal("取消后 2 秒仍未返回：Build 没有理会 ctx")
		}
	})

	// HTTP 侧：重建已后台化——POST /index/rebuild 立即受理（building=true），
	// 最终状态靠 /index/status 轮询；HTTP 请求结束不牵连后台构建
	//（RebuildAsync 内部用 context.Background()，不随请求 ctx 取消）。
	t.Run("HTTP 重建后台化且立即受理", func(t *testing.T) {
		h := newHarness(t)

		// 无工作区：仍回 5000，后台化不改变入口校验。
		env, _ := h.call("POST", "/index/rebuild", nil)
		if env.Code != pkg.CodeSessionBind {
			t.Fatalf("无工作区重建应回 %q，实际 %q（%s）", pkg.CodeSessionBind, env.Code, env.Message)
		}

		workspace := t.TempDir()
		writeFile(t, filepath.Join(workspace, "main.go"), "package main\n\nfunc Hello() {}\n")
		mustSetWorkspace(t, h, workspace)

		env, code := h.call("POST", "/index/rebuild", nil)
		var accepted domain.IndexStatusVO
		h.mustOK(env, code, "触发重建", &accepted)
		// 小工作区可能瞬间建完：受理应答只断言「没有报错、状态形状正确」，
		// building 的真值交给下面的幂等与轮询钉。
		if accepted.IndexBuilding && accepted.IndexBuilt {
			t.Fatalf("受理应答不该同时报在建与已建：%+v", accepted)
		}

		status := waitIndexBuilt(t, h)
		if status.IndexDocCount == 0 || status.IndexWorkspace != workspace {
			t.Fatalf("收尾状态应指向该工作区且有文档：%+v", status)
		}

		// 收尾后再触发一次：幂等重建，最终仍收敛到已建、不在建。
		env, code = h.call("POST", "/index/rebuild", nil)
		h.mustOK(env, code, "再次触发重建", nil)
		again := waitIndexBuilt(t, h)
		if again.IndexDocCount == 0 {
			t.Fatalf("二次重建后文档数应大于 0：%+v", again)
		}
	})
}
