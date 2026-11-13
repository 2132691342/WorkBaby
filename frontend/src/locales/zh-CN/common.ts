// 应用壳 / 标题栏 / 导航 / 通用短语。
export default {
app: {
    starting: '正在启动本地服务…',
    closePromptTitle: '关闭窗口时要做什么？',
    closePromptBody: '收进托盘：应用继续在后台跑，对话不会中断，可以从托盘图标叫回来。\n退出：直接结束应用与所有引擎进程。\n\n这个选择会被记住，以后可以在设置里改。',
    closeToTray: '收进托盘',
    closeToQuit: '退出应用',
    failTag: '启动失败',
    failHint: '后端没有起来。先看日志里最后几行；如果是端口或数据库被占用，重启本应用通常能解决。'
  },

net: {
    requestFailed: '请求失败',
    timeout: '后端响应超时',
    unreachable: '连不上后端'
  },

titlebar: {
    starting: '启动中',
    detecting: '检测中',
    noEngine: '引擎未安装',
    connected: '已连接',
    minimize: '最小化',
    maximize: '最大化'
  },

nav: {
    overview: '概览',
    chat: '对话',
    workspace: '工作区',
    sessions: '会话',
    models: '模型',
    skills: '技能',
    mcp: 'MCP',
    web: '联网',
    index: '代码',
    usage: '用量',
    apps: '应用',
    settings: '设置'
  },

common: {
    save: '保存',
    cancel: '取消',
    delete: '删除',
    edit: '编辑',
    close: '关闭',
    remove: '移除',
    refresh: '刷新',
    retry: '重试',
    create: '新建',
    install: '安装',
    uninstall: '卸载',
    loading: '加载中…',
    empty: '这里还没有内容',
    dismiss: '关闭这条提示',
    clear: '清空',
    enabled: '已启用',
    disabled: '已停用',
    copy: '复制',
    copied: '已复制',
    undo: '撤销',
    searchPlaceholder: '搜索',
    name: '名称',
    actions: '操作',
    unknown: '未知',
    untitled: '未命名',
    saved: '已保存',
    update: '升级',
    yes: '是'
  },

palette: {
    placeholder: '搜页面、动作、会话或命令…',
    empty: '没有匹配项',
    groupNav: '页面',
    groupAction: '动作',
    groupSession: '最近会话',
    groupCommand: '命令',
    newSession: '新建会话',
    newSessionHint: '开始一段新的对话',
    toggleTheme: '切换主题',
    toDark: '切到深色',
    toLight: '切到浅色',
    toggleLocale: '切换语言'
  },

templates: {
    title: '提示词模板',
    subtitle: '存一段可复用的提示词，在输入框里用 /名字 调出后填入',
    empty: '还没有模板，点右上角新建一个',
    create: '新建模板',
    edit: '编辑模板',
    name: '模板名（斜杠命令名）',
    nameHint: '如 review、commit（小写字母/数字/连字符）',
    desc: '一句话说明',
    descHint: '这条模板是干什么的',
    args: '参数提示（可空）',
    argsHint: '如 <文件> <风格>',
    body: '提示词正文',
    bodyHint: '选中后整段填进输入框的内容',
    needNameBody: '模板名与正文不能为空',
    deleteConfirm: '确定删除模板 {name} 吗？'
  },
}
