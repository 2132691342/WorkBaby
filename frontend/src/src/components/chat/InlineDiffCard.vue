<script setup lang="ts">
import { computed, ref } from 'vue'
import { ChevronDown, RotateCcw } from '@/components/common/icons'
import { t } from '@/i18n'
import { useChatStore } from '@/stores/chat'
import { useDialog } from '@/composables/useDialog'
import { useToast } from '@/composables/useToast'
import DiffView from '@/components/chat/DiffView.vue'
import { parseDiffRows } from '@/chat/models/blocks'
import type { FileChange, FileChangeDetail } from '@/types/api'

/**
 * 内联 diff 卡：文件变更直接长在消息流里，折叠态是一行「动作 + 路径 + 增删行数」，
 * 点开就地看 diff、就地回滚——不必为了看一眼 diff 离开当前阅读上下文去侧栏。
 *
 * <p>diff 正文懒加载：列表接口不带 diff（避免一次会话拉几十份全文），
 * 展开时才 GET /chat/changes/:id，且只拉一次。
 */
const props = defineProps<{
  change: FileChange
  /** 是否允许回滚（历史消息同样允许；已回滚的自动禁用）。 */
  rollbackable?: boolean
}>()

const chat = useChatStore()
const dialog = useDialog()
const toast = useToast()

const open = ref(false)
const loading = ref(false)
const loaded = ref(false)
const detail = ref<FileChangeDetail | null>(null)

/** 展开时懒加载 diff；失败只提示不阻断（折叠态信息已足够定位文件）。 */
async function toggle(): Promise<void> {
  open.value = !open.value
  if (!open.value || loaded.value) return
  loading.value = true
  try {
    const d = await chat.loadFileChangeDetail(props.change.id)
    if (d) {
      detail.value = d
      loaded.value = true
    }
  } finally {
    loading.value = false
  }
}

const diffRows = computed(() => (detail.value?.diff ? parseDiffRows(detail.value.diff).length : 0))

/** 动作 → 展示标签（与 FileChangesPanel 同一套 i18n 键，口径一致）。 */
const actionLabel = computed(() => t(`changes.action.${props.change.action}`))

const rolledBack = computed(() => props.change.rolled_back)

async function rollback(): Promise<void> {
  const ok = await dialog.confirm({
    title: t('changes.rollbackTitle'),
    content: t('changes.rollbackConfirm', props.change.rel_path),
    danger: true
  })
  if (!ok) return
  if (await chat.rollbackFileChange(props.change.id)) {
    toast.success(t('changes.rolledBack', props.change.rel_path))
  } else {
    toast.error(t('changes.rollbackFailed'))
  }
}
</script>

<template>
  <div class="wb-diffcard" :class="{ 'wb-diffcard--open': open, 'wb-diffcard--rolled': rolledBack }">
    <div class="wb-diffcard-hd">
      <button type="button" class="wb-diffcard-main" :title="props.change.rel_path" @click="toggle">
        <span class="wb-diffcard-action">{{ actionLabel }}</span>
        <span class="wb-diffcard-path">{{ props.change.rel_path }}</span>
        <span class="wb-diffcard-stat">
          <span v-if="props.change.added_lines > 0" class="add">+{{ props.change.added_lines }}</span>
          <span v-if="props.change.removed_lines > 0" class="del">-{{ props.change.removed_lines }}</span>
        </span>
        <span v-if="rolledBack" class="wb-diffcard-badge">{{ t('changes.rolledBackBadge') }}</span>
        <ChevronDown class="wb-diffcard-chev" :class="{ 'is-open': open }" />
      </button>
      <!-- 回滚放在卡片头右侧：改错了要能立刻撤销，不用先展开 -->
      <button
        v-if="props.rollbackable !== false && !rolledBack"
        type="button"
        class="wb-diffcard-act"
        :title="t('changes.rollback')"
        @click.stop="rollback"
      >
        <RotateCcw />
      </button>
    </div>

    <div v-if="open" class="wb-diffcard-bd">
      <p v-if="loading" class="wb-diffcard-hint">{{ t('common.loading') }}</p>
      <DiffView
        v-else-if="diffRows > 0"
        :diff="detail?.diff ?? ''"
        :max-height="288"
      />
      <p v-else class="wb-diffcard-hint">{{ t('changes.diffEmpty') }}</p>
    </div>
  </div>
</template>

<style scoped>
/* 内联卡片：比侧栏列表项更轻，融进正文流而不抢注意力 */
.wb-diffcard {
  --dc-border: var(--wb-border);
  border: 1px solid var(--dc-border);
  border-radius: 10px;
  background: color-mix(in srgb, var(--wb-mint) 4%, transparent);
  overflow: hidden;
}
.wb-diffcard--rolled {
  background: var(--wb-surface-2);
  opacity: 0.75;
}
.wb-diffcard-hd {
  display: flex;
  align-items: center;
}
.wb-diffcard-main {
  display: flex;
  min-width: 0;
  flex: 1;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border: 0;
  background: transparent;
  color: var(--wb-ink);
  font-size: 12px;
  text-align: left;
  cursor: pointer;
  transition: background 0.15s ease;
}
.wb-diffcard-main:hover {
  background: var(--wb-surface-hover);
}
.wb-diffcard-action {
  flex-shrink: 0;
  border-radius: 4px;
  background: color-mix(in srgb, var(--wb-primary) 10%, transparent);
  padding: 0 4px;
  font-size: 10px;
  font-weight: 500;
  color: var(--wb-primary-strong);
}
.wb-diffcard-path {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--font-mono);
  font-size: 11px;
}
.wb-diffcard-stat {
  flex-shrink: 0;
  font-family: var(--font-mono);
  font-size: 10px;
  font-variant-numeric: tabular-nums;
}
.wb-diffcard-stat .add {
  color: var(--wb-mint);
}
.wb-diffcard-stat .del {
  margin-left: 4px;
  color: var(--wb-danger);
}
.wb-diffcard-badge {
  flex-shrink: 0;
  border-radius: 4px;
  background: var(--wb-surface-hover);
  padding: 0 4px;
  font-size: 10px;
  color: var(--wb-muted);
}
.wb-diffcard-chev {
  width: 12px;
  height: 12px;
  flex-shrink: 0;
  color: var(--wb-muted);
  transition: transform 0.18s ease;
}
.wb-diffcard-chev.is-open {
  transform: rotate(180deg);
}
.wb-diffcard-act {
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  padding: 0 10px;
  align-self: stretch;
  border: 0;
  border-left: 1px solid var(--dc-border);
  background: transparent;
  color: var(--wb-muted);
  cursor: pointer;
  transition: color 0.15s ease, background 0.15s ease;
}
.wb-diffcard-act:hover {
  background: color-mix(in srgb, var(--wb-danger) 8%, transparent);
  color: var(--wb-danger);
}
.wb-diffcard-act svg {
  width: 13px;
  height: 13px;
}
.wb-diffcard-bd {
  border-top: 1px solid var(--dc-border);
  animation: wb-diffcard-in 0.18s ease-out both;
}
.wb-diffcard-hint {
  padding: 8px 10px;
  font-size: 11px;
  color: var(--wb-muted);
}
@keyframes wb-diffcard-in {
  from {
    opacity: 0;
    transform: translateY(-2px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
