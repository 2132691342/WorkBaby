<script setup lang="ts">
// 记忆面板：四条记忆流的清单 + 按类别筛选 + 手动新增 + 单条删除。
// 记忆库是明文的，用户必须能看见里面到底存了什么、能补一条、也能删掉不该在的东西。
// 新增走后端同一套密钥扫描——界面写入不能成为绕过检查的后门。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import type { MemoryEntryVO, MemoryKindCountVO } from '../../types/api'
import { errorText } from '../../utils/errors'
import { fmtRel } from '../../utils/time'
import { useToastStore } from '../../stores/toast'
import EmptyState from '../common/EmptyState.vue'
import AppIcon from '../common/AppIcon.vue'

const { t } = useI18n()
const toast = useToastStore()

const entries = ref<MemoryEntryVO[]>([])
const counts = ref<MemoryKindCountVO[]>([])
const totalCount = ref(0)
const loading = ref(true)
const failed = ref('')
/** 当前筛选的类别；空串 = 全部 */
const kind = ref('')
/** 正在删除的记忆 id（按行锁按钮） */
const deleting = ref('')
/** 新增表单：类别默认跟当前筛选走，用户在哪一类下就在哪一类里加 */
const adding = ref(false)
const saving = ref(false)
const draftKind = ref('memory')
const draftText = ref('')
/** 新增时可选的四类（不受当前筛选限制） */
const ALL_KINDS = ['memory', 'user', 'project', 'failure'] as const

const kinds = computed(() => counts.value)

const shown = computed(() =>
  kind.value ? entries.value.filter((e) => e.memory_kind === kind.value) : entries.value,
)

/** 类别 → 语义色，四条流一眼分开 */
const kindTone: Record<string, string> = {
  memory: 'mute',
  user: 'accent',
  project: 'ok',
  failure: 'bad',
}

async function reload() {
  loading.value = true
  failed.value = ''
  try {
    const resp = await api.memory.list()
    entries.value = resp?.resp_entries ?? []
    counts.value = resp?.resp_counts ?? []
    totalCount.value = resp?.resp_total ?? 0
  } catch (e) {
    failed.value = errorText(e)
  } finally {
    loading.value = false
  }
}

async function remove(entry: MemoryEntryVO) {
  if (deleting.value) return
  deleting.value = entry.memory_id
  try {
    await api.memory.remove(entry.memory_id)
    toast.ok(t('featurePanel.memoryDeleted'))
    await reload()
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: errorText(e) }))
  } finally {
    deleting.value = ''
  }
}

function startAdding() {
  draftKind.value = kind.value || 'memory'
  draftText.value = ''
  adding.value = true
}

async function submitDraft() {
  const text = draftText.value.trim()
  if (!text || saving.value) return
  saving.value = true
  try {
    await api.memory.save(draftKind.value, text)
    toast.ok(t('featurePanel.memorySaved'))
    adding.value = false
    draftText.value = ''
    await reload()
  } catch (e) {
    toast.bad(errorText(e))
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  void reload()
})

defineExpose({ reload })
</script>

<template>
  <div class="mem-wrap">
    <div v-if="loading && !entries.length" class="wb-card pad">
      <el-skeleton animated :rows="4" />
    </div>

    <div v-else-if="failed" class="wb-card pad err">
      <span class="err-tx">{{ t('common.loadFailed', { msg: failed }) }}</span>
      <el-button size="small" @click="reload">{{ t('common.retry') }}</el-button>
    </div>

    <template v-else-if="!totalCount">
      <div v-if="adding" class="create-box">
        <el-select v-model="draftKind" size="small" class="kind-sel">
          <el-option
            v-for="k in ALL_KINDS"
            :key="k"
            :value="k"
            :label="t(`featurePanel.memoryKind.${k}`)"
          />
        </el-select>
        <el-input
          v-model="draftText"
          type="textarea"
          :autosize="{ minRows: 2, maxRows: 8 }"
          :placeholder="t('featurePanel.memoryAddPlaceholder')"
        />
        <div class="create-acts">
          <el-button
            size="small"
            type="primary"
            :loading="saving"
            :disabled="!draftText.trim() || saving"
            @click="submitDraft"
          >
            {{ t('common.confirm') }}
          </el-button>
          <el-button size="small" :disabled="saving" @click="adding = false">
            {{ t('common.cancel') }}
          </el-button>
        </div>
      </div>
      <template v-else>
        <EmptyState
          icon="brain"
          :title="t('featurePanel.memoryEmpty')"
          :desc="t('featurePanel.memoryEmptyHint')"
        />
        <el-button size="small" class="mk-btn" @click="startAdding">
          <AppIcon name="plus" size="ic-2xs" />
          <span class="ml-1">{{ t('featurePanel.memoryAdd') }}</span>
        </el-button>
      </template>
    </template>

    <template v-else>
      <div class="filters">
        <button
          class="wb-row chip"
          :class="{ 'is-on': kind === '' }"
          type="button"
          @click="kind = ''"
        >
          {{ t('featurePanel.memoryAll') }}
          <span class="cnt">{{ totalCount }}</span>
        </button>
        <button
          v-for="k in kinds"
          :key="k.kind_name"
          class="wb-row chip"
          :class="{ 'is-on': kind === k.kind_name, 'is-zero': !k.kind_count }"
          type="button"
          :disabled="!k.kind_count"
          @click="kind = k.kind_name"
        >
          {{ t(`featurePanel.memoryKind.${k.kind_name}`) }}
          <span class="cnt">{{ k.kind_count }}</span>
        </button>
      </div>

      <div v-if="adding" class="create-box">
        <el-select v-model="draftKind" size="small" class="kind-sel">
          <el-option
            v-for="k in ALL_KINDS"
            :key="k"
            :value="k"
            :label="t(`featurePanel.memoryKind.${k}`)"
          />
        </el-select>
        <el-input
          v-model="draftText"
          type="textarea"
          :autosize="{ minRows: 2, maxRows: 8 }"
          :placeholder="t('featurePanel.memoryAddPlaceholder')"
        />
        <div class="create-acts">
          <el-button
            size="small"
            type="primary"
            :loading="saving"
            :disabled="!draftText.trim() || saving"
            @click="submitDraft"
          >
            {{ t('common.confirm') }}
          </el-button>
          <el-button size="small" :disabled="saving" @click="adding = false">
            {{ t('common.cancel') }}
          </el-button>
        </div>
      </div>
      <el-button v-else size="small" class="mk-btn" @click="startAdding">
        <AppIcon name="plus" size="ic-2xs" />
        <span class="ml-1">{{ t('featurePanel.memoryAdd') }}</span>
      </el-button>

      <EmptyState
        v-if="!shown.length && !adding"
        compact
        icon="brain"
        :title="t('featurePanel.memoryFilterEmpty')"
      />

      <ul v-else class="list">
        <li v-for="e in shown" :key="e.memory_id" class="entry">
          <div class="entry-head">
            <span class="tag" :class="`is-${kindTone[e.memory_kind] || 'mute'}`">
              {{ t(`featurePanel.memoryKind.${e.memory_kind}`) }}
            </span>
            <span class="time">{{ fmtRel(e.memory_updated_at) }}</span>
            <el-button
              class="del"
              size="small"
              text
              :loading="deleting === e.memory_id"
              :disabled="!!deleting"
              :title="t('featurePanel.memoryDelete')"
              @click="remove(e)"
            >
              <AppIcon v-if="deleting !== e.memory_id" name="trash" size="ic-xs" />
            </el-button>
          </div>
          <p class="text">{{ e.memory_text }}</p>
        </li>
      </ul>
    </template>
  </div>
</template>

<style scoped>
.create-box {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.create-acts {
  display: flex;
  gap: var(--wb-sp-2);
}
.kind-sel {
  width: calc(160px * var(--wb-vw-scale));
}
.mk-btn {
  align-self: flex-start;
}
.mem-wrap {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
  min-height: 0;
}
.filters {
  display: flex;
  flex-wrap: wrap;
  gap: var(--wb-sp-1);
}
.chip {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  padding: 2px var(--wb-sp-2);
  border-radius: var(--wb-radius-full);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-raised);
  font-size: var(--wb-fs-2xs);
  font-weight: 600;
  color: var(--wb-ink-2);
  cursor: pointer;
  transition:
    background-color var(--wb-dur) var(--wb-ease),
    border-color var(--wb-dur) var(--wb-ease),
    color var(--wb-dur) var(--wb-ease);
}
.chip:hover:not(:disabled) {
  border-color: var(--wb-border-strong);
}
.chip.is-on {
  background: var(--wb-accent-tint);
  border-color: var(--wb-primary);
  color: var(--wb-primary);
}
.chip.is-zero {
  opacity: 0.5;
  cursor: not-allowed;
}
.cnt {
  font-variant-numeric: tabular-nums;
  opacity: 0.72;
}
.list {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  margin: 0;
  padding: 0;
  list-style: none;
}
.entry {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-raised);
  transition:
    border-color var(--wb-dur) var(--wb-ease),
    box-shadow var(--wb-dur) var(--wb-ease);
}
.entry:hover {
  border-color: var(--wb-border-strong);
  box-shadow: var(--wb-card-shadow);
}
.entry-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.tag {
  padding: 1px var(--wb-sp-2);
  border-radius: var(--wb-radius-full);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  border: 1px solid transparent;
}
.tag.is-mute {
  background: var(--wb-elev-hover);
  color: var(--wb-muted);
  border-color: var(--wb-border);
}
.tag.is-accent {
  background: var(--wb-accent-tint);
  color: var(--wb-primary);
  border-color: var(--wb-primary);
}
.tag.is-ok {
  background: var(--wb-accent-tint);
  color: var(--wb-success);
  border-color: var(--wb-success);
}
.tag.is-bad {
  background: var(--wb-danger-soft);
  color: var(--wb-danger);
  border-color: var(--wb-danger);
}
.time {
  margin-left: auto;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
}
.del {
  flex: none;
}
.text {
  margin: 0;
  font-size: var(--wb-fs-sm);
  line-height: 1.6;
  color: var(--wb-ink-2);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.pad {
  padding: var(--wb-sp-4);
}
.err {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--wb-sp-3);
}
.err-tx {
  font-size: var(--wb-fs-sm);
  color: var(--wb-danger);
}
</style>
