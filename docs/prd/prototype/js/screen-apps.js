/* ============================================================
   屏幕：启动态 / 桌宠独立窗口 / 命令面板 / 托盘 / 设计系统总览
   ============================================================ */
(function () {
  'use strict';

  const U = window.U;
  const D = window.DATA;

  /* ============================================================
     启动态
     ============================================================ */
  function bootScreen(state) {
    const failed = state === 'error';
    return {
      id: 'boot.' + state,
      title: failed ? '启动失败' : '启动引导',
      desc: failed
        ? '后端端口未就绪时给出可见错误与重试入口，而不是白屏'
        : '窗口打开即显示真实进度：端口注入 → 库迁移 → 能力装配，三步都可读',
      shell: 'bare',
      render: function () {
        if (failed) {
          return (
            '<div class="boot">' +
            '<div class="boot-mark" style="background:var(--bad);color:#fff;box-shadow:0 0 0 10px var(--bad-soft)">' + ic('warn', 26) + '</div>' +
            '<div class="boot-t">无法连接本地服务</div>' +
            '<div class="t-sub" style="max-width:400px;text-align:center">HTTP 服务未在预期端口就绪，可能被安全软件拦截或端口被占用。' +
            '重试会重新等待 app:ready 事件并再次注入端口。</div>' +
            '<div class="boot-log" style="border-color:color-mix(in srgb, var(--bad) 40%, transparent)">' +
            '<div><span style="color:var(--bad)">[error]</span> dial 127.0.0.1:0: waiting for app:ready … timeout 8s</div>' +
            '<div>[info] data root = ' + U.esc(D.homeDir) + '</div>' +
            '<div>[info] last known port = 51234</div>' +
            '</div>' +
            '<div class="row row-g2 mt2"><button class="btn btn-primary">' + ic('refresh', 14) + '重试连接</button>' +
            '<button class="btn btn-outline">打开日志目录</button></div>' +
            '</div>'
          );
        }
        return (
          '<div class="boot">' +
          '<div class="boot-mark">' + ic('bot', 26) + '</div>' +
          '<div class="boot-t">WorkBaby 正在启动</div>' +
          '<div class="t-sub">本地优先 · 数据不出本机</div>' +
          '<div class="boot-log">' +
          '<div><b>[ok]</b> 数据目录就绪 &nbsp;' + U.esc(D.homeDir) + '</div>' +
          '<div><b>[ok]</b> SQLite 打开（WAL）· 迁移至 v12</div>' +
          '<div><b>[ok]</b> 加载模型服务 4 个 · 熔断 1 个</div>' +
          '<div><i>[..]</i> 装配能力：工具 23 · 技能 5 · MCP 3</div>' +
          '<div><i>[..]</i> 等待本地 HTTP 端口注入…</div>' +
          '</div>' +
          '<div style="width:380px">' + U.progress(62) + '</div>' +
          '</div>'
        );
      }
    };
  }

  /* ============================================================
     桌宠独立窗口
     ============================================================ */
  function petScreen() {
    return {
      id: 'pet.desktop',
      title: '桌宠窗口',
      desc: '独立透明窗口：无边框、可拖动、点击穿透按命中区域动态上报；气泡说出回复末句',
      shell: 'bare',
      render: function () {
        return (
          '<div class="desk">' +
          '<div class="desk-icons">' +
          ['回收站', 'Go', '项目', '工具'].map((n) =>
            '<div class="deskicon"><div class="di">' + ic(n === '回收站' ? 'trash' : 'folder', 15) + '</div>' + n + '</div>'
          ).join('') +
          '</div>' +
          '<div class="pet-mini">' +
          '<div class="pm-head">' + '<span style="color:var(--accent)">' + ic('bot', 14) + '</span>WorkBaby' +
          '<span class="spacer"></span>' + U.iconBtn('minus', '收起') + '</div>' +
          '<div class="pm-body">' +
          '<div class="pm-u">重构好了吗？</div>' +
          '<div class="pm-a">拆完了：新增 scorer.go 与 7 个用例，go test 全绿。要顺手把 chunker 也补上吗？<span class="caret" style="height:10px"></span></div>' +
          '</div>' +
          '<div class="pm-foot"><input class="input grow" placeholder="说点什么…" style="height:26px"><button class="send-btn" style="width:26px;height:26px">' + ic('send', 13) + '</button></div>' +
          '</div>' +
          '<div class="pet-float">' +
          '<div class="pet-bubble">go test 全绿，收工！</div>' +
          '<div class="pet-card">' +
          '<div class="pet-sprite">' +
          '<div style="width:92px;height:92px;border-radius:50%;background:radial-gradient(circle at 35% 30%, var(--accent-hi), var(--accent));display:grid;place-items:center;color:var(--accent-ink);box-shadow:0 14px 26px -12px rgba(0,0,0,.6)">' +
          ic('bot', 42) + '</div></div>' +
          '<div class="pet-chip">' + U.led('ok') + '发呆中' +
          '<span class="spacer"></span>' + ic('chat', 12) + '</div>' +
          '</div></div></div>'
        );
      }
    };
  }

  /* ============================================================
     命令面板（独立展示 + 交互）
     ============================================================ */
  function paletteScreen() {
    return {
      id: 'palette',
      title: '命令面板',
      desc: 'Ctrl+K 唤起：操作 / 权限 / 工具 / 会话 / 导航五类同屏可搜，键盘全通',
      shell: 'app',
      render: function () {
        return (
          '<div class="chat">' +
          '<div class="chat-head"><div class="ch-id"><div class="ch-name">' + U.esc(D.runScript.title) + '</div>' +
          '<div class="ch-sub"><span class="row row-g2">' + U.led('ok') + '空闲</span></div></div>' +
          '<span class="spacer"></span></div>' +
          '<div class="msgs"><div class="msgs-inner">' +
          '<div class="msg-u-wrap"><div class="msg-u">' + U.esc(D.runScript.userMsg) + '</div></div>' +
          '<div class="prose dim" style="padding-left:41px">（消息区在背后，命令面板浮在其上）</div>' +
          '</div></div>' +
          '<div class="palette-scrim">' +
          '<div class="palette">' +
          '<div class="palette-input">' + ic('search', 17) +
          '<input placeholder="输入命令、会话名或设置项…" value="压缩">' +
          U.kbd('Esc') + '</div>' +
          '<div class="palette-list">' +
          '<div class="menu-label">操作</div>' +
          U.menuItem('压缩当前会话历史', 'activity', 'is-on', '保留结论 · 释放 42%') +
          U.menuItem('清空当前会话', 'x', 'is-danger') +
          '<div class="menu-label">导航</div>' +
          U.menuItem('并行压缩设置项 …', 'settings') +
          '<div class="menu-label">会话</div>' +
          U.menuItem('分析上周的慢查询日志', 'chat', '', 'deepseek-chat') +
          U.menuItem('给 core 包补一组护栏测试', 'chat', '', 'deepseek-reasoner') +
          '</div>' +
          '<div class="palette-foot">' + U.kbd('↑↓') + ' 选择 ' + U.kbd('Enter') + ' 执行 ' +
          U.kbd('Esc') + ' 关闭<span class="spacer"></span><span>共 46 项</span></div>' +
          '</div></div></div>'
        );
      },
      mount: function (root, ctx) {
        root.addEventListener('click', (e) => {
          const item = e.target.closest('.menu-item');
          if (item) ctx.toast('执行：' + item.textContent.trim());
          if (e.target.closest('.palette-scrim') === e.target) ctx.toast('点击遮罩关闭面板');
        });
      }
    };
  }

  /* ============================================================
     托盘与系统集成
     ============================================================ */
  function trayScreen() {
    const menu =
      '<div class="tray-menu">' +
      U.menuItem('打开 WorkBaby', 'bot', 'is-on') +
      '<div class="menu-sep"></div>' +
      U.menuItem('新建任务', 'plus', '', 'Ctrl+N') +
      U.menuItem('命令面板', 'search', '', 'Ctrl+K') +
      U.menuItem('切换桌宠形态', 'paw') +
      '<div class="menu-sep"></div>' +
      '<div class="row row-g2" style="padding:4px 8px;font-size:11.5px;color:var(--ink-3)">' +
      U.led('accent is-live') + '<span>1 个任务执行中 · 第 3 轮</span></div>' +
      '<div class="menu-sep"></div>' +
      U.menuItem('退出', 'x', 'is-danger') +
      '</div>';
    return {
      id: 'tray',
      title: '托盘与系统集成',
      desc: '关闭到托盘、单实例、文件关联打开（双击文件即发起阅读任务）',
      shell: 'app',
      render: function () {
        return (
          '<div class="chat">' +
          '<div class="chat-head"><div class="ch-id"><div class="ch-name">系统集成</div>' +
          '<div class="ch-sub"><span>托盘菜单 · 单实例 IPC · 文件关联</span></div></div><span class="spacer"></span></div>' +
          '<div class="msgs"><div class="msgs-inner" style="gap:20px">' +
          '<div class="row row-g6" style="gap:40px;align-items:flex-start">' +
          '<div><div class="t-plate" style="margin-bottom:10px">托盘右键菜单</div>' + menu + '</div>' +
          '<div class="grow" style="max-width:520px">' +
          '<div class="t-plate" style="margin-bottom:10px">行为清单</div>' +
          U.alert('info', 'info', '关闭窗口 = 最小化到托盘', '只有托盘菜单的「退出」才结束进程；单实例互斥保证双击图标不会再开一个。') +
          '<div class="mt3"></div>' +
          U.alert('ok', 'check', '文件关联已注册', '双击 .md / .pdf / .docx / .xlsx 会转交给主实例：导入文件 → 新建会话 → 直接发起一次「阅读总结」。') +
          '<div class="mt3"></div>' +
          U.alert('warn', 'warn', '任务执行中关闭应用', '会先弹确认；确认后检查点写入，重新打开可从中断处继续。') +
          '</div></div>' +
          '<div class="card card-pad"><div class="row row-g3">' +
          '<span class="t-plate">通知</span><span class="faint" style="font-size:11.5px">任务完成 / 审批等待 / 更新提示，均走系统通知，不打断输入</span></div>' +
          '<div class="row row-g3 mt3 wrap">' +
          '<div class="toast" style="position:static">' + '<span style="color:var(--ok)">' + ic('check', 14) + '</span>' +
          '<span class="grow">后台任务完成：扫描依赖里的过期包</span><span class="faint">刚刚</span></div>' +
          '<div class="toast" style="position:static">' + '<span style="color:var(--warn)">' + ic('warn', 14) + '</span>' +
          '<span class="grow">有 1 个审批在等待你的决定</span><span class="faint">2 分钟前</span></div>' +
          '</div></div>' +
          '</div></div></div>'
        );
      }
    };
  }

  /* ============================================================
     设计系统总览（Kitchen Sink）
     ============================================================ */
  function kitScreen() {
    const swatch = (name, v) =>
      '<div style="flex:1;min-width:120px"><div style="height:52px;border-radius:5px;border:1px solid var(--line-2);background:var(' + v + ')"></div>' +
      '<div class="t-mono mt1" style="font-size:10.5px;color:var(--ink-2)">' + name + '</div>' +
      '<div class="t-mono" style="font-size:10px;color:var(--ink-3)">var(' + v + ')</div></div>';
    return {
      id: 'kit',
      title: '设计系统',
      desc: '一套令牌 + 一套控件语法：色板只有「石墨 + 琥珀 + 两盏灯」，字号 6 级、圆角 4 级',
      shell: 'bare',
      render: function () {
        return (
          '<div style="flex:1;overflow-y:auto;padding:32px 40px;background:var(--bg)">' +
          '<div style="max-width:1080px;margin:0 auto">' +
          '<div class="row row-g4"><div class="side-brand" style="padding:0">' +
          '<div class="logo">' + ic('bot', 17) + '</div>' +
          '<div><b style="font-size:19px">WorkBaby 设计系统</b><small>工作台语言 · 石墨 + 琥珀</small></div></div>' +
          '<span class="spacer"></span>' +
          '<button class="btn btn-outline" data-act="kit-theme">' + ic('moon', 14) + '切换主题</button></div>' +

          '<div class="t-plate mt6" style="margin-bottom:12px">01 · 色板（唯一强调色：琥珀）</div>' +
          '<div class="card card-pad"><div class="row row-g3 wrap">' +
          swatch('窗口底', '--bg') + swatch('面板', '--bg-panel') + swatch('卡片', '--bg-card') +
          swatch('悬浮', '--bg-raise') + swatch('凹陷', '--bg-sunken') + swatch('分隔线', '--line') +
          '</div><div class="row row-g3 wrap mt4">' +
          swatch('强调 / 品牌', '--accent') + swatch('强调提亮', '--accent-hi') +
          swatch('状态灯 · 通过', '--ok') + swatch('状态灯 · 注意', '--warn') + swatch('状态灯 · 出错', '--bad') +
          '</div><div class="row row-g3 wrap mt4">' +
          swatch('主文字', '--ink') + swatch('次要文字', '--ink-2') + swatch('弱化文字', '--ink-3') +
          '</div></div>' +

          '<div class="t-plate mt6" style="margin-bottom:12px">02 · 字体（3 族 6 级）</div>' +
          '<div class="card card-pad">' +
          '<div class="row row-g4" style="align-items:baseline"><span class="t-plate" style="width:80px">读数</span>' +
          '<span class="t-num" style="font-size:28px">128,406</span><span class="faint t-mono" style="font-size:11px">Bahnschrift · 28px · tabular-nums</span></div>' +
          '<div class="row row-g4 mt3" style="align-items:baseline"><span class="t-plate" style="width:80px">标题</span>' +
          '<span class="t-title">重构 RAG 检索打分</span><span class="faint t-mono" style="font-size:11px">19px / 600</span></div>' +
          '<div class="row row-g4 mt3" style="align-items:baseline"><span class="t-plate" style="width:80px">正文</span>' +
          '<span class="t-read">工作区文件已更新，可以继续下一步。</span><span class="faint t-mono" style="font-size:11px">13px / 1.7</span></div>' +
          '<div class="row row-g4 mt3" style="align-items:baseline"><span class="t-plate" style="width:80px">等宽</span>' +
          '<span class="t-mono">internal/rag/scorer.go:48</span><span class="faint t-mono" style="font-size:11px">Cascadia Code</span></div>' +
          '<div class="row row-g4 mt3" style="align-items:baseline"><span class="t-plate" style="width:80px">铭牌</span>' +
          '<span class="t-plate">TOKEN USAGE · 07</span><span class="faint t-mono" style="font-size:11px">大写 + 0.09em 字距</span></div>' +
          '</div>' +

          '<div class="t-plate mt6" style="margin-bottom:12px">03 · 控件</div>' +
          '<div class="card card-pad"><div class="row row-g3 wrap">' +
          '<button class="btn btn-primary">' + ic('play', 14) + '主操作</button>' +
          '<button class="btn btn-solid">次操作</button>' +
          '<button class="btn btn-outline">描边</button>' +
          '<button class="btn btn-ghost">幽灵</button>' +
          '<button class="btn btn-danger">危险</button>' +
          '<button class="btn btn-outline" disabled>禁用</button>' +
          '</div><div class="row row-g3 wrap mt4">' +
          U.chip('芯片', false, 'zap') + U.chip('已选中', true, 'check') +
          U.tag('只读', 'solid') + U.tag('写本地', 'warn') + U.tag('执行', 'bad') + U.tag('通过', 'ok') + U.tag('品牌', 'accent') +
          '<span class="row row-g3">' + U.led('ok') + U.led('warn') + U.led('bad') + U.led('accent is-live') + '</span>' +
          '<span class="row row-g2"><span class="ring" style="--p:34"></span><span class="t-mono" style="font-size:11px">34%</span></span>' +
          '<span class="row row-g2"><span class="ring is-warn" style="--p:72"></span><span class="t-mono" style="font-size:11px">72% 预警</span></span>' +
          '</div><div class="row row-g3 mt4" style="max-width:640px">' +
          '<input class="input" placeholder="输入框">' +
          '<select class="select" style="width:150px"><option>下拉</option></select>' +
          U.switchEl(true) + U.switchEl(false) +
          '<span class="kbd">Ctrl K</span>' +
          '</div></div>' +

          '<div class="t-plate mt6" style="margin-bottom:12px">04 · 数据表与状态</div>' +
          '<div class="card">' +
          '<table class="tbl"><thead><tr><th>工具</th><th>风险</th><th class="num">调用次数</th><th>状态</th><th></th></tr></thead><tbody>' +
          '<tr><td class="t-mono" style="font-size:12px">file_edit</td><td>' + U.tag('写本地', 'warn') + '</td><td class="num">1,204</td>' +
          '<td><span class="row row-g2">' + U.led('ok') + '正常</span></td>' +
          '<td><div class="row-act">' + U.iconBtn('eye', '查看') + U.iconBtn('trash', '删除', 'is-danger') + '</div></td></tr>' +
          '<tr><td class="t-mono" style="font-size:12px">exec</td><td>' + U.tag('执行', 'bad') + '</td><td class="num">386</td>' +
          '<td><span class="row row-g2">' + U.led('warn') + '1 次被拒</span></td>' +
          '<td><div class="row-act">' + U.iconBtn('eye', '查看') + '</div></td></tr>' +
          '</tbody></table></div>' +

          '<div class="t-plate mt6" style="margin-bottom:12px">05 · 空态 / 骨架 / 提示</div>' +
          '<div class="row row-g4" style="align-items:stretch">' +
          '<div class="card grow">' + U.empty('inbox', '还没有后台任务', '提交后会出现在这里，跑完自动通知') + '</div>' +
          '<div class="card grow card-pad">' +
          '<div class="skel" style="height:14px;width:70%"></div>' +
          '<div class="skel mt2" style="height:14px;width:90%"></div>' +
          '<div class="skel mt2" style="height:14px;width:52%"></div>' +
          '</div>' +
          '<div class="card grow card-pad">' +
          U.alert('ok', 'check', '已通过', 'go test ./internal/rag/... 0.42s') +
          '<div class="mt3">' + U.alert('bad', 'warn', '工具失败', 'exec 超时 30s，已计入失败上限（2/3）') + '</div>' +
          '</div>' +
          '</div>' +

          '<div class="t-plate mt6" style="margin-bottom:12px">06 · 动效原则</div>' +
          '<div class="card card-pad row row-g4 wrap">' +
          [['hover / 按键', '90ms'], ['面板与弹层', '140ms'], ['对话框入场', '220ms']].map((m) =>
            '<div class="preset" style="width:180px"><div class="p-name">' + m[0] + '</div>' +
            '<div class="p-url">' + m[1] + ' · cubic-bezier(.2,.6,.3,1)</div></div>'
          ).join('') +
          '<div class="t-mono faint" style="font-size:11px;flex:1;min-width:220px">不做弹跳、不做视差；动效只解释状态变化，不装饰界面。</div>' +
          '</div>' +
          '<div style="height:40px"></div>' +
          '</div></div>'
        );
      },
      mount: function (root, ctx) {
        root.addEventListener('click', (e) => {
          if (e.target.closest('[data-act="kit-theme"]')) ctx.toggleTheme();
        });
      }
    };
  }

  /* ============================================================
     工作台（看板式首页）：对齐参考稿的视觉语言
     大标题 + 玻璃球装饰 + 一屏多卡 + 每卡带图
     ============================================================ */
  function workspaceScreen() {
    const T = D.tokenTrend;
    const W = 640, H = 140, MAX = 600;
    const inV = T.map((d) => d.in), outV = T.map((d) => d.out), caV = T.map((d) => d.cache);
    const bars = [42, 58, 51, 74, 66, 92, 80];
    function linePath(vals, w, h, max) {
      return vals.map((v, i) =>
        (i ? 'L' : 'M') + (i * (w / (vals.length - 1))).toFixed(1) + ' ' + (h - (v / max) * h).toFixed(1)
      ).join(' ');
    }
    /* SVG 环：圆角端点 + 渐变描边（conic-gradient 的硬边是"廉价图表"的典型） */
    let gid = 0;
    function ringSVG(p, size) {
      const R = 42;
      const C = 2 * Math.PI * R;
      gid += 1;
      const id = 'rg' + gid;
      return '<svg class="ring-svg" viewBox="0 0 100 100" style="width:' + size + 'px;height:' + size + 'px">' +
        '<defs><linearGradient id="' + id + '" x1="0" y1="0" x2="1" y2="1">' +
        '<stop offset="0%" stop-color="#7fb3f7"/><stop offset="100%" stop-color="#2f80ed"/>' +
        '</linearGradient></defs>' +
        '<circle class="rk" cx="50" cy="50" r="' + R + '"/>' +
        '<circle class="rv" cx="50" cy="50" r="' + R + '" stroke="url(#' + id + ')" ' +
        'stroke-dasharray="' + C.toFixed(1) + '" stroke-dashoffset="' + (C * (1 - p / 100)).toFixed(1) + '" ' +
        'transform="rotate(-90 50 50)"/></svg>';
    }

    function kpi(icon, label, value, unit, hint, delta, bs) {
      const max = Math.max.apply(null, bs || bars);
      return `<div class="kpi"><div class="kpi-top"><span class="kpi-ic">${ic(icon, 16)}</span>
        <span class="kpi-label">${label}</span><span class="kpi-delta">${delta}</span></div>
        <div class="kpi-value">${value}<span class="kpi-unit">${unit}</span></div>
        <div class="kpi-foot">${U.plate(hint)}
        <div class="mini-bars">${(bs || bars).map((v) =>
          `<i class="${v === max ? 'is-peak' : ''}" style="height:${Math.round((v / max) * 100)}%"></i>`).join('')}</div>
        </div></div>`;
    }
    const bar = (label, p, n) => `<div class="bar-row"><span class="bl">${label}</span>
      <span class="bt"><i style="width:${p}%"></i></span><span class="bn">${n}</span></div>`;

    return {
      id: 'board',
      title: '工作台',
      desc: '看板式首页：一屏看到运行规模 / 消耗 / 工具 / 知识沉淀，每张卡都带图（对齐参考稿）',
      shell: 'app',
      navGroup: '工作台',
      navName: '工作台（看板）',
      render: function () {
        return (
          '<div class="scroll-y" style="flex:1;padding:24px 32px 56px">' +
          '<div style="max-width:1200px;margin:0 auto">' +

          /* 头部：大标题 + 玻璃球装饰 */
          '<div class="board-hero">' +
          '<div class="bh-deco"></div>' +
          '<div class="bh-txt">' +
          '<h1>早上好，LIKX</h1>' +
          '<p>今天已跑 9 次 run、消耗 4.2M token，1 个后台任务在跑，2 条审批已处理</p></div>' +
          '<div class="bh-actions">' +
          U.btn('新建任务', 'solid', 'plus') + U.btn('打开对话', 'primary', 'send') +
          '</div></div>' +

          /* 仪表读数条：指针化的一行读数，替代"同尺寸卡片网格" */
          '<div class="statbar">' +
          [['今日消息', D.stats.msg, '条', '较昨日 +12%', true],
           ['活跃会话', D.stats.sess, '个', '较昨日 +4', true],
           ['Token 消耗', D.stats.token, '', '缓存命中 74%', false],
           ['工具调用', '386', '次', '成功率 99%', true],
           ['知识片段', '1,242', '', '5 篇文档 · 1 篇索引中', false]].map((s) =>
            '<div class="st"><div class="sv">' + s[1] + (s[2] ? '<i>' + s[2] + '</i>' : '') + '</div>' +
            '<div class="sl">' + s[0] + '</div>' +
            '<div class="sd' + (s[4] ? ' is-up' : '') + '">' + s[3] + '</div></div>').join('') +
          '</div>' +

          /* 中部：趋势 + 环形 + 分布 */
          '<div class="board-grid" style="margin-bottom:16px">' +
          '<div class="board-card span-2">' +
          '<div class="bc-head"><span class="kpi-ic">' + ic('chart', 16) + '</span><span class="bc-t">Token 消耗</span>' +
          '<span class="spacer"></span><span class="legend"><span class="lg"><i class="sw in"></i>输入</span>' +
          '<span class="lg"><i class="sw out"></i>输出</span><span class="lg"><i class="sw cache"></i>缓存</span></span></div>' +
          '<div class="row row-g3" style="align-items:baseline;margin-bottom:6px">' +
          '<span class="t-num" style="font-size:30px;font-weight:700;letter-spacing:-.02em">4.2M</span>' +
          '<span class="t-sub">总消耗</span></div>' +
          '<svg class="chart" viewBox="0 0 ' + W + ' ' + H + '" preserveAspectRatio="none" style="height:168px">' +
          [0, 1, 2, 3].map((i) => '<line class="grid-line" x1="0" y1="' + (i * H / 3) + '" x2="' + W + '" y2="' + (i * H / 3) + '"></line>').join('') +
          '<path class="area" d="' + linePath(inV, W, H, MAX) + ' L' + W + ' ' + H + ' L0 ' + H + ' Z"></path>' +
          '<path class="ln-in" d="' + linePath(inV, W, H, MAX) + '"></path>' +
          '<path class="ln-out" d="' + linePath(outV, W, H, MAX) + '"></path>' +
          '<path class="ln-cache" d="' + linePath(caV, W, H, MAX) + '"></path></svg>' +
          '<div class="row" style="justify-content:space-between;margin-top:6px">' +
          T.map((d) => '<span class="t-mono faint" style="font-size:10px">' + d.label + '</span>').join('') + '</div></div>' +

          '<div class="board-card"><div class="bc-head"><span class="kpi-ic">' + ic('check', 16) + '</span>' +
          '<span class="bc-t">任务成功率</span></div>' +
          '<div style="display:flex;justify-content:center;padding:6px 0 10px">' +
          '<div class="ring-box">' + ringSVG(94, 96) + '<b class="ring-num">94<i>%</i></b></div></div>' +
          '<div class="row between" style="font-size:12px;color:var(--ink-2)"><span>近 50 次 run</span><span class="t-mono">+6%</span></div>' +
          '<div class="t-sub mt2">失败 3 次：工具超时 2 / 网络 1</div></div>' +

          '<div class="board-card"><div class="bc-head"><span class="kpi-ic">' + ic('cpu', 16) + '</span>' +
          '<span class="bc-t">模型使用</span></div>' +
          bar('deepseek-chat', 62, '62%') + bar('reasoner', 24, '24%') +
          bar('gpt-4o', 9, '9%') + bar('ollama', 5, '5%') +
          '<div class="t-sub mt3">本月 148 次调用 · 无熔断</div></div>' +
          '</div>' +

          /* 下部：知识 / 记忆 / 会话 / 待办 / 助手 */
          '<div class="board-grid">' +
          '<div class="board-card"><div class="bc-head"><span class="kpi-ic">' + ic('book', 16) + '</span>' +
          '<span class="bc-t">知识库</span></div>' +
          '<div style="display:flex;justify-content:center;padding:4px 0 10px">' +
          '<div class="ring-box">' + ringSVG(68, 96) + '<b class="ring-num">68<i>%</i></b></div></div>' +
          '<div class="row between" style="font-size:12px;color:var(--ink-2)"><span>已索引片段</span><span class="t-mono">1,242</span></div>' +
          '<div class="mt2">' + U.progress(68) + '</div>' +
          '<div class="t-sub mt2">1 篇索引中 · 1 篇失败</div></div>' +

          '<div class="board-card span-3"><div class="bc-head"><span class="kpi-ic">' + ic('chat', 16) + '</span>' +
          '<span class="bc-t">最近会话</span><span class="spacer"></span>' + U.btn('全部', 'ghost', null, 'sm') + '</div>' +
          D.sessions.slice(0, 4).map((s, i) =>
            '<div class="row row-g3" style="padding:7px 0;border-bottom:1px solid var(--line)">' +
            '<div class="ring-box">' + ringSVG([92, 64, 41, 18][i], 40) +
            '<b class="ring-num" style="font-size:11px">' + [92, 64, 41, 18][i] + '</b></div>' +
            '<div class="grow"><div style="font-size:12.5px">' + U.esc(s.name) + '</div>' +
            '<div class="t-mono faint" style="font-size:10.5px">' + s.time + ' · ' + s.model + '</div></div>' +
            '<span class="row row-g2" style="font-size:11px;color:var(--ink-3)">' +
            (i === 0 ? U.led('ok') : U.led('')) + (i === 0 ? '运行中' : '空闲') + '</span></div>').join('') +
          '</div>' +

          '</div></div></div>'
        );
      },
      mount: function (root, ctx) {
        root.addEventListener('click', (e) => {
          if (e.target.closest('.send-btn')) ctx.toast('已发送到当前会话（原型演示）');
          if (e.target.closest('.kpi, .board-card')) return;
        });
      }
    };
  }

  window.WB_APPS = {
    workspace: workspaceScreen,
    boot: bootScreen,
    pet: petScreen,
    palette: paletteScreen,
    tray: trayScreen,
    kit: kitScreen
  };
})();
