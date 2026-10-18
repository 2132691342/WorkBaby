<script setup lang="ts">
// 行为面板：工作目录、默认思考档、压缩参数、托盘与回车，以及 PI 的配置文件位置。
import { computed, onMounted, ref } from 'vue'
import * as api from '../../api'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import { OpenDirectoryDialog, SetAutoStart } from '../../../wailsjs/go/main/App'
import AppIcon from '../common/AppIcon.vue'
import PnHead from '../common/PnHead.vue'

const settings = useSettingsStore()
const toast = useToastStore()

const LEVELS = [
  { key: '', name: '跟随 PI 默认' },
  { key: 'off', name: '不思考' },
  { key: 'minimal', name: '很少' },
  { key: 'low', name: '少' },
  { key: 'medium', name: '适中' },
  { key: 'high', name: '多' },
  { key: 'xhigh', name: '很多' },
  { key: 'max', name: '最多' },
]

const reserve = ref(0)
const keep = ref(0)
const level = ref('')
const busy = ref('')
const autoStart = ref(false)

const workspace = computed(() => settings.values['workspace'] || settings.boot?.workspace || '')
const paths = computed(() => settings.piSettings)

async function pickWorkspace() {
  const dir = await OpenDirectoryDialog('选择工作目录')
  if (!dir || dir === workspace.value) return
  try {
    await settings.setValue('workspace', dir)
    await settings.loadBoot()
    toast.ok(`工作目录已切换：${dir}`)
  } catch (e) {
    toast.bad(`切换失败：${(e as Error)?.message || '请重试'}`)
  }
}

async function saveThinking() {
  busy.value = 'thinking'
  try {
    await api.settings.setThinking(level.value)
    await settings.loadPISettings()
    toast.ok('已写进 PI 的 settings.json')
  } catch (e) {
    toast.bad(`保存失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

async function saveCompaction() {
  busy.value = 'compaction'
  try {
    await api.settings.setCompaction(Number(reserve.value) || 0, Number(keep.value) || 0)
    await settings.loadPISettings()
    toast.ok('压缩参数已保存')
  } catch (e) {
    toast.bad(`保存失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

async function toggleAutoStart() {
  const next = !autoStart.value
  try {
    await SetAutoStart(next)
    autoStart.value = next
    toast.ok(next ? '已设为开机启动' : '已取消开机启动')
  } catch (e) {
    toast.bad(`设置失败：${(e as Error)?.message || '请重试'}`)
  }
}

onMounted(async () => {
  if (!settings.piSettings) await settings.loadPISettings()
  level.value = settings.piSettings?.default_thinking_level || ''
  reserve.value = settings.piSettings?.compaction_reserve || 0
  keep.value = settings.piSettings?.compaction_keep || 0
})
</script>

<template>
  <div class="panel">
    <PnHead icon="sliders" title="行为" sub="这里改的是「怎么用」：工作目录、思考深度、压缩阈值与窗口行为。" />

    <!-- 宽屏两列铺满主区，窄窗自动回单列 -->
    <div class="cols">
      <section class="card block">
        <div class="b-head"><b>工作目录</b><span class="muted">助手读写文件、跑命令都在这里面</span></div>
        <div class="row">
          <el-button size="small" @click="pickWorkspace">更换</el-button>
          <span class="sp" />
          <span class="mono path">{{ workspace || '（还没设置）' }}</span>
        </div>
      </section>

      <section class="card block">
        <div class="b-head"><b>默认思考档</b><span class="muted">写进 PI settings.json 的 defaultThinkingLevel</span></div>
        <div class="row">
          <el-select v-model="level" class="sel">
            <el-option v-for="l in LEVELS" :key="l.key" :label="l.name" :value="l.key" />
          </el-select>
          <el-button size="small" type="primary" plain :loading="busy === 'thinking'" :disabled="!!busy" @click="saveThinking">
            保存
          </el-button>
        </div>
      </section>

      <section class="card block">
        <div class="b-head"><b>压缩阈值</b><span class="muted">上下文快满时，PI 会保留最近这些 token</span></div>
        <div class="grid2">
          <el-form-item class="field">
            <template #label>预留余量（reserveTokens）</template>
            <el-input-number v-model="reserve" :min="0" :controls="false" placeholder="缺省 16384" class="w-full" />
          </el-form-item>
          <el-form-item class="field">
            <template #label>保留最近（keepRecentTokens）</template>
            <el-input-number v-model="keep" :min="0" :controls="false" placeholder="缺省 20000" class="w-full" />
          </el-form-item>
        </div>
        <div class="row">
          <span class="muted sm">填 0 表示交回 PI 的缺省值</span>
          <span class="sp" />
          <el-button size="small" type="primary" plain :loading="busy === 'compaction'" :disabled="!!busy" @click="saveCompaction">
            保存
          </el-button>
        </div>
      </section>

      <section class="card block">
        <div class="b-head"><b>窗口与输入</b></div>
        <div class="row">
        <div class="rl">
          <b>关闭时收进托盘</b>
          <span class="muted sm">关掉之后点 × 就是真的退出</span>
        </div>
        <span class="sp" />
        <el-switch :model-value="settings.values['minimize_to_tray'] !== 'false'" @change="settings.setValue('minimize_to_tray', settings.values['minimize_to_tray'] === 'false' ? 'true' : 'false')" />
      </div>
      <div class="row">
        <div class="rl">
          <b>回车发送</b>
          <span class="muted sm">关掉之后 Enter 换行、Ctrl+Enter 发送</span>
        </div>
        <span class="sp" />
        <el-switch :model-value="settings.values['send_on_enter'] !== 'false'" @change="settings.setValue('send_on_enter', settings.values['send_on_enter'] === 'false' ? 'true' : 'false')" />
      </div>
      <div class="row">
        <div class="rl">
          <b>开机启动</b>
          <span class="muted sm">登录 Windows 后自动把 WorkBaby 放到托盘</span>
        </div>
        <span class="sp" />
        <el-switch :model-value="autoStart" @change="toggleAutoStart" />
        </div>
      </section>
    </div>

    <section class="card block">
      <div class="b-head">
        <b>PI 的配置文件</b>
        <span class="muted">改这里等于改 pi 命令用的那一份，两边永远一致</span>
      </div>
      <div class="paths">
        <div v-for="(label, key) in {
          agent_dir: '配置目录',
          settings_path: 'settings.json',
          models_path: 'models.json',
          mcp_path: 'mcp.json',
          sessions_dir: '会话目录',
          skills_dir: '技能目录',
          extensions_dir: '扩展目录',
        }" :key="key" class="prow">
          <span class="pl">{{ label }}</span>
          <span class="mono pv">{{ (paths as Record<string, string> | null)?.[key] || '—' }}</span>
        </div>
      </div>
      <p class="hint">
        <AppIcon name="info" size="ic-2xs" />
        这些目录属于 PI 本身；WorkBaby 只读写它们，不另存一份。
      </p>
    </section>
  </div>
</template>

<style scoped>
.panel {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
}
.cols {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(420px, 100%), 1fr));
  gap: var(--wb-sp-3);
  align-items: start;
}
.block {
  padding: var(--wb-sp-4);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}
.b-head {
  display: flex;
  align-items: baseline;
  gap: var(--wb-sp-3);
  flex-wrap: wrap;
}
.b-head b {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
}
.rl {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.rl b {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.sp {
  flex: 1;
}
.path {
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sel {
  width: 200px;
  height: var(--wb-ctl-h-sm);
}
.grid2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--wb-sp-3);
}
.field {
  display: flex;
  flex-direction: column;
  gap: 5px;
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.paths {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.prow {
  display: grid;
  grid-template-columns: 110px 1fr;
  gap: var(--wb-sp-2);
  align-items: baseline;
}
.pl {
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.pv {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink-2);
  word-break: break-all;
}
.sm {
  font-size: var(--wb-fs-2xs);
}
.hint {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
</style>
