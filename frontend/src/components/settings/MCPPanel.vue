<script setup lang="ts">
// MCP 面板：配置读写的是 PI 的 mcp.json（全局 + 项目各一份），
// 在线状态与工具清单来自 `pi mcp list --json`——连没连上只有 PI 知道。
import { computed, onMounted, ref, watch } from 'vue'
import * as api from '../../api'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import type { MCPListRESP, MCPServerVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'
import CodeEditor from '../common/CodeEditor.vue'
import PnHead from '../common/PnHead.vue'

const toast = useToastStore()
const settingsStore = useSettingsStore()
const data = ref<MCPListRESP | null>(null)
const scope = ref<'global' | 'project'>('global')
const raw = ref('')
const busy = ref('')

const servers = computed(() => data.value?.servers ?? [])
const hasProject = computed(() => !!data.value?.project_path)

const STATE: Record<string, { label: string; cls: string }> = {
  connected: { label: '已连接', cls: 'ok' },
  error: { label: '连接失败', cls: 'bad' },
  unknown: { label: '未探测', cls: '' },
}

async function reload() {
  try {
    data.value = await api.mcp.list()
    settingsStore.mcp = data.value
    raw.value = scope.value === 'global' ? data.value.global_raw : data.value.project_raw
  } catch (e) {
    toast.bad(`读取 MCP 失败：${(e as Error)?.message || '请重试'}`)
  }
}

// 切换作用域时把配置原文换成对应那份
watch(scope, (next) => {
  raw.value = next === 'global' ? data.value?.global_raw || '{}' : data.value?.project_raw || '{}'
})

async function save() {
  if (busy.value) return
  busy.value = 'save'
  try {
    await api.mcp.save(scope.value, raw.value)
    await reload()
    toast.ok('已保存，PI 进程已重启')
  } catch (e) {
    toast.bad(`保存失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

async function refresh() {
  if (busy.value) return
  busy.value = 'reload'
  try {
    await api.mcp.reload()
    await reload()
    toast.ok('已重连')
  } catch (e) {
    toast.bad(`重连失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

async function remove(s: MCPServerVO) {
  if (busy.value) return
  busy.value = `del:${s.name}`
  try {
    await api.mcp.remove(s.name, s.scope)
    await reload()
    toast.ok(`已删除 ${s.name}`)
  } catch (e) {
    toast.bad(`删除失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

async function login(s: MCPServerVO) {
  if (busy.value) return
  busy.value = `login:${s.name}`
  try {
    await api.mcp.login(s.name)
    toast.info('已在浏览器里打开登录页，完成后点「重连」')
  } catch (e) {
    toast.bad(`发起登录失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

function addTemplate() {
  const tpl = {
    mcpServers: {
      'my-server': { command: 'node', args: ['C:/path/to/server.js'] },
    },
  }
  raw.value = JSON.stringify(tpl, null, 2)
}

onMounted(() => {
  if (settingsStore.mcp) {
    data.value = settingsStore.mcp
    raw.value = scope.value === 'global' ? data.value.global_raw : data.value.project_raw
    void reload()
  } else void reload()
})
</script>

<template>
  <div class="panel">
    <PnHead icon="globe" title="MCP 服务">
      <template #sub>
        配置在 <span class="mono">{{ data?.global_path || '~/.pi/agent/mcp.json' }}</span>；在线状态来自 <span class="mono">pi mcp list</span>——对话开始后 PI 进程起来才会探测，未连接不等于配置坏了。
      </template>
      <el-button :loading="busy === 'reload'" :disabled="!!busy" @click="refresh">重连</el-button>
    </PnHead>

    <div v-if="!servers.length" class="empty card">
      <span class="em-ic"><AppIcon name="globe" /></span>
      <b>还没有配置 MCP 服务</b>
      <p>MCP 让助手用上外部工具（浏览器、数据库、内部系统）。在下面粘一段配置即可。</p>
      <el-button @click="addTemplate">填一个模板</el-button>
    </div>

    <div v-else class="cards">
      <article v-for="s in servers" :key="`${s.scope}:${s.name}`" class="mcard card">
        <div class="m-top">
          <span class="m-ic"><AppIcon name="globe" size="ic-sm" /></span>
          <div class="m-id">
            <b>{{ s.name }}</b>
            <span class="muted sm">{{ s.scope === 'project' ? '项目级' : '全局' }} · 暴露档 {{ s.exposure }}</span>
          </div>
          <el-tag :type="STATE[s.state]?.cls || 'info'" size="small" effect="plain"
            :title="s.state === 'unknown' ? 'PI 进程还没起来；对话开始后自动探测' : ''">
            {{ STATE[s.state]?.label || s.state }}
          </el-tag>
        </div>
        <div class="mono sm muted m-tr">{{ s.transport || '（没有命令或地址）' }}</div>
        <div class="m-tools">
          <span v-for="t in s.tools.slice(0, 6)" :key="t" class="chip sm">{{ t }}</span>
          <span v-if="s.tools.length > 6" class="muted sm">+{{ s.tools.length - 6 }}</span>
          <span v-if="!s.tools.length && s.state !== 'unknown'" class="muted sm">还没有工具清单</span>
          <span v-if="!s.tools.length && s.state === 'unknown'" class="muted sm">对话开始后 PI 会自动接入并列出工具</span>
        </div>
        <p v-if="s.error" class="err">{{ s.error }}</p>
        <div class="m-acts">
          <el-button size="small" :loading="busy === `login:${s.name}`" :disabled="!!busy" @click="login(s)">登录</el-button>
          <el-popconfirm title="删除后助手就用不上这个服务了，确定？" width="260" @confirm="remove(s)">
            <template #reference>
              <el-button size="small" type="danger" plain :loading="busy === `del:${s.name}`" :disabled="!!busy">删除</el-button>
            </template>
          </el-popconfirm>
        </div>
      </article>
    </div>

    <section class="editor card">
      <div class="ed-head">
        <b>配置原文</b>
        <div class="seg">
          <el-radio-group v-model="scope" size="small">
            <el-radio-button value="global">全局</el-radio-button>
            <el-radio-button value="project" :disabled="!hasProject">项目</el-radio-button>
          </el-radio-group>
        </div>
        <span class="sp" />
        <el-button size="small" @click="addTemplate">填模板</el-button>
      </div>
      <CodeEditor v-model="raw" lang="json" :min-height="240" />
      <div class="ed-acts">
        <el-button type="primary" :loading="busy === 'save'" :disabled="!!busy" @click="save">保存</el-button>
        <span class="muted sm">保存前会做 JSON 与 mcpServers 结构校验；不通过一个字符都不写</span>
      </div>
    </section>
  </div>
</template>

<style scoped>
.panel {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
}
.cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(320px, 100%), 1fr));
  gap: var(--wb-sp-3);
}
.mcard {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-4);
}
.m-top {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.m-ic {
  width: var(--wb-tile-sm);
  height: var(--wb-tile-sm);
  border-radius: var(--wb-radius-sm);
  display: grid;
  place-items: center;
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
  flex: none;
}
.m-id {
  display: flex;
  flex-direction: column;
  min-width: 0;
  flex: 1;
}
.m-id b {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.m-tr {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.m-tools {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.err {
  font-size: var(--wb-fs-xs);
  color: var(--wb-danger);
  word-break: break-word;
}
.m-acts {
  display: flex;
  gap: var(--wb-sp-2);
  margin-top: auto;
}
.sm {
  font-size: var(--wb-fs-2xs);
}
.editor {
  padding: var(--wb-sp-4);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}
.ed-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  flex-wrap: wrap;
}
.ed-head b {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.sp {
  flex: 1;
}
.textarea {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-xs);
  line-height: 1.7;
  resize: vertical;
}
.ed-acts {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
}
.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-8) var(--wb-sp-4);
  text-align: center;
}
.em-ic {
  width: var(--wb-tile);
  height: var(--wb-tile);
  border-radius: var(--wb-radius);
  display: grid;
  place-items: center;
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
}
.empty p {
  color: var(--wb-muted);
  font-size: var(--wb-fs-xs);
  max-width: 440px;
  line-height: 1.7;
}
</style>
