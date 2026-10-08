<script setup lang="ts">
// 行为面板：只管「助手怎么做事、怎么待着」的事。
// 主题字体在「外观」，两者刻意分开——用户找设置时是带着目的来的，
// 「我想让它别老弹窗确认」和「我想把界面调暗」不该挤在同一屏里互相干扰。
import { onMounted, ref } from 'vue'
import * as api from '../../api'
import type { ModelCapability, RuntimeInfo } from '../../types/api'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import { fmtCount } from '../../utils/num'
import { AutoStartEnabled, OpenDirectoryDialog, SetAutoStart } from '../../../wailsjs/go/main/App'
import AppIcon from '../common/AppIcon.vue'

const store = useSettingsStore()
const toast = useToastStore()

// setValue 失败已经弹过 toast，这里吞掉避免未捕获异常
async function applySetting(key: string, value: string) {
  try {
    await store.setValue(key, value)
  } catch {
    /* 已提示 */
  }
}

const autoStart = ref(false)
const autoStartErr = ref('')
const runtimeInfo = ref<RuntimeInfo | null>(null)
const runtimeErr = ref('')
const cap = ref<ModelCapability | null>(null)
const winInput = ref('')
const winErr = ref('')

// 权限档取值必须与后端 domain.PermissionXxx 一致，否则设置写入直接被拒。
const PERMISSIONS = [
  { key: 'ask', name: '先问我', desc: '读写文件、跑命令都先问我一句，推荐新手使用' },
  { key: 'auto_edit', name: '少打扰', desc: '改文件不问，跑命令和跑脚本仍然要审批' },
  { key: 'yolo', name: '全自动', desc: '不再询问，直接执行（只建议在自己电脑上用）' },
]

const permission = () => store.values['permission'] || 'ask'
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
  autoStartErr.value = ''
  const next = !autoStart.value
  try {
    await SetAutoStart(next)
    autoStart.value = await AutoStartEnabled()
    toast.ok(next ? '已开启开机自动启动' : '已关闭开机自动启动')
  } catch (e) {
    autoStartErr.value = (e as Error)?.message || '设置失败，请重启后再试一次'
    autoStart.value = await AutoStartEnabled().catch(() => autoStart.value)
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
  return source === 'bundled' ? 'g' : source === 'system' ? 'w' : 'r'
}
function rtText(source: string, version: string) {
  if (source === 'bundled') return `内置就绪 · ${version}`
  if (source === 'system') return '使用系统已安装的版本'
  return '未就绪'
}

// 当前模型的上下文窗口：本地没资料就显示「未知」并允许手填，
// 而不是拿一个猜出来的数字冒充权威。provider_id 必须带上——
// 模型级配置按「服务 + 模型」存储，不传就查不到用户填过的值。
async function loadCapability() {
  const model = store.boot?.default_model || store.values['default_model']
  if (!model) return
  const pid = store.boot?.default_provider_id || undefined
  try {
    cap.value = await api.models.capability(model, pid)
    winInput.value = store.values['context_window'] || ''
  } catch {
    cap.value = null
  }
}

async function pickWorkspace() {
  const dir = await OpenDirectoryDialog('选择工作目录')
  if (!dir) return
  try {
    await store.setValue('workspace', dir)
    await store.loadBoot()
    toast.ok('工作目录已更换')
  } catch {
    /* setValue 已提示 */
  }
}

async function saveWindow() {
  winErr.value = ''
  const v = winInput.value.trim()
  if (!v) {
    try {
      await store.setValue('context_window', '')
      cap.value && (cap.value.known = false)
      toast.ok('已清除手填窗口，跟随内置目录')
    } catch {
      /* 已提示 */
    }
    return
  }
  const n = Number(v)
  if (!Number.isFinite(n) || n <= 0) {
    winErr.value = '请填一个大于 0 的数字'
    return
  }
  try {
    await store.setValue('context_window', String(Math.floor(n)))
    await loadCapability()
    toast.ok('上下文窗口已保存')
  } catch {
    /* 已提示 */
  }
}

onMounted(async () => {
  await Promise.all([loadAutoStart(), loadRuntime(), loadCapability()])
})
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
          :class="{ 'is-on': permission() === o.key }"
          type="button"
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
        <button class="btn btn-sm btn-primary" type="button" @click="pickWorkspace">
          <AppIcon name="folder" size="ic-xs" /> 更换
        </button>
      </div>
    </div>

    <div class="card p-sm">
      <h3>后台与启动</h3>
      <label class="row">
        <span class="grow">
          <b>关闭窗口时收进托盘</b>
          <span>关掉窗口后助手仍在后台待命，右下角托盘图标可以再唤起</span>
        </span>
        <input
          class="wb-switch"
          type="checkbox"
          :checked="minimizeToTray()"
          @change="applySetting('minimize_to_tray', ($event.target as HTMLInputElement).checked ? 'true' : 'false')"
        />
      </label>
      <label class="row">
        <span class="grow">
          <b>开机自动启动</b>
          <span>开机登录后自动在后台待命，要用时点右下角托盘图标</span>
        </span>
        <input class="wb-switch" type="checkbox" :checked="autoStart" @change="toggleAutoStart" />
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
        <input
          class="wb-switch"
          type="checkbox"
          :checked="sendOnEnter()"
          @change="applySetting('send_on_enter', ($event.target as HTMLInputElement).checked ? 'true' : 'false')"
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
            <template v-else><i class="unknown">未知（按 128K 估算）</i></template>
          </span>
          <span>最大输出</span>
          <span>{{ fmtCount(cap.max_output) }}</span>
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
          <button class="btn btn-sm btn-outline" type="button" @click="saveWindow">保存</button>
        </div>
        <p v-if="winErr" class="hint err">{{ winErr }}</p>
      </div>
    </div>

    <div class="card p-sm">
      <h3>内置运行时</h3>
      <p class="hint">Python 与 PowerShell 随程序分发、首次启动自动解压；缺失时自动用系统里已装的。</p>
      <div v-if="runtimeErr" class="hint err">{{ runtimeErr }}</div>
      <div v-else-if="runtimeInfo" class="kv">
        <dt>Python</dt>
        <dd>
          <span class="led" :class="rtLed(runtimeInfo.python_source)" />
          <span>{{ rtText(runtimeInfo.python_source, runtimeInfo.python_version) }}</span>
        </dd>
        <template v-if="!runtimeInfo.python_exe">
          <dt>Python 原因</dt>
          <dd class="why">{{ runtimeInfo.python_error || '未知' }}</dd>
        </template>
        <dt>PowerShell</dt>
        <dd>
          <span class="led" :class="rtLed(runtimeInfo.powershell_source)" />
          <span>{{ rtText(runtimeInfo.powershell_source, runtimeInfo.powershell_version) }}</span>
        </dd>
        <template v-if="!runtimeInfo.powershell_exe">
          <dt>PS 原因</dt>
          <dd class="why">{{ runtimeInfo.powershell_error || '未知' }}</dd>
        </template>
      </div>
      <button
        class="btn btn-sm btn-ghost act"
        type="button"
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
        <dt>数据目录</dt>
        <dd class="mono">{{ store.values['workspace'] ? '见上方工作目录' : '—' }}</dd>
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
.perm-row {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--wb-sp-2);
}
@media (max-width: 860px) {
  .perm-row {
    grid-template-columns: minmax(0, 1fr);
  }
}
.perm {
  text-align: left;
  display: grid;
  gap: 2px;
  padding: var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  border: 1px solid var(--wb-border);
  background: var(--wb-surface);
  color: var(--wb-ink);
  cursor: pointer;
  min-width: 0;
  transition: border-color var(--wb-dur) var(--wb-ease), background var(--wb-dur) var(--wb-ease);
}
.perm:hover {
  border-color: var(--wb-border-strong);
}
.perm.is-on {
  border-color: var(--wb-primary);
  background: var(--wb-primary-soft);
}
.perm b {
  font-size: var(--wb-fs-md);
}
.perm span {
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
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
</style>
