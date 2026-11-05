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
}

const props = defineProps<{
  open: boolean
  form: McpForm
  /** 正在编辑的服务器名（空表示新建） */
  editing: string
  saving: boolean
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
</script>

<template>
  <el-dialog
    v-model="dialog"
    :title="props.editing ? t('mcp.editTitle') : t('mcp.addTitle')"
    width="520"
  >
    <el-form label-position="top" size="small">
      <el-form-item :label="t('mcp.name')">
        <el-input
          v-model="props.form.server_name"
          :disabled="nameLocked"
          :placeholder="t('mcp.namePlaceholder')"
        />
      </el-form-item>

      <el-form-item :label="t('mcp.mode')">
        <el-radio-group v-model="props.form.mode">
          <el-radio-button value="command">{{ t('mcp.modeCommand') }}</el-radio-button>
          <el-radio-button value="url">{{ t('mcp.modeUrl') }}</el-radio-button>
        </el-radio-group>
      </el-form-item>

      <el-form-item v-if="props.form.mode === 'command'" :label="t('mcp.command')">
        <el-input v-model="props.form.command_text" :placeholder="t('mcp.commandPlaceholder')" />
      </el-form-item>
      <el-form-item v-else :label="t('mcp.url')">
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
    </el-form>

    <template #footer>
      <button class="wb-btn" @click="dialog = false">{{ t('common.cancel') }}</button>
      <button
        class="wb-btn primary"
        :class="{ 'is-loading': props.saving }"
        :disabled="props.saving || !props.form.server_name.trim()"
        @click="emit('save')"
      >
        {{ t('common.save') }}
      </button>
    </template>
  </el-dialog>
</template>
