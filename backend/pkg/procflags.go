package pkg

// CREATE_NO_WINDOW：子进程不分配控制台窗口。
// 它与 SysProcAttr.HideWindow 一起用：HideWindow 管「启动时不显形」，
// 这个标志管「进程压根不要控制台」，缺一个仍可能闪一下黑框。
const createNoWindow = 0x08000000
