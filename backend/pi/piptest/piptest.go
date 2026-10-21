// Package piptest 给测试提供可编程的假 PI 进程与仓库定位，只被测试引用，不进主程序。
// 夹具本体在 backend/pi/pitest（package main）：它必须编译成独立 exe 才能真的起子进程。
package piptest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// RepoRoot 从测试工作目录往上找 go.mod；测试要在仓库根编译夹具、拼相对路径。
func RepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("找不到仓库根目录（go.mod）")
		}
		dir = parent
	}
}

// 夹具只编译一次：三个包各编一次会把总时长做三次乘法。产物按夹具源码内容寻址，
// 内容没变就直接复用；三个包的测试并行跑，所以编译前要拿一把跨进程锁。
var (
	buildMu   sync.Mutex
	buildOnce sync.Map // 内容指纹 → exe 路径
)

// Build 编译假 PI，返回 exe 路径。同一份源码在一次测试运行内只编译一次。
func Build(t *testing.T) string {
	t.Helper()
	buildMu.Lock()
	defer buildMu.Unlock()

	fingerprint, err := fixtureFingerprint()
	if err != nil {
		t.Fatalf("算不出夹具指纹: %v", err)
	}
	if hit, ok := buildOnce.Load(fingerprint); ok {
		return hit.(string)
	}

	exe := filepath.Join(os.TempDir(), "workbaby-fakepi-"+fingerprint+".exe")
	if err := buildFixture(RepoRoot(t), exe); err != nil {
		t.Fatalf("编译假 PI 失败: %v", err)
	}
	buildOnce.Store(fingerprint, exe)
	return exe
}

// buildFixture 在跨进程锁下编译夹具：谁先拿到锁谁编，后来者直接用现成的产物。
func buildFixture(root, exe string) error {
	lock, err := os.OpenFile(exe+".lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		// 锁被别人持着：等它编完即可。
		if !waitForFixture(exe, 60*time.Second) {
			return fmt.Errorf("等另一个包编译夹具超时")
		}
		return nil
	}
	defer func() {
		_ = lock.Close()
		_ = os.Remove(exe + ".lock")
	}()

	// 拿到锁时产物可能已经在了（别人编完就退了）。
	if _, err := os.Stat(exe); err == nil {
		return nil
	}
	cmd := exec.Command("go", "build", "-o", exe, "./backend/pi/pitest")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v\n%s", err, out)
	}
	return nil
}

// waitForFixture 轮询等夹具产物出现。
func waitForFixture(exe string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(exe); err == nil {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// fixtureFingerprint 取夹具源码与其依赖的指纹：内容变了就换一个 exe 名，
// 不会拿到上一次的旧产物。依赖只算夹具包自己——它只依赖标准库。
func fixtureFingerprint() (string, error) {
	root := ""
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			root = dir
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}

	files, err := filepath.Glob(filepath.Join(root, "backend", "pi", "pitest", "*.go"))
	if err != nil {
		return "", err
	}
	h := newDigest()
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		h.write([]byte(f))
		h.write(raw)
	}
	return h.sum(), nil
}
