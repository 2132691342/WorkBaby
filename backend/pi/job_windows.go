//go:build windows

// 子进程的生命周期兜底：把每个 PI 进程挂进本进程的 Job 对象，
// WorkBaby 被强杀（任务管理器 / 崩溃）时由系统一起带走，不留还在烧 token 的孤儿内核。
package pi

import (
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"WorkBaby/backend/pkg"
)

var (
	jobOnce   sync.Once
	jobHandle windows.Handle
	jobErr    error
)

// attachToJob 把已启动的子进程挂进 Job；失败只告警，不影响正常使用。
func attachToJob(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	job, err := jobObject()
	if err != nil {
		pkg.Warnf("pi: 创建进程组失败，强杀应用后可能留下孤儿进程: %v", err)
		return
	}
	ph, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		pkg.Warnf("pi: 打开进程 %d 失败: %v", cmd.Process.Pid, err)
		return
	}
	defer windows.CloseHandle(ph)
	if err := windows.AssignProcessToJobObject(job, ph); err != nil {
		pkg.Warnf("pi: 进程 %d 挂不进进程组: %v", cmd.Process.Pid, err)
	}
}

// jobObject 懒创建 Job：KILL_ON_JOB_CLOSE 让最后一个句柄关闭（进程消失）时杀掉组内全部进程。
func jobObject() (windows.Handle, error) {
	jobOnce.Do(func() {
		h, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			jobErr = err
			return
		}
		var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if _, err := windows.SetInformationJobObject(
			h, windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)),
		); err != nil {
			windows.CloseHandle(h)
			jobErr = err
			return
		}
		jobHandle = h
	})
	return jobHandle, jobErr
}
