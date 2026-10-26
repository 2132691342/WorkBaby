<!-- 运行时环境：这台机器上跑助手要用的 Node / Git、命令执行环境、终端入口与数据位置。
     只讲「有没有、够不够、挂在哪级 PATH」；引擎自身（版本、升级、参数）在引擎面板。 -->
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import type { EngineStatusVO, PathLevel, RuntimeComponent, ToolchainVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'
import SettingsSection from './SettingsSection.vue'

const { t } = useI18n()
const toast = useToastStore()
const settings = useSettingsStore()

const status = ref<EngineStatusVO | null>(null)
const toolchain = ref<ToolchainVO | null>(null)
const loading = ref(true)
const rechecking = ref(false)
const fixingShell = ref(false)
const enablingTerminal = ref(false)
const switchingLevel = ref('')
/** 正在改 PATH 的组件:级别（同时只允许一趟：写系统级要等 UAC） */
const promoting = ref('')
const switchingNode = ref(false)
const enablingGit = ref(false)

async function loadStatus() {
  try {
    status.value = await api.engine.status()
  } catch (e) {
    toast.bad(t('common.loadFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    loading.value = false
  }
}

/** 工具链读不出来不算故障：面板上的运行时读数仍然成立 */
async function loadToolchain() {
  try {
    toolchain.value = await api.env.toolchain()
  } catch {
    toolchain.value = null
  }
}

/** 重新检测：刚装了 Node / Git 或改过 PATH 时点这个，后端会先作废探测缓存 */
async function recheck() {
  if (rechecking.value) return
  rechecking.value = true
  try {
    await api.env.recheck()
    await Promise.all([loadStatus(), loadToolchain()])
    toast.ok(t('settings.runtime.rechecked'))
  } catch (e) {
    toast.bad((e as Error)?.message || t('common.saveFailed', { msg: '' }))
  } finally {
    rechecking.value = false
  }
}

/** 改 PATH 级别：系统级会拉起 UAC，这一趟可能要等用户输密码 */
async function promote(component: RuntimeComponent, level: PathLevel) {
  if (promoting.value) return
  promoting.value = `${component}:${level}`
  try {
    toolchain.value = await api.env.promote(component, level)
    await loadStatus()
    toast.ok(t('settings.runtime.promoteDone', { level: levelLabel(level) }))
  } catch (e) {
    toast.bad((e as Error)?.message || t('common.saveFailed', { msg: '' }))
  } finally {
    promoting.value = ''
  }
}

function promoteNode(level: string) {
  void promote('node', level as PathLevel)
}

function promoteGit(level: string) {
  void promote('git', level as PathLevel)
}

/** 换解释器：后续升级与插件管理都跟着走这份 Node，助手进程会重启 */
async function switchNode() {
  const alt = toolchain.value?.alt
  if (!alt || switchingNode.value) return
  switchingNode.value = true
  try {
    toolchain.value = await api.env.switchNode(alt.exe)
    await loadStatus()
    toast.ok(t('settings.runtime.nodeSwitchDone', { version: alt.version }))
  } catch (e) {
    toast.bad((e as Error)?.message || t('common.saveFailed', { msg: '' }))
  } finally {
    switchingNode.value = false
  }
}

/** 机器上一份 git 都没有：解压内置 MinGit 兜底（要等几秒） */
async function enableBundledGit() {
  if (enablingGit.value) return
  enablingGit.value = true
  try {
    toolchain.value = await api.env.enableBundledGit()
    await loadStatus()
    toast.ok(t('settings.runtime.gitEnabled', { version: toolchain.value?.git.version || '' }))
  } catch (e) {
    toast.bad((e as Error)?.message || t('common.saveFailed', { msg: '' }))
  } finally {
    enablingGit.value = false
  }
}

/** 命令执行环境的几种形态：认不出的一律按「没有」说，别把内部名字丢给用户看 */
function shellKindLabel(kind: string, source = '') {
  switch (kind) {
    case 'git-bash':
      return source === 'bundled'
        ? t('settings.runtime.shellKindBundled')
        : t('settings.runtime.shellKindGitBash')
    case 'msys':
      return t('settings.runtime.shellKindMSYS')
    case 'cygwin':
      return t('settings.runtime.shellKindCygwin')
    case 'powershell':
      return t('settings.runtime.shellKindPowerShell')
    default:
      return t('settings.runtime.shellKindNone')
  }
}

/** 重新探测并写回 PI：用户装了 Git 或改过 PATH 之后点这里 */
async function fixShell() {
  if (fixingShell.value) return
  fixingShell.value = true
  try {
    const vo = await api.env.fixShell()
    await loadStatus()
    if (!vo.shell_usable) {
      toast.bad(t('settings.runtime.shellFailed', { msg: vo.shell_error || '' }))
      return
    }
    toast.ok(
      vo.shell_kind === 'powershell'
        ? t('settings.runtime.shellFixedPS')
        : t('settings.runtime.shellFixedBash', { kind: shellKindLabel(vo.shell_kind) }),
    )
  } catch (e) {
    toast.bad(t('settings.runtime.shellFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    fixingShell.value = false
  }
}

/** 生成终端入口并挂 PATH：界面里能用不等于终端里能用，两者要各自说清楚 */
async function enableTerminal() {
  if (enablingTerminal.value) return
  enablingTerminal.value = true
  try {
    await api.env.enableTerminal()
    await loadStatus()
    if (status.value?.terminal_ready) {
      toast.ok(t('settings.runtime.terminalDone'))
    } else {
      toast.info(
        t('settings.runtime.terminalStill', {
          msg: status.value?.terminal_error || status.value?.terminal_shadowed_by || '',
        }),
      )
    }
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    enablingTerminal.value = false
  }
}

/** 入口当前挂在哪一级 PATH：system 全机可用，user 只影响本账户 */
function levelLabel(level: string | undefined) {
  switch (level) {
    case 'system':
      return t('settings.runtime.levelSystem')
    case 'user':
      return t('settings.runtime.levelUser')
    case 'both':
      return t('settings.runtime.levelBoth')
    default:
      return t('settings.runtime.levelNone')
  }
}

/** 切换入口的 PATH 级别：后端会拉起 UAC 授权并等它完成，这一趟可能要等用户输密码 */
async function switchLevel(level: 'system' | 'user') {
  if (switchingLevel.value) return
  switchingLevel.value = level
  try {
    const vo = await api.env.switchTerminalLevel(level)
    await loadStatus()
    if (vo.ready) {
      toast.ok(t('settings.runtime.levelSwitched', { level: levelLabel(vo.level) }))
    } else {
      toast.info(t('settings.runtime.terminalStill', { msg: vo.error || '' }))
    }
  } catch (e) {
    toast.bad((e as Error)?.message || t('common.saveFailed', { msg: '' }))
  } finally {
    switchingLevel.value = ''
  }
}

function runtimeSource(src: string) {
  switch (src) {
    case 'bundled':
      return t('settings.runtime.sourceBundled')
    case 'env':
    case 'override':
      return t('settings.runtime.sourceOverride')
    default:
      return t('settings.runtime.sourceSys')
  }
}

async function copyPath(p: string) {
  if (!p) return
  try {
    await navigator.clipboard.writeText(p)
    toast.ok(t('common.copiedPath'))
  } catch {
    toast.bad(t('common.copyFailed'))
  }
}

onMounted(() => {
  void loadStatus()
  void loadToolchain()
  if (!settings.piSettings) void settings.loadPISettings()
})
</script>

<template>
  <div class="panel-wrap">
    <SettingsSection
      icon="terminal"
      :title="t('settings.runtime.nodeGit')"
      :desc="t('settings.runtime.desc')"
    >
      <template #actions>
        <el-button size="small" :loading="rechecking" :disabled="rechecking" @click="recheck">
          {{ rechecking ? t('settings.runtime.rechecking') : t('settings.runtime.recheck') }}
        </el-button>
      </template>

      <div v-if="loading" class="wb-card rt-card">
        <el-skeleton animated :rows="3" />
      </div>

      <div v-else-if="status" class="rt-grid">
        <!-- Node：来源、版本、PATH 落地级别与切换 -->
        <div class="rt-kv">
          <span class="wb-label">{{ t('settings.runtime.node') }}</span>
          <p class="rt-line">
            <AppIcon name="check" size="ic-2xs" :class="status.node_ready ? 'ok' : 'bad'" />
            <span v-if="status.node_ready">
              {{ t('settings.runtime.nodeOk', { version: status.node_version, source: runtimeSource(status.node_source) }) }}
            </span>
            <span v-else>{{ t('settings.runtime.nodeMissing', { msg: status.node_error }) }}</span>
            <el-tag v-if="toolchain?.bound" size="small" type="info">
              {{ t('settings.runtime.nodeBound') }}
            </el-tag>
          </p>
          <p class="rt-mono muted">{{ status.node_path || '—' }}</p>

          <!-- 当前用内置、系统里另有一份合规的：一键换过去，别让两套生态并存 -->
          <div v-if="toolchain?.alt" class="rt-alt">
            <p class="rt-meta">
              {{ t('settings.runtime.nodeAlt', { version: toolchain.alt.version, source: runtimeSource(toolchain.alt.source) }) }}
            </p>
            <el-popconfirm :title="t('settings.runtime.nodeSwitchHint')" width="300" @confirm="switchNode">
              <template #reference>
                <el-button size="small" :loading="switchingNode" :disabled="switchingNode">
                  {{ t('settings.runtime.nodeSwitch') }}
                </el-button>
              </template>
            </el-popconfirm>
          </div>

          <div class="rt-row">
            <span class="rt-level">{{ t('settings.runtime.nodeLevel') }}</span>
            <el-tag size="small" :type="status.node_level === 'system' ? 'success' : 'info'">
              {{ levelLabel(status.node_level) }}
            </el-tag>
            <el-dropdown trigger="click" :disabled="!!promoting || !status.node_ready" @command="promoteNode">
              <el-button size="small" :loading="promoting.startsWith('node:')">
                {{ t('settings.runtime.levelChange') }}
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="system" :disabled="status.node_level === 'system'">
                    {{ t('settings.runtime.levelSystem') }}
                  </el-dropdown-item>
                  <el-dropdown-item command="user" :disabled="status.node_level === 'user'">
                    {{ t('settings.runtime.levelUser') }}
                  </el-dropdown-item>
                  <el-dropdown-item command="none" :disabled="status.node_level === 'none'">
                    {{ t('settings.runtime.levelNone') }}
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
          <p class="rt-meta">{{ t('settings.runtime.nodeLevelHint') }}</p>
          <p v-if="toolchain?.node.conflict" class="rt-warn">
            <AppIcon name="alert" size="ic-2xs" />
            {{ t('settings.runtime.levelConflict', { path: toolchain.node.conflict }) }}
          </p>
        </div>

        <!-- Git：助手跑命令与界面读状态共用这一份 -->
        <div class="rt-kv">
          <span class="wb-label">{{ t('settings.runtime.git') }}</span>
          <p class="rt-line">
            <AppIcon :name="status.git_ready ? 'check' : 'info'" size="ic-2xs" :class="status.git_ready ? 'ok' : 'bad'" />
            <span v-if="status.git_ready">
              {{ t('settings.runtime.gitOk', { version: status.git_version, source: runtimeSource(status.git_source) }) }}
            </span>
            <span v-else>{{ t('settings.runtime.gitMissing') }}</span>
          </p>
          <p class="rt-mono muted">{{ status.git_path || '—' }}</p>

          <div v-if="status.git_ready" class="rt-row">
            <span class="rt-level">{{ t('settings.runtime.nodeLevel') }}</span>
            <el-tag size="small" :type="status.git_level === 'system' ? 'success' : 'info'">
              {{ levelLabel(status.git_level) }}
            </el-tag>
            <el-dropdown trigger="click" :disabled="!!promoting" @command="promoteGit">
              <el-button size="small" :loading="promoting.startsWith('git:')">
                {{ t('settings.runtime.levelChange') }}
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="system" :disabled="status.git_level === 'system'">
                    {{ t('settings.runtime.levelSystem') }}
                  </el-dropdown-item>
                  <el-dropdown-item command="user" :disabled="status.git_level === 'user'">
                    {{ t('settings.runtime.levelUser') }}
                  </el-dropdown-item>
                  <el-dropdown-item command="none" :disabled="status.git_level === 'none'">
                    {{ t('settings.runtime.levelNone') }}
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
          <el-button v-else size="small" :loading="enablingGit" :disabled="enablingGit" @click="enableBundledGit">
            {{ t('settings.runtime.gitEnable') }}
          </el-button>
          <p class="rt-meta">{{ t('settings.runtime.gitHint') }}</p>
        </div>
      </div>
    </SettingsSection>

    <SettingsSection icon="zap" :title="t('settings.runtime.cmd')" :desc="t('settings.runtime.cmdHint')" divided>
      <template v-if="status">
        <div class="rt-block">
          <span class="wb-label">{{ t('settings.runtime.shell') }}</span>
          <p class="rt-line">
            <AppIcon :name="status.shell_usable ? 'check' : 'alert'" size="ic-2xs" :class="status.shell_usable ? 'ok' : 'bad'" />
            {{ shellKindLabel(status.shell_kind, status.shell_source) }}
          </p>
          <p class="rt-mono muted">{{ status.shell_path || status.shell_error || '—' }}</p>
          <p class="rt-meta">{{ t('settings.runtime.shellDesc') }}</p>
          <div class="rt-row">
            <el-button size="small" :loading="fixingShell" :disabled="fixingShell" @click="fixShell">
              {{ fixingShell ? t('settings.runtime.shellFixing') : t('settings.runtime.shellFix') }}
            </el-button>
          </div>
        </div>

        <div class="rt-block">
          <span class="wb-label">{{ t('settings.runtime.terminal') }}</span>
          <p class="rt-line">
            <AppIcon :name="status.terminal_ready ? 'check' : 'info'" size="ic-2xs" :class="status.terminal_ready ? 'ok' : 'bad'" />
            {{ status.terminal_ready ? t('settings.runtime.terminalReady') : t('settings.runtime.terminalMissing') }}
          </p>
          <p class="rt-mono muted">{{ status.terminal_path || status.terminal_error || '—' }}</p>
          <p v-if="status.other_pis?.length" class="rt-warn">
            <AppIcon name="alert" size="ic-2xs" />
            {{ t('settings.runtime.terminalConflict', { paths: status.other_pis.join('；') }) }}
          </p>
          <div class="rt-row">
            <el-button
              v-if="!status.terminal_ready && !status.terminal_shadowed_by"
              size="small"
              :loading="enablingTerminal"
              :disabled="enablingTerminal || !status.pi_ready"
              @click="enableTerminal"
            >
              {{ enablingTerminal ? t('settings.runtime.terminalEnabling') : t('settings.runtime.terminalEnable') }}
            </el-button>
            <p v-else-if="status.terminal_shadowed_by" class="rt-meta">
              {{ t('settings.runtime.terminalShadowHint') }}
            </p>
            <p v-else-if="!status.terminal_on_path" class="rt-meta">
              {{ t('settings.runtime.terminalOnPath') }}
            </p>
            <p v-else class="rt-meta">{{ t('settings.runtime.terminalHint') }}</p>
          </div>

          <div v-if="status.terminal_on_path" class="rt-row">
            <span class="rt-level">{{ t('settings.runtime.levelLabel') }}</span>
            <el-tag size="small" :type="status.terminal_level === 'system' ? 'success' : 'info'">
              {{ levelLabel(status.terminal_level) }}
            </el-tag>
            <el-popconfirm
              v-if="status.terminal_level !== 'system'"
              :title="t('settings.runtime.levelUpHint')"
              width="280"
              @confirm="switchLevel('system')"
            >
              <template #reference>
                <el-button size="small" :loading="switchingLevel === 'system'" :disabled="!!switchingLevel">
                  {{ t('settings.runtime.levelUp') }}
                </el-button>
              </template>
            </el-popconfirm>
            <el-popconfirm
              v-if="status.terminal_level !== 'user'"
              :title="t('settings.runtime.levelDownHint')"
              width="280"
              @confirm="switchLevel('user')"
            >
              <template #reference>
                <el-button size="small" :loading="switchingLevel === 'user'" :disabled="!!switchingLevel">
                  {{ t('settings.runtime.levelDown') }}
                </el-button>
              </template>
            </el-popconfirm>
          </div>
          <p v-if="status.terminal_on_path" class="rt-meta">{{ t('settings.runtime.levelHint') }}</p>
        </div>
      </template>
      <el-skeleton v-else animated :rows="3" />
    </SettingsSection>

    <SettingsSection
      icon="folder"
      :title="t('settings.runtime.location')"
      :desc="t('settings.runtime.locationHint')"
      divided
    >
      <div class="paths">
        <button
          v-for="p in [
            { key: 'agentDir', value: status?.agent_dir },
            { key: 'installPrefix', value: toolchain?.install_prefix },
            { key: 'installRoot', value: status?.install_root },
            { key: 'settingsPath', value: settings.piSettings?.settings_path },
            { key: 'modelsPath', value: settings.piSettings?.models_path },
            { key: 'mcpPath', value: settings.piSettings?.mcp_path },
            { key: 'sessionsDir', value: settings.piSettings?.sessions_dir },
            { key: 'extensionsDir', value: settings.piSettings?.extensions_dir },
            { key: 'skillsDir', value: settings.piSettings?.skills_dir },
          ]"
          :key="p.key"
          class="path-row"
          type="button"
          :title="t('settings.runtime.copyPath')"
          @click="copyPath(p.value || '')"
        >
          <span class="path-k">{{ t(`settings.runtime.${p.key}`) }}</span>
          <span class="path-v wb-mono">{{ p.value || '—' }}</span>
          <AppIcon name="copy" size="ic-xs" class="path-copy" />
        </button>
      </div>
    </SettingsSection>
  </div>
</template>

<style scoped>
/* 表单类面板收窄：控件列与标签列都靠左，面板宽 920px 时右侧会拖出一片空地 */
.panel-wrap {
  --wb-panel-w: calc(720px * var(--wb-vw-scale));
}
.rt-card {
  padding: var(--wb-sp-4);
}
.rt-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(calc(300px * var(--wb-vw-scale)), 1fr));
  gap: var(--wb-sp-4);
}
.rt-kv,
.rt-block {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
  min-width: 0;
}
.rt-block + .rt-block {
  margin-top: var(--wb-sp-5);
  padding-top: var(--wb-sp-5);
  border-top: 1px solid var(--wb-border);
}
.rt-line {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-1);
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-2);
}
.rt-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  margin-top: var(--wb-sp-1);
}
.rt-level {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  flex: none;
}
.rt-meta {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  overflow-wrap: anywhere;
  line-height: var(--wb-lh-base);
}
.rt-warn {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-1);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-warning);
  line-height: var(--wb-lh-base);
}
.rt-mono {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink-2);
  overflow-wrap: anywhere;
}
.rt-mono.muted {
  color: var(--wb-muted);
}
.rt-alt {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  margin-top: var(--wb-sp-1);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  background: var(--wb-elev-hover);
  animation: rt-alt-in var(--wb-dur) var(--wb-ease);
}
@keyframes rt-alt-in {
  from {
    opacity: 0;
    transform: translateY(calc(-2px * var(--wb-vw-scale)));
  }
  to {
    opacity: 1;
    transform: none;
  }
}
.ok {
  color: var(--wb-success);
}
.bad {
  color: var(--wb-warning);
}
.paths {
  display: flex;
  flex-direction: column;
  gap: 1px;
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius);
  overflow: hidden;
}
.path-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  text-align: left;
  transition: background var(--wb-dur-fast) var(--wb-ease);
}
.path-row:hover {
  background: var(--wb-elev-hover);
}
.path-k {
  width: calc(120px * var(--wb-vw-scale));
  flex: none;
  font-size: var(--wb-fs-xs);
  font-weight: 600;
  color: var(--wb-muted);
}
.path-v {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.path-copy {
  color: var(--wb-muted);
  opacity: 0;
  transition: opacity var(--wb-dur-fast) var(--wb-ease);
}
.path-row:hover .path-copy {
  opacity: 1;
}
</style>
