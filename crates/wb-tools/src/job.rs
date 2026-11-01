//! Windows 作业对象：给子进程套上内核级上限（内存 / 进程数 / 关闭即杀）。
//! 这不是文件系统沙箱——它只保证「跑飞了也拖不垮机器、退出即收尸」。

/// 作业限制档位。
#[derive(Debug, Clone, Copy)]
pub struct JobLimits {
    /// 单进程内存上限（字节）
    pub memory_bytes: usize,
    /// 作业内进程数上限
    pub max_processes: u32,
}

impl Default for JobLimits {
    fn default() -> Self {
        Self {
            memory_bytes: 1024 * 1024 * 1024,
            max_processes: 16,
        }
    }
}

#[cfg(windows)]
mod imp {
    use super::JobLimits;
    use windows_sys::Win32::Foundation::{CloseHandle, HANDLE};
    use windows_sys::Win32::System::JobObjects::{
        AssignProcessToJobObject, CreateJobObjectW, JobObjectExtendedLimitInformation,
        SetInformationJobObject, JOBOBJECT_EXTENDED_LIMIT_INFORMATION,
        JOB_OBJECT_LIMIT_ACTIVE_PROCESS, JOB_OBJECT_LIMIT_JOB_MEMORY,
        JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, JOB_OBJECT_LIMIT_PROCESS_MEMORY,
    };
    use windows_sys::Win32::System::Threading::{
        OpenProcess, PROCESS_SET_QUOTA, PROCESS_TERMINATE,
    };

    /// 作业句柄：活着就约束范围内的进程，Drop 时内核杀光残留并回收。
    pub struct Job {
        handle: HANDLE,
    }

    impl Job {
        /// 建一个带上限的作业。
        pub fn create(limits: JobLimits) -> Option<Self> {
            unsafe {
                let handle = CreateJobObjectW(std::ptr::null(), std::ptr::null());
                if handle.is_null() {
                    return None;
                }
                let mut info: JOBOBJECT_EXTENDED_LIMIT_INFORMATION = std::mem::zeroed();
                info.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
                    | JOB_OBJECT_LIMIT_PROCESS_MEMORY
                    | JOB_OBJECT_LIMIT_JOB_MEMORY
                    | JOB_OBJECT_LIMIT_ACTIVE_PROCESS;
                info.ProcessMemoryLimit = limits.memory_bytes;
                info.JobMemoryLimit = limits.memory_bytes;
                info.BasicLimitInformation.ActiveProcessLimit = limits.max_processes;
                let ok = SetInformationJobObject(
                    handle,
                    JobObjectExtendedLimitInformation,
                    &info as *const _ as *const std::ffi::c_void,
                    std::mem::size_of::<JOBOBJECT_EXTENDED_LIMIT_INFORMATION>() as u32,
                );
                if ok == 0 {
                    CloseHandle(handle);
                    return None;
                }
                Some(Self { handle })
            }
        }

        /// 把已启动的进程收进作业；失败返回 false（仍可继续跑，只是没上限）。
        pub fn assign(&self, pid: u32) -> bool {
            unsafe {
                let process = OpenProcess(PROCESS_SET_QUOTA | PROCESS_TERMINATE, 0, pid);
                if process.is_null() {
                    return false;
                }
                let ok = AssignProcessToJobObject(self.handle, process);
                CloseHandle(process);
                ok != 0
            }
        }
    }

    impl Drop for Job {
        fn drop(&mut self) {
            // KILL_ON_JOB_CLOSE：最后一个句柄关掉时内核杀光作业内所有进程
            unsafe {
                CloseHandle(self.handle);
            }
        }
    }

    // 句柄本身是内核对象的所有权凭据，只被本结构独占持有；
    // 跨线程转移所有权是安全的（Win32 句柄可在线程间使用）。
    unsafe impl Send for Job {}
    unsafe impl Sync for Job {}
}

#[cfg(not(windows))]
mod imp {
    use super::JobLimits;

    /// 非 Windows 无作业对象：保留同名接口，限制不生效。
    pub struct Job;

    impl Job {
        /// 空实现。
        pub fn create(_limits: JobLimits) -> Option<Self> {
            None
        }

        /// 空实现。
        pub fn assign(&self, _pid: u32) -> bool {
            false
        }
    }
}

pub use imp::Job;
