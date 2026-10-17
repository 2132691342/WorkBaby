<script setup lang="ts">
// 行为面板：助手怎么做事、怎么待着（执行方式 / 工作目录 / 驻留自启 / 发送键 / 保留期）。
import { computed, onMounted, ref, watch } from 'vue'
import * as api from '../../api'
import type { ModelCapability, RuntimeInfo } from '../../types/api'
import { useSessionStore } from '../../stores/session'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import { fmtCount } from '../../utils/num'
import { AutoStartEnabled, OpenDirectoryDialog, SetAutoStart } from '../../../wailsjs/go/main/App'
import AppIcon from '../common/AppIcon.vue'

const store = useSettingsStore()
const toast = useToastStore()
const session = useSessionStore()

// 正在保存的设置键：同一时刻只允许一个写入，防连点把状态点乱。
const busyKey = ref('')

// setValue 失败已弹过 toast，这里吞掉避免未捕获异常；busyKey 用 `key:value` 区分同一键的多个目标。
async function applySetting(key: string, value: string) {
  if (busyKey.value) return
  busyKey.value = `${key}:${value}`
  try {
    await store.setValue(key, value)
  } catch {
    /* 已提示 */
  } finally {
    busyKey.value = ''
  }
}

// 开关转发事件时会给出目标控件，键盘操作也要能切换。
function onSwitch(key: string, on: boolean) {
  void applySetting(key, on ? 'true' : 'false')
}

const autoStart = ref(false)
const autoStartErr = ref('')
const runtimeInfo = ref<RuntimeInfo | null>(null)
const runtimeErr = ref('')
const cap = ref<ModelCapability | null>(null)
const winInput = ref('')
const winErr = ref('')

// 输出预算被厂商硬上限钳住（只在窗口来自目录时可能发生）：读起来像算错了，得说明一句
const clampedByVendor = computed(
  () => !!cap.value && cap.value.max_output < Math.floor(cap.value.context_window / 8),
)

// 权限档取值必须与后端 domain.Permission* 一致，否则设置写入直接被拒。
// 这三个档位由 WorkBaby 的审批扩展实行（specs/19）。
const PERMISSIONS = [
  { key: 'ask_write', name: '先问我', desc: '写文件、跑命令都先问我一句，推荐新手使用' },
  { key: 'auto', name: '全自动', desc: '不再询问，直接执行（只建议在自己电脑上用）' },
  { key: 'readonly', name: '只读', desc: '只看不改：写文件与跑命令一律不执行' },
]

const permission = () => store.values['permission'] || 'ask_write'
const minimizeToTray = () => store.values['minimize_to_tray'] !== 'false'
const sendOnEnter = () => store.values['send_on_enter'] !== 'false'

async function loadAutoStart() {
  try {
    autoStart.value = await AutoStartEnabled()
  } catch {
    autoStart.value = false
  }
}

async function toggleAutoStart() {
  if (busyKey.value) return
  busyKey.value = `auto_start:${!autoStart.value}`
  autoStartErr.value = ''
  const next = !autoStart.value
  autoStart.value = next // 先翻转：开关的点亮必须是瞬时的
  try {
    await SetAutoStart(next)
    autoStart.value = await AutoStartEnabled()
    toast.ok(next ? '已开启开机自动启动' : '已关闭开机自动启动')
  } catch (e) {
    autoStartErr.value = (e as Error)?.message || '设置失败，请重启后再试一次'
    autoStart.value = await AutoStartEnabled().catch(() => !next)
  } finally {
    busyKey.value = ''
  }
}

async function loadRuntime() {
  runtimeErr.value = ''
  try {
    runtimeInfo.value = await api.runtime.status()
  } catch (e) {
    runtimeErr.value = (e as Error)?.message || '读取运行时状态失败'
  }
}

// 重新检测要有可见反馈：清缓存重探测，首次解压失败修好后这一次点击就能重试。
const checking = ref(false)
async function redetect() {
  if (checking.value) return
  checking.value = true
  runtimeErr.value = ''
  try {
    runtimeInfo.value = await api.runtime.redetect()
    toast.ok('已重新检测内置运行时')
  } catch (e) {
    runtimeErr.value = (e as Error)?.message || '重新检测失败'
  } finally {
    checking.value = false
  }
}

// 运行时的三种状态：内置就绪 / 用系统已装的那份 / 未就绪
function rtLed(source: string) {
  // npm-global / env 都算就绪（只是来源不同）。
  return source === 'bundled' || source === 'npm-global' || source === 'env'
    ? 'g'
    : source === 'system'
      ? 'w'
      : 'r'
}
function rtText(source: string, version: string) {
  if (source === 'bundled') return `内置就绪 · ${version}`
  if (source === 'npm-global') return `已安装在内置目录 · ${version}`
  if (source === 'env') return `已指定 · ${version}`
  if (source === 'system') return '使用系统已安装的版本'
  return '未就绪'
}

// 当前模型的上下文窗口：本地没资料就报「未知」并允许手填，不拿猜出来的数字冒充权威。
// provider_id 必传：模型级配置按「服务 + 模型」存，不传查不到用户填过的值。
async function loadCapability() {
  const model = store.boot?.default_model || store.values['default_model']
  if (!model) return
  const pid = store.boot?.default_provider_id || undefined
  try {
    cap.value = await api.models.capability(model, pid)
    winInput.value = store.values['context_window'] || ''
  } catch (e) {
    // 能力读不到时输入框会以空白按「跟随默认」呈现，不给提示会被误读成「这个模型没有上限」
    cap.value = null
    toast.bad(`读不到这个模型的能力信息：${(e as Error)?.message || '请检查模型服务'}`)
  }
}

const pickingWs = ref(false)
async function pickWorkspace() {
  if (pickingWs.value) return
  const dir = await OpenDirectoryDialog('选择工作目录')
  if (!dir) return
  pickingWs.value = true
  try {
    await store.setValue('workspace', dir)
    // 当前会话也要切：只写全局默认值的话，正在聊的这个会话还挂在旧目录上，
    // 输入框那枚芯片和实际工作目录会给出两套答案。
    const cur = session.currentId
    if (cur) {
      try {
        await api.sessions.setWorkspace(cur, dir)
      } catch (e) {
        toast.bad(`全局默认值已换，当前会话没换成：${(e as Error)?.message || '请重试'}`)
        return
      }
    }
    await store.loadBoot()
    toast.ok('工作目录已更换')
  } catch {
    /* setValue 已提示 */
  } finally {
    pickingWs.value = false
  }
}

// ---- 高级护栏：默认收起。这三项都直接写进 PI 的 settings.json。
// 恢复默认 = 写空值（后端 getInt 对空值回落到 PI 自己的缺省）。
// 步数上限 / 工具并发 / 上游等待已删除：它们是内核参数，现在由 PI 管，
// 在这边设一份没有协议面可写的值只会得到一个「看起来生效」的假开关。
const ADV_FIELDS = [
  { key: 'compaction_reserve_tokens', name: '压缩触发余量', def: 16384, min: 1024, max: 500000, hint: '上下文剩余低于这个数就开始整理。调大更早整理（上下文更清爽）。' },
  { key: 'compaction_keep_recent', name: '压缩后保留量', def: 20000, min: 2000, max: 200000, hint: '整理时最近这么多 token 原样留着。调大能让助手记得更多上下文。' },
  { key: 'context_window', name: '上下文窗口', def: 0, min: 0, max: 2000000, hint: '手填覆盖内置目录的窗口大小。0 = 跟随目录；代理改名的模型靠它认窗口。' },
] as const

const advOpen = ref(false)
const advInput = ref<Record<string, string>>({})
const advErr = ref('')
const advSaving = ref(false)

// 展开时把当前值（或空 = 跟随缺省）填进输入框，收起再开不会带上未保存的草稿。
watch(advOpen, (open) => {
  if (!open) return
  advErr.value = ''
  const next: Record<string, string> = {}
  for (const f of ADV_FIELDS) next[f.key] = store.values[f.key] || ''
  advInput.value = next
})

function onAdvInput(key: string, e: Event) {
  const el = e.target as HTMLInputElement
  advInput.value = { ...advInput.value, [key]: el.value }
}

// 恢复默认必须真的写库：只清输入框的话用户以为已经恢复，
// 下次打开看到的还是旧值——这是一枚「看起来有效」的死按钮。
const advResetting = ref(false)
async function resetAdv() {
  if (advResetting.value || advSaving.value) return
  advResetting.value = true
  advErr.value = ''
  try {
    for (const f of ADV_FIELDS) await store.setValue(f.key, '')
    advInput.value = {}
    toast.ok('已恢复默认，下一轮对话生效')
  } catch {
    /* setValue 已提示 */
  } finally {
    advResetting.value = false
  }
}

async function saveAdv() {
  if (advSaving.value) return
  advErr.value = ''
  const write: Array<[string, string]> = []
  for (const f of ADV_FIELDS) {
    const raw = (advInput.value[f.key] ?? '').trim()
    if (raw === '') {
      write.push([f.key, ''])
      continue
    }
    const n = Number(raw)
    if (!Number.isInteger(n) || n < f.min || n > f.max) {
      advErr.value = `${f.name}要填 ${f.min}–${f.max} 之间的整数`
      return
    }
    write.push([f.key, String(n)])
  }
  advSaving.value = true
  try {
    for (const [k, v] of write) await store.setValue(k, v)
    toast.ok('已保存，下一轮对话生效')
    advOpen.value = false
  } catch {
    /* setValue 已提示 */
  } finally {
    advSaving.value = false
  }
}

// ---- 执行记录保留期与归档 ----
// 归档丢的是「逐字对话」：结论、动作时间线、用量账都留着，仪表盘不受影响。
const retInput = ref('')
const retSaving = ref(false)
const archiving = ref(false)
const retErr = ref('')

// 正在输入时不回填：设置同步回来会把用户刚敲的数字盖掉。
const retFocused = ref(false)
watch(
  () => store.values['orchestra_retention_days'],
  (v) => {
    if (!retFocused.value) retInput.value = v || '30'
  },
  { immediate: true },
)

async function saveRetention() {
  if (retSaving.value) return
  retErr.value = ''
  const n = Number(retInput.value.trim())
  if (!Number.isInteger(n) || n < 0 || n > 3650) {
    retErr.value = '请填 0 到 3650 之间的整数（0 = 永久保留）'
    return
  }
  retSaving.value = true
  try {
    await store.setValue('orchestra_retention_days', String(n))
    retInput.value = String(n)
    toast.ok(n === 0 ? '执行记录改为永久保留' : `超过 ${n} 天的逐字对话会自动归档`)
  } catch {
    /* setValue 已提示 */
  } finally {
    retSaving.value = false
  }
}

async function archiveNow() {
  if (archiving.value) return
  archiving.value = true
  try {
    const res = await api.archive.now()
    if (res.retention_days === 0) toast.info('保留期是永久，不需要归档')
    else if (!res.sessions) toast.info('没有到期要归档的记录')
    else toast.ok(`已归档 ${res.sessions} 段逐字对话，${res.detached_runs} 条执行记录只留结论与时间线`)
  } catch (e) {
    toast.bad(`归档失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    archiving.value = false
  }
}

const savingWindow = ref(false)
async function saveWindow() {
  if (savingWindow.value) return
  winErr.value = ''
  const v = winInput.value.trim()
  if (!v) {
    savingWindow.value = true
    try {
      await store.setValue('context_window', '')
      cap.value && (cap.value.known = false)
      toast.ok('已清除手填窗口，跟随内置目录')
    } catch {
      /* 已提示 */
    } finally {
      savingWindow.value = false
    }
    return
  }
  const n = Number(v)
  if (!Number.isFinite(n) || n <= 0) {
    winErr.value = '请填一个大于 0 的数字'
    return
  }
  savingWindow.value = true
  try {
    await store.setValue('context_window', String(Math.floor(n)))
    await loadCapability()
    toast.ok('上下文窗口已保存')
  } catch {
    /* 已提示 */
  } finally {
    savingWindow.value = false
  }
}

onMounted(async () => {
  await Promise.all([loadAutoStart(), loadRuntime()])
})

// 引导数据可能晚于组件挂载（父组件 onMounted 在子组件之后）：跟着 boot 走一遍，否则整块读数会空着。
watch(
  () => store.boot?.default_model,
  (model) => {
    if (model) void loadCapability()
  },
  { immediate: true },
)
</script>

<template>
  <div class="bp">
    <div class="card p-sm">
      <h3>动手前先问我</h3>
      <p class="hint">助手做「危险」的事之前要不要先问你。改了立刻对下一轮生效。</p>
      <div class="perm-row">
        <button
          v-for="o in PERMISSIONS"
          :key="o.key"
          class="perm"
          :class="{
            'is-on': permission() === o.key,
            // 三张卡共用一个 busyKey：任一写入期间，三张都灰；这一张加 is-loading 转圈
            // 表明「哪张在被点」。用户在等的时候知道自己的点击有没有被处理。
            'is-loading': busyKey === `permission:${o.key}`,
          }"
          type="button"
          :disabled="!!busyKey"
          @click="applySetting('permission', o.key)"
        >
          <b>{{ o.name }}</b>
          <span>{{ o.desc }}</span>
        </button>
      </div>
    </div>

    <div class="card p-sm">
      <h3>工作目录</h3>
      <p class="hint">助手能读写的范围就是这里，界面上所有文件都相对它解析。</p>
      <div class="ws-row">
        <code class="ws-path">{{ store.values['workspace'] || store.boot?.workspace || '未设置' }}</code>
        <button
          class="btn btn-sm btn-primary"
          type="button"
          :class="{ 'is-loading': pickingWs }"
          :disabled="pickingWs"
          @click="pickWorkspace"
        >
          <AppIcon name="folder" size="ic-xs" /> 更换
        </button>
      </div>
    </div>

    <div class="card p-sm">
      <h3>执行记录保留期</h3>
      <p class="hint">
        团队与任务每次执行都留下一份结论和动作时间线；跑完之后的原始逐字对话，
        超过保留期就在启动或执行收尾时自动清掉。用量统计、评估记录和时间线都不受影响。0 = 永久保留。
      </p>
      <div class="ret-row">
        <label class="ret-field">
          <span>保留</span>
          <input
            v-model="retInput"
            class="input ret-input"
            type="number"
            min="0"
            max="3650"
            aria-label="保留天数"
            @focus="retFocused = true"
            @blur="retFocused = false"
            @keyup.enter="saveRetention"
          />
          <span>天</span>
        </label>
        <button
          class="btn btn-sm btn-primary"
          type="button"
          :class="{ 'is-loading': retSaving }"
          :disabled="retSaving"
          @click="saveRetention"
        >
          保存
        </button>
        <button
          class="btn btn-sm btn-ghost"
          type="button"
          :class="{ 'is-loading': archiving }"
          :disabled="archiving"
          title="现在就按保留期清一次"
          @click="archiveNow"
        >
          <AppIcon name="refresh" size="ic-xs" /> 立即归档
        </button>
      </div>
      <p v-if="retErr" class="hint err">{{ retErr }}</p>
    </div>

    <div class="card p-sm">
      <h3>后台与启动</h3>
      <label class="row">
        <span class="grow">
          <b>关闭窗口时收进托盘</b>
          <span>关掉窗口后助手仍在后台待命，右下角托盘图标可以再唤起</span>
        </span>
        <button
          class="switch"
          type="button"
          role="switch"
          :aria-checked="minimizeToTray()"
          :class="{ 'is-on': minimizeToTray(), 'is-loading': busyKey.startsWith('minimize_to_tray:') }"
          :disabled="busyKey.startsWith('minimize_to_tray:')"
          @click="onSwitch('minimize_to_tray', !minimizeToTray())"
        />
      </label>
      <label class="row">
        <span class="grow">
          <b>开机自动启动</b>
          <span>开机登录后自动在后台待命，要用时点右下角托盘图标</span>
        </span>
        <button
          class="switch"
          type="button"
          role="switch"
          :aria-checked="autoStart"
          :class="{ 'is-on': autoStart, 'is-loading': busyKey.startsWith('auto_start:') }"
          :disabled="busyKey.startsWith('auto_start:')"
          @click="toggleAutoStart"
        />
      </label>
      <p v-if="autoStartErr" class="hint err">{{ autoStartErr }}</p>
    </div>

    <div class="card p-sm">
      <h3>对话</h3>
      <label class="row">
        <span class="grow">
          <b>回车发送</b>
          <span>关闭后回车换行，发送要用 Ctrl+Enter</span>
        </span>
        <button
          class="switch"
          type="button"
          role="switch"
          :aria-checked="sendOnEnter()"
          :class="{ 'is-on': sendOnEnter(), 'is-loading': busyKey.startsWith('send_on_enter:') }"
          :disabled="busyKey.startsWith('send_on_enter:')"
          @click="onSwitch('send_on_enter', !sendOnEnter())"
        />
      </label>

      <div class="cap" v-if="cap">
        <div class="cap-head">
          <AppIcon name="brain" size="ic-sm" />
          <b>当前模型：{{ cap.id }}</b>
        </div>
        <div class="cap-grid">
          <span>上下文窗口</span>
          <span>
            <template v-if="cap.known">
              <b :title="String(cap.context_window)">{{ fmtCount(cap.context_window) }}</b>
            </template>
            <template v-else><i class="unknown">未知（按 {{ fmtCount(cap.context_window) }} 估算）</i></template>
          </span>
          <span>最大输出</span>
          <span>
            {{ fmtCount(cap.max_output) }}
            <!-- 窗口来自目录时才会被厂商上限钳住；钳过就说清楚，别让用户以为算错了 -->
            <em v-if="clampedByVendor" class="cap-note">已按厂商上限收紧</em>
          </span>
          <span>支持思考</span>
          <span>{{ cap.thinking ? '是' : '否' }}</span>
          <span>支持识图</span>
          <span>{{ cap.vision ? '是' : '否' }}</span>
        </div>
        <p v-if="cap.note && !cap.known" class="hint">{{ cap.note }}</p>
        <div class="cap-fix">
          <input
            v-model="winInput"
            class="inp"
            type="number"
            min="1024"
            step="1024"
            placeholder="手填上下文窗口（如 200000）"
          />
          <button
            class="btn btn-sm btn-outline"
            type="button"
            :class="{ 'is-loading': savingWindow }"
            :disabled="savingWindow"
            @click="saveWindow"
          >
            保存
          </button>
        </div>
        <p v-if="winErr" class="hint err">{{ winErr }}</p>
      </div>
    </div>

    <div class="card p-sm">
      <!-- 高级护栏：默认收起。新手改错这里不会炸（后端对非法值回落缺省），
           但收起来能保证「看不见 = 不用管」。 -->
      <button
        class="adv-head"
        type="button"
        :aria-expanded="advOpen"
        aria-controls="adv-body"
        @click="advOpen = !advOpen"
      >
        <AppIcon name="sliders" size="ic-sm" />
        <b>高级</b>
        <span class="adv-sub">一般不用改</span>
        <AppIcon class="chev" :name="advOpen ? 'chevron-down' : 'chevron-right'" size="ic-xs" />
      </button>
      <div v-if="advOpen" id="adv-body" class="adv-body">
        <label v-for="f in ADV_FIELDS" :key="f.key" class="row">
          <span class="grow">
            <b>{{ f.name }}</b>
            <span>{{ f.hint }}</span>
          </span>
          <input
            class="inp inp-num"
            type="number"
            :min="f.min"
            :max="f.max"
            :value="advInput[f.key] ?? ''"
            :placeholder="String(f.def)"
            @input="onAdvInput(f.key, $event)"
          />
        </label>
        <p v-if="advErr" class="hint err">{{ advErr }}</p>
        <div class="adv-acts">
          <button
            class="btn btn-sm btn-ghost"
            type="button"
            :class="{ 'is-loading': advResetting }"
            :disabled="advSaving || advResetting"
            @click="resetAdv"
          >
            全部恢复默认
          </button>
          <button
            class="btn btn-sm btn-primary"
            type="button"
            :class="{ 'is-loading': advSaving }"
            :disabled="advSaving"
            @click="saveAdv"
          >
            保存
          </button>
        </div>
      </div>
    </div>

    <div class="card p-sm">
      <h3>内置运行时</h3>
      <p class="hint">Node / Python 随程序分发、首次启动自动解压。助手引擎（PI）靠内置的 Node 跑。</p>
      <div v-if="runtimeErr" class="hint err">{{ runtimeErr }}</div>
      <div v-else-if="runtimeInfo" class="kv">
        <dt>助手引擎</dt>
        <dd>
          <span class="led" :class="rtLed(runtimeInfo.pi_source)" />
          <span>{{ rtText(runtimeInfo.pi_source, runtimeInfo.pi_version) }}</span>
        </dd>
        <template v-if="!runtimeInfo.pi_exe">
          <dt>PI 原因</dt>
          <dd class="why">{{ runtimeInfo.pi_error || '未知' }}</dd>
        </template>
        <dt>Node</dt>
        <dd>
          <span class="led" :class="rtLed(runtimeInfo.node_source)" />
          <span>{{ rtText(runtimeInfo.node_source, runtimeInfo.node_version) }}</span>
        </dd>
        <template v-if="!runtimeInfo.node_exe">
          <dt>Node 原因</dt>
          <dd class="why">{{ runtimeInfo.node_error || '未知' }}</dd>
        </template>
        <dt>Python</dt>
        <dd>
          <span class="led" :class="rtLed(runtimeInfo.python_source)" />
          <span>{{ rtText(runtimeInfo.python_source, runtimeInfo.python_version) }}</span>
        </dd>
        <template v-if="!runtimeInfo.python_exe">
          <dt>Python 原因</dt>
          <dd class="why">{{ runtimeInfo.python_error || '未知' }}</dd>
        </template>
      </div>
      <button
        class="btn btn-sm btn-ghost act"
        type="button"
        :class="{ 'is-loading': checking }"
        :disabled="checking"
        @click="redetect"
      >
        <AppIcon v-if="!checking" name="refresh" size="ic-xs" />
        {{ checking ? '检测中…' : '重新检测' }}
      </button>
    </div>

    <div class="card p-sm">
      <h3>关于</h3>
      <div class="kv">
        <dt>版本</dt>
        <dd class="mono">{{ store.boot?.version || '—' }}</dd>
        <dt>日志</dt>
        <dd class="mono">%APPDATA%\WorkBaby\logs\app.log</dd>
      </div>
    </div>
  </div>
</template>

<style scoped>
.bp {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
  min-width: 0;
}
.hint {
  color: var(--wb-muted);
  font-size: var(--wb-fs-sm);
  margin: var(--wb-sp-2) 0 var(--wb-sp-3);
}
.hint.err {
  color: var(--wb-danger);
  margin-bottom: 0;
}
/* .perm-row / .perm 是共享选择卡，已收回到 wb-ui.css */
.row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-3) 0;
  cursor: pointer;
  border-top: 1px solid var(--wb-line);
}
.row:first-of-type {
  border-top: none;
}
.row .grow {
  display: grid;
  gap: 2px;
  min-width: 0;
}
.row b {
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}
.row span {
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.ws-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  min-width: 0;
}
/* 保留期那一行与「工作目录」同一读法：读数在左，动作在右 */
.ret-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex-wrap: wrap;
}
.ret-field {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-2);
}
.ret-input {
  width: 84px;
  height: var(--wb-ctl-h-sm);
  text-align: center;
  font-family: var(--font-mono);
}
.ws-path {
  flex: 1;
  min-width: 0;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-2);
  padding: var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  background: var(--wb-raise);
  border: 1px solid var(--wb-line);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.cap {
  margin-top: var(--wb-sp-2);
  padding: var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  border: 1px solid var(--wb-line);
  background: var(--wb-surface-2);
  display: grid;
  gap: var(--wb-sp-2);
}
.cap-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  color: var(--wb-ink);
  font-size: var(--wb-fs-sm);
}
.cap-grid {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: var(--wb-sp-1) var(--wb-sp-3);
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-2);
}
.cap-grid > span:nth-child(odd) {
  color: var(--wb-muted);
}
.unknown {
  color: var(--wb-warning);
  font-style: italic;
}
.cap-note {
  margin-left: var(--wb-sp-2);
  font-style: normal;
  font-size: var(--wb-fs-xs);
  color: var(--wb-warning);
}
.cap-fix {
  display: flex;
  gap: var(--wb-sp-2);
  min-width: 0;
}
.inp {
  flex: 1;
  min-width: 0;
  height: var(--wb-ctl-h);
  padding: 0 var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  border: 1px solid var(--wb-border-strong);
  background: var(--wb-surface);
  color: var(--wb-ink);
  font-size: var(--wb-fs-sm);
  font-family: var(--font-mono);
}
.inp:focus {
  outline: none;
  border-color: var(--wb-primary);
  box-shadow: 0 0 0 3px var(--wb-primary-soft);
}
.act {
  margin-top: var(--wb-sp-2);
}
.why {
  color: var(--wb-danger);
  word-break: break-all;
}
/* ---- 高级折叠 ---- */
.adv-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  padding: var(--wb-sp-1) 0;
  color: var(--wb-ink);
  text-align: left;
  border-radius: var(--wb-radius-sm);
  transition: color var(--wb-dur-fast) var(--wb-ease), transform var(--wb-dur-fast) var(--wb-ease);
}
.adv-head .ic {
  color: var(--wb-muted);
}
.adv-head b {
  font-size: var(--wb-fs-md);
}
.adv-head:hover {
  color: var(--wb-primary);
}
.adv-head:hover .ic {
  color: inherit;
}
.adv-head:active {
  transform: scale(0.985);
}
.adv-sub {
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.chev {
  margin-left: auto;
}
.adv-body {
  animation: adv-in var(--wb-dur) var(--wb-ease);
}
@keyframes adv-in {
  from {
    opacity: 0;
  }
}
.inp-num {
  width: var(--wb-w-input-sm);
  flex: none;
  text-align: right;
}
.adv-acts {
  display: flex;
  justify-content: flex-end;
  gap: var(--wb-sp-2);
  padding-top: var(--wb-sp-3);
}
</style>
