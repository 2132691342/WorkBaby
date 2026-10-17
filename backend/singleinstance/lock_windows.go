//go:build windows

package singleinstance

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockExclusive 以独占方式锁住文件首字节；已被锁说明另一个实例在跑。
func lockExclusive(f *os.File) error {
	var ol windows.Overlapped
	return windows.LockFileEx(windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, &ol)
}

// unlockFile 解锁。
func unlockFile(f *os.File) {
	var ol windows.Overlapped
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &ol)
}
