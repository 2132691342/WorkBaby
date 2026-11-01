<script setup lang="ts">
// MCP 面板：配置读写全局（数据目录）与工作区（.workbaby）两份 mcp.json，
// 在线状态与工具清单来自后端连接池的真实握手结果。
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useToastStore } from '../../stores/toast'
import type { MCPListRESP, MCPServerVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'
import CodeEditor from '../common/CodeEditor.vue'
import SettingsSection from './SettingsSection.vue'
import { errorText } from '../../utils/errors'

const { t } = useI18n()
const toast = useToastStore()

const data = ref<MCPListRESP | null>(null)
const scope = ref<'global' | 'workspace'>('global')
const raw = ref('')
const busy = ref('')

const servers = computed(() => data.value?.resp_servers ?? [])
const hasProject = computed(() => !!data.value?.resp_workspace_path)

const STATE: Record<string, { key: string; tone: string }> = {
  connected: { key: 'online', tone: 'ok' },
  error: { key: 'offline', tone: 'bad' },
  unknown: { key: 'unknown', tone: '' },
}

async function reload() {
  try {
    data.value = await api.mcp.list()
    raw.value = scope.value === 'global' ? data.value.resp_global_raw || '{}' : data.value.resp_workspace_raw || '{}'
  } catch (e) {
    toast.bad(t('common.loadFailed', { msg: errorText(e) }))
  }
}

watch(scope, (next) => {
  raw.value = next === 'global' ? data.value?.resp_global_raw || '{}' : data.value?.resp_workspace_raw || '{}'
})

async function save() {
  if (busy.value) return
  busy.value = 'save'
  try {
    await api.mcp.save(scope.value, raw.value)
    await reload()
    toast.ok(t('settings.mcp.saved'))
  } catch (e) {
    toast.bad(errorText(e) || t('settings.mcp.invalidJson'))
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
    toast.ok(t('settings.mcp.reloaded'))
  } catch (e) {
    toast.bad(t('common.loadFailed', { msg: errorText(e) }))
  } finally {
    busy.value = ''
  }
}

async function remove(s: MCPServerVO) {
  if (busy.value) return
  busy.value = `del:${s.mcp_name}`
  try {
    await api.mcp.remove(s.mcp_name, s.mcp_scope)
    await reload()
    toast.ok(`${t('common.remove')} ${s.mcp_name}`)
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: errorText(e) }))
  } finally {
    busy.value = ''
  }
}

async function login(s: MCPServerVO) {
  if (busy.value) return
  busy.value = `login:${s.mcp_name}`
  try {
    await api.mcp.login(s.mcp_name)
    toast.info(t('settings.mcp.login'))
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: errorText(e) }))
  } finally {
    busy.value = ''
  }
}

function addTemplate() {
  raw.value = JSON.stringify(
    { mcpServers: { 'my-server': { command: 'node', args: ['C:/path/to/server.js'] } } },
    null,
    2,
  )
}

onMounted(() => {
  void reload()
})
</script>

<template>
  <div class="panel-wrap">
    <SettingsSection icon="globe" :title="t('settings.mcp.title')" :desc="t('settings.mcp.desc')">
      <template #actions>
        <el-button size="small" :loading="busy === 'reload'" :disabled="!!busy" @click="refresh">
          {{ busy === 'reload' ? t('settings.mcp.reloading') : t('settings.mcp.reload') }}
        </el-button>
      </template>

      <div v-if="!servers.length" class="wb-card empty-box">
        <span class="wb-tile wb-tile--lg"><AppIcon name="globe" size="ic-xl" /></span>
        <b>{{ t('settings.mcp.empty') }}</b>
        <p>{{ t('settings.mcp.emptyHint') }}</p>
      </div>

      <div v-else class="mcp-grid">
        <article v-for="s in servers" :key="`${s.mcp_scope}:${s.mcp_name}`" class="mcp">
          <header class="mcp-head">
            <span class="mcp-led" :class="`is-${STATE[s.mcp_state]?.tone || 'na'}`" />
            <span class="mcp-name" :title="s.mcp_name">{{ s.mcp_name || t('settings.mcp.unnamed') }}</span>
            <el-tag size="small" :type="s.mcp_state === 'connected' ? 'success' : s.mcp_state === 'error' ? 'danger' : 'info'">
              {{ t(`settings.mcp.${STATE[s.mcp_state]?.key || 'unknown'}`) }}
            </el-tag>
          </header>
          <!-- 传输地址（URL 或启动命令）可能很长：独立一行 + 省略号，
               混进头部会把服务名挤到零宽、把文字顶出卡片边界 -->
          <p v-if="s.mcp_target" class="mcp-transport" :title="s.mcp_target">
            <AppIcon :name="s.mcp_target.startsWith('http') ? 'globe' : 'terminal'" size="ic-2xs" />
            <span class="mcp-transport-tx">{{ s.mcp_target }}</span>
          </p>
          <p v-if="s.mcp_error" class="mcp-err">{{ s.mcp_error }}</p>
          <p class="mcp-meta">
            <span>{{ s.mcp_scope === 'workspace' ? t('settings.mcp.workspace') : t('settings.mcp.global') }}</span>
            <span v-if="s.mcp_tools?.length">{{ t('settings.mcp.tools', { n: s.mcp_tools.length }) }}</span>
          </p>
          <footer class="mcp-acts">
            <el-button size="small" :loading="busy === `login:${s.mcp_name}`" :disabled="!!busy" @click="login(s)">
              {{ t('settings.mcp.login') }}
            </el-button>
            <span class="sp" />
            <el-popconfirm :title="t('settings.mcp.removeTitle')" width="240" @confirm="remove(s)">
              <template #reference>
                <el-button size="small" circle :disabled="!!busy">
                  <AppIcon name="trash" size="ic-xs" />
                </el-button>
              </template>
            </el-popconfirm>
          </footer>
        </article>
      </div>

      <div class="mcp-raw">
        <div class="raw-head">
          <span class="wb-label">{{ t('settings.mcp.raw') }}</span>
          <el-radio-group v-model="scope" size="small">
            <el-radio-button value="global">{{ t('settings.mcp.global') }}</el-radio-button>
            <el-radio-button value="project" :disabled="!hasProject">{{ t('settings.mcp.workspace') }}</el-radio-button>
          </el-radio-group>
          <span class="sp" />
          <el-button size="small" :disabled="!!busy" @click="addTemplate">{{ t('common.add') }}</el-button>
          <el-button size="small" type="primary" :loading="busy === 'save'" :disabled="!!busy" @click="save">
            {{ t('settings.mcp.save') }}
          </el-button>
        </div>
        <p class="wb-mono raw-path">
          {{ scope === 'global' ? data?.resp_global_path : data?.resp_workspace_path || t('settings.mcp.scopeProject') }}
        </p>
        <CodeEditor v-model="raw" lang="json" :min-height="260" />
      </div>
    </SettingsSection>
  </div>
</template>

<style scoped>
.mcp-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(calc(280px * var(--wb-vw-scale)), 1fr));
  gap: var(--wb-sp-3);
}
.mcp {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-3);
  border-radius: var(--wb-radius-lg);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-raised);
  box-shadow: var(--wb-card-shadow);
}
.mcp-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.mcp-led {
  width: 7px;
  height: 7px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-border-strong);
  flex: none;
}
.mcp-led.is-ok {
  background: var(--wb-success);
}
.mcp-led.is-bad {
  background: var(--wb-danger);
}
.mcp-name {
  flex: 1;
  min-width: 0;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-sm);
  font-weight: 700;
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mcp-transport {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-1);
  min-width: 0;
  padding: var(--wb-sp-1) var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  background: var(--wb-elev-hover);
  color: var(--wb-muted);
  font-size: var(--wb-fs-2xs);
}
.mcp-transport-tx {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--font-mono);
}
.mcp-err {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-danger);
  overflow-wrap: anywhere;
}
.mcp-meta {
  display: flex;
  gap: var(--wb-sp-3);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.mcp-acts {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.mcp-raw {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding-top: var(--wb-sp-2);
}
.raw-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.raw-path {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.empty-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-8) var(--wb-sp-6);
  text-align: center;
}
.empty-box b {
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}
.empty-box p {
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
  max-width: calc(420px * var(--wb-vw-scale));
  line-height: var(--wb-lh-base);
}
.sp {
  flex: 1;
}
</style>
