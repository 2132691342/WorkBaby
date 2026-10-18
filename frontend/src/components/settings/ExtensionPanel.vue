<script setup lang="ts">
// 插件面板：PI 的扩展与包。
// 扩展是「往助手脑子里加一个新动作」的东西，代价是它能执行代码——所以默认来源要说清、要能一键停掉。
import { computed, onMounted, ref } from 'vue'
import * as api from '../../api'
import { useSetupLog } from '../../composables/useSetupLog'
import { useToastStore } from '../../stores/toast'
import type { ExtensionListRESP } from '../../types/api'
import { OpenFileDialog } from '../../../wailsjs/go/main/App'
import AppIcon from '../common/AppIcon.vue'
import PageState from '../common/PageState.vue'

const toast = useToastStore()
const { lines: setupLines, begin: beginSetup, end: endSetup } = useSetupLog()

const data = ref<ExtensionListRESP | null>(null)
const loading = ref(true)
const err = ref('')
const busyPath = ref('')
const importing = ref(false)

const kindName: Record<string, string> = {
  file: '文件',
  dir: '目录',
  pattern: '通配登记',
  missing: '磁盘上找不到',
}

const entries = computed(() => data.value?.entries ?? [])
const packages = computed(() => data.value?.packages ?? [])

async function load() {
  err.value = ''
  try {
    data.value = await api.extensions.list()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

async function toggle(path: string, enabled: boolean) {
  if (busyPath.value) return
  busyPath.value = path
  err.value = ''
  try {
    await api.extensions.toggle(path, enabled)
    await load()
    toast.ok(enabled ? '插件已启用' : '插件已停用')
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busyPath.value = ''
  }
}

async function importPlugin() {
  if (importing.value) return
  err.value = ''
  const p = await OpenFileDialog('选择插件文件', '*.ts;*.js')
  if (!p) return
  importing.value = true
  try {
    const r = await api.extensions.import([p])
    if (!r.imported) {
      err.value = r.skipped[0] || '这个文件导入不了：插件是一个 .ts / .js 文件'
      return
    }
    await load()
    toast.ok('插件已导入并启用')
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    importing.value = false
  }
}

// 删除两段式确认：与技能 / 模型服务同一交互
const confirming = ref('')
let confirmTimer = 0
function askRemove(path: string) {
  if (confirming.value === path) {
    confirming.value = ''
    clearTimeout(confirmTimer)
    void remove(path)
    return
  }
  confirming.value = path
  clearTimeout(confirmTimer)
  confirmTimer = window.setTimeout(() => (confirming.value = ''), 3000)
}

async function remove(path: string) {
  if (busyPath.value) return
  busyPath.value = path
  err.value = ''
  try {
    await api.extensions.remove(path)
    await load()
    toast.ok('插件已删除')
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busyPath.value = ''
  }
}

// 装包：来源写法照 pi install（npm: / git: / 本地路径 / 裸包名）。
// 进度走 SSE 的全局通道，接口立刻返回，所以这里轮询列表直到装上或超时。
const source = ref('')
const installing = ref(false)
async function install() {
  const raw = source.value.trim()
  if (!raw) {
    err.value = '先填来源：例如 npm:@scope/pi-tools、git:github.com/owner/repo、./本地目录'
    return
  }
  if (installing.value) return
  err.value = ''
  installing.value = true
  beginSetup()
  try {
    await api.extensions.install(raw)
    // 装包是后台动作：pull 包可能要几十秒，轮询列表比空等一个请求体面
    const deadline = Date.now() + 5 * 60 * 1000
    let ok = false
    while (Date.now() < deadline) {
      await new Promise((r) => setTimeout(r, 1500))
      await load()
      if (packages.value.some((p) => p.source === raw)) {
        ok = true
        break
      }
    }
    if (ok) {
      source.value = ''
      toast.ok('插件包已装上')
    } else {
      err.value = '还没装上，看下面的输出确认卡在哪一步'
    }
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    installing.value = false
    endSetup()
  }
}

const removingPkg = ref('')
async function uninstall(pkgSource: string) {
  if (removingPkg.value) return
  err.value = ''
  removingPkg.value = pkgSource
  beginSetup()
  try {
    await api.extensions.uninstall(pkgSource)
    const deadline = Date.now() + 3 * 60 * 1000
    while (Date.now() < deadline) {
      await new Promise((r) => setTimeout(r, 1200))
      await load()
      if (!packages.value.some((p) => p.source === pkgSource)) break
    }
    toast.ok('插件包已卸载')
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    removingPkg.value = ''
    endSetup()
  }
}

onMounted(load)
</script>

<template>
  <div class="plugins">
    <div class="toolbar">
      <button
        class="btn"
        type="button"
        :class="{ 'is-loading': importing }"
        :disabled="importing"
        @click="importPlugin"
      >
        <AppIcon name="download" size="ic-xs" /> 导入插件文件
      </button>
    </div>
    <p class="hint">
      插件给助手加新动作（新工具、新命令）。它能在你的电脑上执行代码，只装你信得过的来源。
      停用只是把它从助手的清单里摘掉，文件还在。启停会让引擎重读一遍配置：正在跑的对话会被中断。
    </p>

    <p v-if="err" class="alert is-bad">{{ err }}</p>

    <PageState
      :loading="loading"
      :error="err && !entries.length ? err : ''"
      :empty="!entries.length"
      empty-title="还没有装插件"
      empty-sub="导入一个 .ts / .js 插件文件，或在下面填来源装一个插件包"
    >
      <div class="rowlist">
        <div
          v-for="e in entries"
          :key="e.path"
          class="rli"
          :class="{ 'is-off': !e.enabled }"
        >
          <div class="grow main">
            <h5>
              {{ e.name }}
              <span class="tag">{{ kindName[e.kind] || e.kind }}</span>
              <span v-if="e.builtin" class="tag is-brand">自带</span>
            </h5>
            <p class="path" :title="e.path">{{ e.path }}</p>
          </div>
          <button
            v-if="!e.builtin && e.kind !== 'pattern' && e.kind !== 'missing'"
            class="switch"
            type="button"
            role="switch"
            :aria-checked="e.enabled"
            :aria-label="e.enabled ? `停用插件 ${e.name}` : `启用插件 ${e.name}`"
            :class="{ 'is-on': e.enabled, 'is-loading': busyPath === e.path }"
            :title="e.enabled ? '点击停用' : '点击启用'"
            :disabled="busyPath === e.path"
            @click="toggle(e.path, !e.enabled)"
          />
          <button
            v-if="!e.builtin"
            class="icon-btn is-sm is-danger"
            :class="{ 'is-confirm': confirming === e.path, 'is-loading': busyPath === e.path }"
            type="button"
            :title="confirming === e.path ? '再点一次确认删除' : '删除'"
            :disabled="busyPath === e.path"
            @click="askRemove(e.path)"
          >
            <AppIcon name="trash" size="ic-xs" />
          </button>
        </div>
      </div>
    </PageState>

    <div class="card p-sm pkg">
      <h3>插件包</h3>
      <p class="hint">
        用助手引擎自己的包管理装：<code>npm:@scope/pkg</code>、<code>git:github.com/owner/repo</code>，
        或本地目录 <code>./某个目录</code>。装完需要重启对话进程让它加载，这一步自动做。
      </p>
      <div class="install-row">
        <input
          v-model="source"
          class="input grow"
          placeholder="npm:@scope/pi-tools"
          :disabled="installing"
          @keydown.enter.prevent="install"
        />
        <button
          class="btn btn-primary"
          type="button"
          :class="{ 'is-loading': installing }"
          :disabled="installing"
          @click="install"
        >
          装上
        </button>
      </div>
      <div v-if="packages.length" class="rowlist">
        <div v-for="p in packages" :key="p.source" class="rli">
          <div class="grow main">
            <h5>{{ p.source }}</h5>
            <p class="path">已登记在 settings.json</p>
          </div>
          <button
            class="btn btn-sm"
            type="button"
            :class="{ 'is-loading': removingPkg === p.source }"
            :disabled="!!removingPkg"
            @click="uninstall(p.source)"
          >
            卸载
          </button>
        </div>
      </div>
      <pre v-if="setupLines.length" class="log"><code>{{ setupLines.join('\n') }}</code></pre>
    </div>
  </div>
</template>

<style scoped>
.plugins {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
  min-width: 0;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex-wrap: wrap;
  min-width: 0;
}
.hint {
  margin: 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
/* 行内主列要能吃满宽度：flex-basis 必须 0，auto 会按最长内容（长路径）预估 */
.main {
  flex: 1 1 0;
  min-width: 0;
}
.path {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.rli.is-off .main h5 {
  color: var(--wb-muted);
}
.tag.is-brand {
  color: var(--wb-primary);
  border-color: var(--wb-primary);
}
.icon-btn.is-confirm {
  color: var(--wb-danger);
  background: var(--wb-danger-soft);
}
.pkg {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.install-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  min-width: 0;
}
.log {
  margin: 0;
  max-height: 220px;
  overflow: auto;
  padding: var(--wb-sp-3);
  border-radius: var(--wb-radius);
  background: var(--wb-surface-2);
  border: 1px solid var(--wb-border);
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  white-space: pre-wrap;
}
</style>
