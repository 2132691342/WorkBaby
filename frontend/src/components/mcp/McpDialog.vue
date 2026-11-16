<script setup lang="ts">
// MCP 服务器编辑弹窗：本地命令 或 远端地址，二选一。
// 两种模式互斥：一个服务器要么拉本地进程要么连远端，同时给两组字段
// 会让人以为都能填，而写进配置时只有一个生效。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

/** 草稿：表单里是「一个字符串」而不是结构化字段（命令与 env 都是一行一条）。 */
export interface McpForm {
  server_name: string
  mode: 'command' | 'url'
  command_text: string
  server_url: string
  transport_kind: string
  desc: string
  work_dir: string
  env_text: string
  /** 连接超时（秒）；空串表示沿用默认 */
  timeout: string
  scope: 'user' | 'project'
}

const props = defineProps<{
  open: boolean
  form: McpForm
  /** 正在编辑的服务器名（空表示新建） */
  editing: string
  saving: boolean
  /** 项目级 mcp.json 路径；空表示没有工作区，项目级不可选 */
  projectPath: string
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
  save: []
}>()

const { t } = useI18n()

const dialog = computed({
  get: () => props.open,
  set: (v: boolean) => emit('update:open', v),
})

/** 名字在编辑时不可改：它是配置里的键，改了等于新建一条。 */
const nameLocked = computed(() => Boolean(props.editing))

/** 作用域与名字同理：编辑时改作用域等于把配置挪个文件再新建一条。 */
const scopeLocked = computed(() => Boolean(props.editing))

// 连接方式与要填的字段互相咬合：本地命令必须有命令行，远程地址必须是 http(s)。
const commandMissing = computed(
  () => props.form.mode === 'command' && !props.form.command_text.trim(),
)
const urlInvalid = computed(() => {
  if (props.form.mode !== 'url') return false
  const url = props.form.server_url.trim()
  return !url || !/^https?:\/\//i.test(url)
})

/** 超时留空合法（沿用默认）；填了就必须是正数。 */
const timeoutInvalid = computed(() => {
  const text = props.form.timeout.trim()
  if (!text) return false
  const value = Number(text)
  return !Number.isFinite(value) || value <= 0
})

/** 保存：按钮与回车共用这一道闸，避免两条路径校验不一致。 */
function submit(): void {
  if (props.saving || !canSave.value) return
  emit('save')
}

const canSave = computed(
  () =>
    Boolean(props.form.server_name.trim()) &&
    !commandMissing.value &&
    !urlInvalid.value &&
    !timeoutInvalid.value,
)
</script>

<template>
  <el-dialog
    v-model="dialog"
    :title="props.editing ? t('mcp.editTitle') : t('mcp.addTitle')"
    width="520"
    :close-on-click-modal="false"
  >
    <!-- 表单内回车 = 保存：这是这类弹窗最自然的提交手势 -->
    <el-form label-position="top" size="small" @submit.prevent="submit">
      <el-form-item :label="t('mcp.name')">
        <el-input
          v-model="props.form.server_name"
          :disabled="nameLocked"
          :placeholder="t('mcp.namePlaceholder')"
          @keyup.enter="submit"
        />
      </el-form-item>

      <el-form-item :label="t('mcp.scope')">
        <el-radio-group v-model="props.form.scope" :disabled="scopeLocked">
          <el-radio-button value="user">{{ t('mcp.userScope') }}</el-radio-button>
          <el-radio-button value="project" :disabled="!props.projectPath">
            {{ t('mcp.projectScope') }}
          </el-radio-button>
        </el-radio-group>
        <p v-if="!props.projectPath || scopeLocked" class="scope-hint">{{ t('mcp.scopeHint') }}</p>
      </el-form-item>

      <el-form-item :label="t('mcp.mode')">
        <el-radio-group v-model="props.form.mode">
          <el-radio-button value="command">{{ t('mcp.modeCommand') }}</el-radio-button>
          <el-radio-button value="url">{{ t('mcp.modeUrl') }}</el-radio-button>
        </el-radio-group>
      </el-form-item>

      <el-form-item v-if="props.form.mode === 'command'" :label="t('mcp.command')" :error="commandMissing ? t('mcp.commandRequired') : ''">
        <el-input v-model="props.form.command_text" :placeholder="t('mcp.commandPlaceholder')" />
      </el-form-item>
      <el-form-item v-else :label="t('mcp.url')" :error="urlInvalid ? t('mcp.urlInvalid') : ''">
        <el-input v-model="props.form.server_url" placeholder="https://example.com/mcp" />
      </el-form-item>

      <template v-if="props.form.mode === 'command'">
        <el-form-item :label="t('mcp.workDir')">
          <el-input v-model="props.form.work_dir" :placeholder="t('mcp.workDirPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('mcp.env')">
          <el-input
            v-model="props.form.env_text"
            type="textarea"
            :rows="3"
            :placeholder="t('mcp.envPlaceholder')"
          />
        </el-form-item>
      </template>

      <el-form-item :label="t('mcp.desc')">
        <el-input v-model="props.form.desc" :placeholder="t('mcp.descPlaceholder')" />
      </el-form-item>

      <el-form-item :label="t('mcp.timeout')" :error="timeoutInvalid ? t('mcp.timeoutInvalid') : ''">
        <el-input
          v-model="props.form.timeout"
          type="number"
          min="1"
          :placeholder="t('mcp.timeoutPlaceholder')"
        />
      </el-form-item>
    </el-form>

    <template #footer>
      <button type="button" class="wb-btn" @click="dialog = false">{{ t('common.cancel') }}</button>
      <button type="button"
        class="wb-btn primary"
        :class="{ 'is-loading': props.saving }"
        :disabled="props.saving || !canSave"
        @click="submit"
      >
        {{ t('common.save') }}
      </button>
    </template>
  </el-dialog>
</template>

<style scoped>
.scope-hint {
  margin: var(--wb-sp-1) 0 0;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-faint);
  line-height: 1.4;
}
</style>
