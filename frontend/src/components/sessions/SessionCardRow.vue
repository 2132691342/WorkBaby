<script setup lang="ts">
// 会话列表的单行卡片：标题、元信息、悬停动作。
// 从 SessionList 拆出来只为守住 500 行上限：它自己不拉数据，
// 相对时间 / 高亮 / 动作全部由父组件传函数或事件进来。
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import type { SessionRow } from '@/types/api'

const props = defineProps<{
  row: SessionRow
  /** 当前打开的会话路径：命中的那一行亮起来 */
  active: string
  /** 对话页窄栏形态：动作收成图标、藏路径 */
  compact: boolean
  /** 标题高亮片段：全文命中但标题不含词时就是原样一段 */
  segments: { text: string; hit: boolean }[]
  /** 「3 天前」这类相对时间 */
  relative: string
  /** 悬停里的完整时刻 */
  stamp: string
}>()

const emit = defineEmits<{
  open: [path: string]
  rename: [row: SessionRow]
  remove: [row: SessionRow]
}>()

const { t } = useI18n()
</script>

<template>
  <div
    class="session-row wb-row-click"
    :class="{ 'is-active': props.row.session_path === props.active }"
    @click="emit('open', props.row.session_path)"
  >
    <div class="row-top">
      <!-- 标题按过滤词高亮：搜到的那一段要能一眼看见，否则还要自己找 -->
      <span class="row-title" :title="props.row.session_title">
        <span v-for="(seg, si) in props.segments" :key="si" :class="{ 'is-hit': seg.hit }">{{ seg.text }}</span>
      </span>
      <span class="row-time" :title="props.stamp">{{ props.relative }}</span>
    </div>
    <div class="row-meta">
      <span v-if="props.row.session_model" class="row-model">{{ props.row.session_model }}</span>
      <span class="row-dot">·</span>
      <span>{{ props.row.message_count }} {{ t('sessions.messages') }}</span>
      <span v-if="props.row.cost_usd" class="row-dot">·</span>
      <span v-if="props.row.cost_usd">${{ props.row.cost_usd.toFixed(3) }}</span>
      <!-- 动作收在元信息行尾：常驻占位、悬停淡入。放独立行会撑高行高，
           鼠标一扫一排会话全在抖。窄栏用图标，主栏用文字。 -->
      <div class="row-actions">
        <button
          class="wb-btn text sm"
          :title="t('sessions.rename')"
          :aria-label="`${t('sessions.rename')} ${props.row.session_title}`"
          @click.stop="emit('rename', props.row)"
        >
          <AppIcon v-if="props.compact" name="edit" :size="13" />
          <template v-else>{{ t('sessions.rename') }}</template>
        </button>
        <button
          class="wb-btn text sm danger"
          :title="t('common.delete')"
          :aria-label="`${t('common.delete')} ${props.row.session_title}`"
          @click.stop="emit('remove', props.row)"
        >
          <AppIcon v-if="props.compact" name="trash" :size="13" />
          <template v-else>{{ t('common.delete') }}</template>
        </button>
      </div>
    </div>
    <div v-if="!props.compact" class="row-path">{{ props.row.session_cwd }}</div>
  </div>
</template>

<style scoped>
.session-row {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  cursor: pointer;
  position: relative;
}

.row-top {
  display: flex;
  align-items: baseline;
  gap: var(--wb-sp-2);
}

.row-title {
  flex: 1;
  min-width: 0;
  font-size: var(--wb-fs-sm);
  font-weight: 550;
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 搜索命中：淡品牌底 + 主色字。不用下划线——等宽小字里下划线会与
   路径、ID 的下划线语法混在一起。 */
.row-title .is-hit {
  border-radius: 2px;
  padding: 0 1px;
  background: var(--wb-brand-soft);
  color: var(--wb-brand);
  font-weight: 700;
}

.row-time {
  flex: none;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}

.row-meta {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-1);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-mute);
  overflow: hidden;
  white-space: nowrap;
}

.row-model {
  max-width: 150px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.row-dot {
  color: var(--wb-faint);
}

.row-path {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 动作常驻占位、悬停/键盘聚焦淡入：display 切换会让行高突变，
   鼠标扫过列表时一排行跟着跳。 */
.row-actions {
  display: flex;
  gap: var(--wb-sp-1);
  margin-left: auto;
  flex: none;
  opacity: 0;
  transform: translateX(4px);
  pointer-events: none;
  transition:
    opacity var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

.session-row:hover .row-actions,
.session-row:focus-within .row-actions {
  opacity: 1;
  transform: none;
  pointer-events: auto;
}

/* 行内的文字按钮补一层淡底：全局 .wb-btn.text 只换色，
   而会话行是密集列表——没有底色时鼠标扫过看不出哪个按钮被指着。 */
.row-actions .wb-btn.text:hover:not(:disabled) {
  background: var(--wb-hover);
  border-radius: var(--wb-r-xs);
}
.row-actions .wb-btn.text.danger:hover:not(:disabled) {
  background: var(--wb-bad-soft);
}
</style>
