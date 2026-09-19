<script setup lang="ts">
/**
 * 自定义子智能体管理视图。
 *
 * <p>每个条目 = 人设 system prompt + 工具 allow/deny 策略 + 记忆开关 + 轮次预算；
 * 保存即写入 harness 注册表，主 Agent 可经 delegate_task 按名委派，也可 /agent 切换为会话主 Agent。
 */
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { Bot, Plus, Trash2 } from '@/components/common/icons'
import { useAgentProfilesStore } from '@/stores/agents'
import { useChatStore } from '@/stores/chat'
import { t } from '@/i18n'
import FormDialog from '@/components/common/FormDialog.vue'
import Field from '@/components/common/Field.vue'
import type { AgentProfile } from '@/types/api'

const agents = useAgentProfilesStore()
const { profiles: list, loading, error, editingName, form } = storeToRefs(agents)
const chat = useChatStore()
const { models } = storeToRefs(chat)

onMounted(async () => {
  agents.load()
  // 模型下拉需要已配置的模型清单：先拉一次，避免让用户手打模型名（打错只会在运行时以 5003 暴露）
  if (models.value.length === 0) await chat.loadModels()
})

/** 可选模型名（按模型名去重，一个模型可能被多个 Provider 提供）。 */
const modelOptions = computed(() => {
  const seen = new Set<string>()
  for (const m of models.value) {
    if (m.model) seen.add(m.model)
  }
  return [...seen].sort()
})

/** 可选的推理强度档位（与后端 thinkingLevels 一致）。 */
const THINKING_LEVELS = ['off', 'low', 'medium', 'high'] as const

// ===== 新建 / 编辑统一弹窗 =====
const showForm = ref(false)
const saving = ref(false)

function openCreate(): void {
  agents.startCreate()
  showForm.value = true
}
function openEdit(p: AgentProfile): void {
  agents.startEdit(p)
  showForm.value = true
}
function closeForm(): void {
  if (saving.value) return
  agents.cancel()
  showForm.value = false
}
async function submitForm(): Promise<void> {
  saving.value = true
  try {
    const saved = await agents.submit()
    if (saved) showForm.value = false
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="scroll wb-ui">
    <div class="wrap wrap-md">
      <header class="hero">
        <div class="tile"><Bot class="ic" /></div>
        <div>
          <h1>{{ t('agents.title') }}</h1>
          <p>{{ t('agents.subtitle') }}</p>
        </div>
        <span class="sp" />
        <button class="btn btn-primary" @click="openCreate">
          <Plus class="ic ic-sm" />
          {{ t('agents.new') }}
        </button>
      </header>

      <div v-if="error && !showForm" class="alert a-danger">{{ error }}</div>

      <section class="card p-5">
        <h2 class="mb-3 font-display text-sm font-semibold text-wb-ink">{{ t('agents.list') }}</h2>
        <div v-if="loading" class="text-sm text-wb-muted">{{ t('ui.status.loading') }}</div>
        <el-table v-else-if="list.length > 0" :data="list" stripe class="wb-el-table">
          <el-table-column :label="t('agents.name')" min-width="160">
            <template #default="{ row }">
              <span class="font-medium text-wb-ink">{{ (row as AgentProfile).name }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('agents.description')" min-width="220" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="text-xs text-wb-muted">{{ (row as AgentProfile).description || t('skill.noDescription') }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('agents.tools')" min-width="160" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="text-xs text-wb-muted">{{ (row as AgentProfile).tools_allow?.length ? (row as AgentProfile).tools_allow!.join(', ') : t('agents.allTools') }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('agents.model')" min-width="150" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="text-xs" :class="(row as AgentProfile).model ? 'text-wb-primary-strong' : 'text-wb-muted'">
                {{ (row as AgentProfile).model || t('agents.modelInherit') }}
              </span>
              <span v-if="(row as AgentProfile).model && (row as AgentProfile).thinking" class="text-xs text-wb-muted">
                · {{ t(`chat.effort.${(row as AgentProfile).thinking}`) }}
              </span>
            </template>
          </el-table-column>
          <el-table-column :label="t('agents.maxTurns')" width="90" align="center">
            <template #default="{ row }">
              <span class="text-xs text-wb-muted">{{ (row as AgentProfile).max_turns || t('agents.turnsDefault') }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="t('common.enabled')" width="80">
            <template #default="{ row }">
              <el-switch
                size="small"
                :model-value="(row as AgentProfile).enabled"
                @change="(v: string | number | boolean) => void agents.toggleEnabled(row as AgentProfile, Boolean(v))"
              />
            </template>
          </el-table-column>
          <el-table-column :label="t('memory.center.col.actions')" width="120" fixed="right">
            <template #default="{ row }">
              <el-button link type="primary" size="small" @click="openEdit(row as AgentProfile)">{{ t('ui.btn.edit') }}</el-button>
              <el-button link type="danger" size="small" @click="agents.remove(row as AgentProfile)">
                <Trash2 class="h-3.5 w-3.5" />
              </el-button>
            </template>
          </el-table-column>
        </el-table>
        <el-empty v-else :description="t('agents.empty')" :image-size="80" class="py-6">
          <el-button type="primary" size="small" @click="openCreate">
            <Plus class="h-3.5 w-3.5" />
            {{ t('agents.new') }}
          </el-button>
        </el-empty>
        <p class="fs11 muted" style="margin: 8px 0 0">{{ t('agents.usageHint') }}</p>
      </section>
    </div>

    <FormDialog
      v-model="showForm"
      :title="editingName ? t('agents.edit') : t('agents.new')"
      :submitting="saving"
      :confirm-text="editingName ? t('ui.btn.save') : t('ui.btn.create')"
      @confirm="submitForm"
      @close="closeForm"
    >
      <div class="wb-fgrid">
        <Field :label="t('agents.name')" required>
          <input v-model="form.name" class="input mono" :placeholder="t('agents.nameHint')" />
        </Field>
        <Field :label="t('common.enabled')">
          <div class="flex items-center" style="height: 28px">
            <span class="switch" :class="{ on: form.enabled }" style="cursor: pointer" @click="form.enabled = !form.enabled" />
          </div>
        </Field>
        <Field :label="t('agents.description')" full>
          <input v-model="form.description" class="input" :placeholder="t('agents.descriptionHint')" />
        </Field>
        <Field :label="t('agents.systemPrompt')" full>
          <textarea v-model="form.system_prompt" class="input mono" rows="8" :placeholder="t('agents.systemPromptHint')" />
        </Field>
      </div>

      <div class="wb-fsect mt4">
        <p class="wb-fsect__title">{{ t('agents.toolsAllow') }}</p>
        <el-select
          v-model="form.tools_allow"
          multiple
          filterable
          allow-create
          default-first-option
          :reserve-keyword="false"
          class="w-full"
          :placeholder="t('agents.toolsAllowHint')"
        >
          <el-option v-for="tn in form.tools_allow || []" :key="tn" :value="tn" :label="tn" />
        </el-select>
      </div>
      <div class="wb-fsect mt4">
        <p class="wb-fsect__title">{{ t('agents.toolsDeny') }}</p>
        <el-select
          v-model="form.tools_deny"
          multiple
          filterable
          allow-create
          default-first-option
          :reserve-keyword="false"
          class="w-full"
          :placeholder="t('agents.toolsDenyHint')"
        >
          <el-option v-for="tn in form.tools_deny || []" :key="tn" :value="tn" :label="tn" />
        </el-select>
      </div>

      <div class="wb-fgrid mt4">
        <Field :label="t('agents.model')">
          <el-select
            v-model="form.model"
            filterable
            clearable
            allow-create
            default-first-option
            class="w-full"
            :placeholder="t('agents.modelHint')"
          >
            <el-option v-for="mn in modelOptions" :key="mn" :value="mn" :label="mn" />
          </el-select>
        </Field>
        <Field :label="t('agents.thinking')">
          <select v-model="form.thinking" class="input" :disabled="!form.model">
            <option value="">{{ t('agents.thinkingFollow') }}</option>
            <option v-for="lv in THINKING_LEVELS" :key="lv" :value="lv">{{ t(`chat.effort.${lv}`) }}</option>
          </select>
        </Field>
      </div>
      <p class="fs11 muted" style="margin: 6px 0 0">{{ t('agents.modelBoundaryHint') }}</p>

      <div class="wb-fgrid mt4">
        <Field :label="t('agents.memory')">
          <div class="flex items-center" style="height: 28px">
            <span class="switch" :class="{ on: form.memory_enable }" style="cursor: pointer" @click="form.memory_enable = !form.memory_enable" />
          </div>
        </Field>
        <Field :label="t('agents.maxTurns')">
          <el-input-number v-model="form.max_turns" :min="0" :max="40" class="w-full" :placeholder="t('agents.turnsDefault')" />
        </Field>
      </div>
      <p class="fs11 muted" style="margin: 6px 0 0">{{ t('agents.formHint') }}</p>
    </FormDialog>
  </div>
</template>
