<script setup lang="ts">
// 插件面板：PI 的扩展（extensions 目录 + settings.json 登记）与包（pi install）。
// 装 / 卸都走 PI 自己的 CLI，进度经 chat:setup 事件流式回传。
import { computed, onMounted, ref } from 'vue'
import * as api from '../../api'
import { useToastStore } from '../../stores/toast'
import type { ExtensionListRESP, ExtensionVO, PackageVO } from '../../types/api'
import { OpenFileDialog } from '../../../wailsjs/go/main/App'
import AppIcon from '../common/AppIcon.vue'

const toast = useToastStore()
const data = ref<ExtensionListRESP | null>(null)
const busy = ref('')
const source = ref('')
const log = ref<string[]>([])

const entries = computed(() => data.value?.entries ?? [])
const packages = computed(() => data.value?.packages ?? [])

async function reload() {
  try {
    data.value = await api.extensions.list()
  } catch (e) {
    toast.bad(`读取插件失败：${(e as Error)?.message || '请重试'}`)
  }
}

async function toggle(e: ExtensionVO) {
  if (busy.value) return
  busy.value = `t:${e.path}`
  try {
    await api.extensions.toggle(e.path, !e.enabled)
    await reload()
    toast.ok(e.enabled ? `已停用 ${e.name}` : `已启用 ${e.name}`)
  } catch (err) {
    toast.bad(`切换失败：${(err as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

async function remove(e: ExtensionVO) {
  if (busy.value) return
  busy.value = `d:${e.path}`
  try {
    await api.extensions.remove(e.path)
    await reload()
    toast.ok(`已删除 ${e.name}`)
  } catch (err) {
    toast.bad(`删除失败：${(err as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

async function importExt() {
  if (busy.value) return
  busy.value = 'import'
  try {
    const path = await OpenFileDialog('选择扩展文件', '*.ts;*.js;*.mjs')
    if (!path) return
    const r = await api.extensions.import([path])
    await reload()
    if (r.imported) toast.ok('已导入并启用')
    if (r.skipped?.length) toast.info(r.skipped.join('；'))
  } catch (e) {
    toast.bad(`导入失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

async function install() {
  const src = source.value.trim()
  if (!src || busy.value) return
  busy.value = 'install'
  log.value = []
  try {
    await api.extensions.install(src)
    toast.ok('已提交安装，进度见下方')
    source.value = ''
    // 装包是后台动作：稍等再刷一次，让列表跟上。
    window.setTimeout(() => void reload(), 3000)
  } catch (e) {
    toast.bad(`安装失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

async function uninstall(p: PackageVO) {
  if (busy.value) return
  busy.value = `u:${p.source}`
  log.value = []
  try {
    await api.extensions.uninstall(p.source)
    await reload()
    toast.ok(`已卸载 ${p.name}`)
  } catch (e) {
    toast.bad(`卸载失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

async function restart() {
  if (busy.value) return
  busy.value = 'reload'
  try {
    await api.extensions.reload()
    await reload()
    toast.ok('已重载，PI 进程已重启')
  } catch (e) {
    toast.bad(`重载失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

function onSetup(line: string) {
  log.value = [...log.value.slice(-200), line]
}

defineExpose({ onSetup })
onMounted(() => void reload())
</script>

<template>
  <div class="panel">
    <header class="pn-head">
      <div>
        <h3>插件</h3>
        <p class="pn-sub">
          包走 <span class="mono">pi install</span>（npm / git / 本地），扩展走 <span class="mono">extensions</span> 目录 + settings.json 登记。
        </p>
      </div>
      <div class="acts">
        <button class="btn" type="button" :class="{ 'is-loading': busy === 'reload' }" :disabled="!!busy" @click="restart">重载</button>
        <button class="btn" type="button" :class="{ 'is-loading': busy === 'import' }" :disabled="!!busy" @click="importExt">导入扩展</button>
      </div>
    </header>

    <section class="block card">
      <div class="b-head">
        <b>包（{{ packages.length }}）</b>
        <span class="muted">随包安装的技能、扩展、提示模板一起生效</span>
      </div>
      <div class="inst">
        <input v-model="source" class="input mono" placeholder="npm:@scope/pkg 或 git:github.com/owner/repo 或 ./本地目录" @keydown.enter="install" />
        <button class="btn btn-primary" type="button" :class="{ 'is-loading': busy === 'install' }" :disabled="!!busy || !source.trim()" @click="install">
          安装
        </button>
      </div>
      <div v-if="!packages.length" class="muted small">还没有装过包。</div>
      <div v-else class="rows">
        <div v-for="p in packages" :key="p.source" class="row">
          <span class="rk" :class="{ bad: p.missing }"><AppIcon name="database" size="ic-sm" /></span>
          <div class="rr">
            <b>{{ p.name }}</b>
            <span class="mono sm muted">{{ p.source }}</span>
          </div>
          <span v-if="p.missing" class="tag bad">目录缺失</span>
          <button class="btn btn-sm danger" type="button" :class="{ 'is-loading': busy === `u:${p.source}` }" :disabled="!!busy" @click="uninstall(p)">
            卸载
          </button>
        </div>
      </div>
    </section>

    <section class="block card">
      <div class="b-head">
        <b>扩展（{{ entries.length }}）</b>
        <span class="mono muted">{{ data?.dir || '' }}</span>
      </div>
      <div v-if="!entries.length" class="muted small">extensions 目录里还没有条目。</div>
      <div v-else class="rows">
        <div v-for="e in entries" :key="e.path" class="row">
          <span class="rk"><AppIcon name="zap" size="ic-sm" /></span>
          <div class="rr">
            <b>{{ e.name }}</b>
            <span class="mono sm muted">{{ e.path }}</span>
          </div>
          <span class="tag" :class="{ ok: e.enabled }">{{ e.enabled ? '已启用' : '未启用' }}</span>
          <button class="btn btn-sm" type="button" :class="{ 'is-loading': busy === `t:${e.path}` }" :disabled="!!busy" @click="toggle(e)">
            {{ e.enabled ? '停用' : '启用' }}
          </button>
          <button class="btn btn-sm danger" type="button" :class="{ 'is-loading': busy === `d:${e.path}` }" :disabled="!!busy" @click="remove(e)">
            删除
          </button>
        </div>
      </div>
    </section>

    <section v-if="log.length" class="block card">
      <div class="b-head"><b>输出</b></div>
      <pre class="log">{{ log.join('\n') }}</pre>
    </section>
  </div>
</template>

<style scoped>
.panel {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
}
.pn-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--wb-sp-3);
  flex-wrap: wrap;
}
.pn-head h3 {
  font-size: var(--wb-fs-lg);
  color: var(--wb-ink);
}
.pn-sub {
  margin-top: 4px;
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.acts {
  display: flex;
  gap: var(--wb-sp-2);
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
}
.b-head b {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.inst {
  display: flex;
  gap: var(--wb-sp-2);
}
.rows {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: 8px 10px;
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-radius-sm);
}
.rk {
  width: var(--wb-tile-sm);
  height: var(--wb-tile-sm);
  border-radius: var(--wb-radius-sm);
  display: grid;
  place-items: center;
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
  flex: none;
}
.rk.bad {
  background: var(--wb-danger-soft);
  color: var(--wb-danger);
}
.rr {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
.rr b {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.sm {
  font-size: var(--wb-fs-2xs);
}
.log {
  margin: 0;
  max-height: 220px;
  overflow: auto;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  line-height: 1.7;
  color: var(--wb-ink-2);
  white-space: pre-wrap;
}
.small {
  font-size: var(--wb-fs-xs);
}
</style>
