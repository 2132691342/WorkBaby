// 单实例转交链路：二次启动给主实例写一行（文件路径，或空行 = 只唤起窗口），
// 主实例必须原样收到——收不到就是「双击文件没反应」「再点一次图标窗口不出现」。
package singleinstance

import (
	"testing"
	"time"
)

func TestSecondLaunchHandoff(t *testing.T) {
	dir := t.TempDir()
	inst, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(inst.Release)
	if err := inst.StartListener(); err != nil {
		t.Fatal(err)
	}

	recv := func(what string) string {
		t.Helper()
		select {
		case p := <-inst.FileChannel():
			return p
		case <-time.After(3 * time.Second):
			t.Fatalf("等待转交超时：%s", what)
			return ""
		}
	}

	// 两种载荷走同一条通道：带路径 = 打开文件，空路径 = 只想把已有窗口带出来。
	// 空的那次最容易被「顺手过滤掉空值」的写法吞掉，所以必须一起断言。
	want := `C:\demo\季度总结.md`
	if err := SendPathToRunningInstance(dir, want); err != nil {
		t.Fatalf("发送失败: %v", err)
	}
	if got := recv("文件路径"); got != want {
		t.Fatalf("路径没有原样送达: %q", got)
	}
	if err := SendPathToRunningInstance(dir, ""); err != nil {
		t.Fatalf("发送失败: %v", err)
	}
	if got := recv("唤起请求"); got != "" {
		t.Fatalf("唤起请求应送达空路径: %q", got)
	}
}
