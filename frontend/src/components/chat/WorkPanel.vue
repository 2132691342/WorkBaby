<script setup lang="ts">
// 右侧常驻工作台：多标签并排留存一次会话里看过的文件差异 / 内容。
// 与抽屉的区别：不遮消息、不丢滚动位置，拖宽持久化；聊天入口开标签即激活。
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import FileInspect from '@/components/workspace/FileInspect.vue'
import { useWorkPanel } from '@/composables/useWorkPanel'

const { t } = useI18n()
const panel = useWorkPanel()

// 拖宽：左缘热区按下后横向移动改宽度（向左拖变宽），松手即持久化。
const dragging = ref(false)
let startX = 0
let startWidth = 0

function onResizeDown(e: PointerEvent): void {
  dragging.value = true
  startX = e.clientX
  startWidth = panel.width.value
  ;(e.target as HTMLElement).setPointerCapture(e.pointerId)
}
function onResizeMove(e: PointerEvent): void {
  if (!dragging.value) return
  panel.setWidth(startWidth + (startX - e.clientX))
}

function onMode(mode: 'diff' | 'file'): void {
  const tab = panel.active.value
  if (tab) panel.setMode(tab.id, mode)
}

function onResizeUp(): void {
  dragging.value = false
}

function closeAll(): void {
  for (const tab of [...panel.tabs.value]) panel.closeTab(tab.id)
}
</script>

<template>
  <!-- 开合过渡走 wb-fade（淡入淡出 + 6px 轻浮）：面板不是抽屉，不做滑入滑出。 -->
  <Transition name="wb-fade">
    <aside v-if="panel.open.value" class="work-panel" :style="{ width: panel.width.value + 'px' }">
    <div
      class="wp-resize"
      :class="{ 'is-drag': dragging }"
      role="separator"
      aria-orientation="vertical"
      :title="t('chat.wpResize')"
      @pointerdown="onResizeDown"
      @pointermove="onResizeMove"
      @pointerup="onResizeUp"
      @pointercancel="onResizeUp"
    />

    <div class="wp-head">
      <AppIcon name="workspace" :size="14" />
      <span class="wp-title">{{ t('chat.wpTitle') }}</span>
      <span class="wp-count">{{ panel.tabs.value.length }}</span>
      <button type="button" class="wb-btn icon ghost wp-closeall" :title="t('chat.wpCloseAll')" :aria-label="t('chat.wpCloseAll')" @click="closeAll">
        <AppIcon name="trash" :size="13" />
      </button>
    </div>

    <div class="wp-tabs" role="tablist">
      <div
        v-for="tab in panel.tabs.value"
        :key="tab.id"
        class="wp-tab"
        :class="{ active: tab.id === panel.activeId.value }"
        role="tab"
        :aria-selected="tab.id === panel.activeId.value"
        :title="tab.path"
        @click="panel.activeId.value = tab.id"
      >
        <AppIcon :name="tab.mode === 'diff' ? 'git' : 'file'" :size="12" />
        <span class="wp-tab-name">{{ tab.title }}</span>
        <button type="button"
          class="wp-tab-x wb-pressable"
          :title="t('chat.wpCloseTab')"
          @click.stop="panel.closeTab(tab.id)"
        >
          <AppIcon name="close" :size="11" />
        </button>
      </div>
    </div>

    <FileInspect
      v-if="panel.active.value"
      :path="panel.active.value.path"
      :mode="panel.active.value.mode"
      @update:mode="onMode"
    />
  </aside>
  </Transition>
</template>

<style scoped>
.work-panel {
  position: relative;
  flex: none;
  display: flex;
  flex-direction: column;
  min-height: 0;
  border-left: 1px solid var(--wb-line);
  background: var(--wb-window);
}

/* 左缘拖宽热区：平时隐形，悬停 / 拖拽时显一条竖条 */
.wp-resize {
  position: absolute;
  left: -3px;
  top: 0;
  bottom: 0;
  width: 6px;
  cursor: col-resize;
  z-index: 5;
}
.wp-resize::after {
  content: '';
  position: absolute;
  left: 2px;
  top: 50%;
  transform: translateY(-50%);
  width: 2px;
  height: var(--wb-ctl-icon);
  border-radius: var(--wb-r-full);
  background: var(--wb-line-strong);
  opacity: 0;
  transition: opacity var(--wb-dur-fast) var(--wb-ease);
}
.wp-resize:hover::after,
.wp-resize.is-drag::after {
  opacity: 1;
  background: var(--wb-brand);
}

.wp-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-bottom: 1px solid var(--wb-line);
  color: var(--wb-mute);
}
.wp-title {
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  color: var(--wb-ink);
}
.wp-count {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}
.wp-closeall {
  margin-left: auto;
}

.wp-tabs {
  display: flex;
  gap: 2px;
  overflow-x: auto;
  padding: var(--wb-sp-2) var(--wb-sp-2) 0;
  border-bottom: 1px solid var(--wb-line);
  scrollbar-width: none;
}
.wp-tabs::-webkit-scrollbar {
  display: none;
}

.wp-tab {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  flex: none;
  max-width: 160px;
  padding: 5px 8px;
  border: 1px solid transparent;
  border-bottom: none;
  border-radius: var(--wb-r-md) var(--wb-r-md) 0 0;
  color: var(--wb-mute);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  cursor: pointer;
  transition:
    color var(--wb-dur-fast) var(--wb-ease),
    background var(--wb-dur-fast) var(--wb-ease);
}
.wp-tab:hover {
  color: var(--wb-ink);
  background: var(--wb-hover);
}
.wp-tab.active {
  color: var(--wb-brand);
  background: var(--wb-surface);
  border-color: var(--wb-line);
}
.wp-tab-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
/* 缩放交给 .wb-pressable（小按钮基类）；这里只管换色 */
.wp-tab-x {
  display: inline-flex;
  border: none;
  background: transparent;
  color: var(--wb-faint);
  padding: 1px;
  border-radius: var(--wb-r-xs);
}
.wp-tab-x:hover {
  color: var(--wb-bad);
  background: var(--wb-hover);
}
</style>