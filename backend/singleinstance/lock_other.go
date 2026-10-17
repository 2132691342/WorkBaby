//go:build !windows

package singleinstance

import (
	"os"
	"syscall"
)

// lockExclusive 非 Windows 上用 flock（本应用仅面向 Windows，这里只为保持可编译）。
func lockExclusive(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

// unlockFile 解锁。
func unlockFile(f *os.File) { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
