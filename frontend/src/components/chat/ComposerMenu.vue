<script setup lang="ts">
// 输入区的两个弹出菜单：`/` 命令 与 `@` 文件引用。
// 键盘高亮自己算 scrollTop（scrollIntoView 会把外层页面一起滚）、
// 菜单内滚动 / 点选项不让输入框丢焦点（焦点一丢 ↑↓ 与回车就失效）——必须自己实现。
import { nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import MentionBrowser from '@/components/chat/MentionBrowser.vue'
import type { MenuItem } from '@/composables/useComposerMenu'

const props = defineProps<{
  kind: 'slash' | 'mention'
  /** 斜杠菜单的候选项 */
  items: MenuItem[]
  /** 高亮到第几项 */
  index: number
}>()

const emit = defineEmits<{
  pick: [item: MenuItem]
  hover: [index: number]
  close: []
  /** @ 菜单选中了一个文件（相对路径 + 展示名） */
  pickFile: [relPath: string, name: string]
}>()

const { t } = useI18n()
const listEl = ref<HTMLElement | null>(null)
/** @ 菜单的浏览器：它的键盘导航在它内部实现，这一层只把按键转下去。 */
const mentionRef = ref<{ move: (d: number) => void; pickCurrent: () => boolean } | null>(null)

defineExpose({
  /** 键盘导航转给 @ 浏览器：列表在它那里，下标也在那里。 */
  move: (delta: number) => mentionRef.value?.move(delta),
  pickCurrent: () => mentionRef.value?.pickCurrent() ?? false,
})

// 键盘移动之后要把高亮项带进可视区。
// index 变化由父组件触发（按 ↑↓），这里只负责让它真的看得见。
watch(
  () => props.index,
  (i) => void nextTick(() => ensureVisible(i)),
)

/**
 * 把第 i 项带进可视区。
 *
 * 用容器自己的 scrollTop 算，而不是 el.scrollIntoView()：
 * 后者会滚动所有可滚动的祖先，于是「菜单里按一下 ↓」把整个聊天区
 * 一起挪走了 —— 用户看到的是页面跳了一下，而高亮项还没对准。
 */
function ensureVisible(i: number): void {
  const box = listEl.value
  if (!box) return
  const el = box.querySelector<HTMLElement>(`[data-idx="${i}"]`)
  if (!el) return
  const top = el.offsetTop - box.offsetTop
  const bottom = top + el.offsetHeight
  if (top < box.scrollTop) box.scrollTop = top
  else if (bottom > box.scrollTop + box.clientHeight) box.scrollTop = bottom - box.clientHeight
}

/** @ 菜单选中了一个文件：原样交上去。 */
function onMentionPick(relPath: string, name: string): void {
  emit('pickFile', relPath, name)
}

/**
 * 按住不放：不要让输入框丢焦点。
 *
 * 一旦焦点跑到按钮上，↑↓ 与回车就不再送到输入框，
 * 键盘导航整套失效（keepFocus 专门挡掉这个坑）。
 */
function keepFocus(event: PointerEvent): void {
  const el = event.target as HTMLElement | null
  if (el?.closest('input, textarea')) return
  event.preventDefault()
}

// 键盘导航与悬停选择会打架：按 ↓ 让列表滚动，滚动把新的一行带到静止的
// 指针底下，mouseenter 把高亮拽回去，列表看起来就「不跟着光标滚」。
// 判据：滚动引发的 mouseenter 里指针坐标没变过——指针真的动了才算悬停选择。
let lastHoverX = -1
let lastHoverY = -1

function onHover(index: number, event: MouseEvent): void {
  const moved = event.screenX !== lastHoverX || event.screenY !== lastHoverY
  lastHoverX = event.screenX
  lastHoverY = event.screenY
  if (moved) emit('hover', index)
}

/** 来源标签：内置由界面直接执行，引擎的只是填进输入框。 */
function tagClass(source: MenuItem['source']): string {
  return source === 'builtin' ? 'is-builtin' : 'is-engine'
}
</script>

<template>
  <!-- 菜单贴着输入框上沿往上弹，左右各留一点边距，
       弹面板用 pointerdown 保焦：按钮按下的一瞬间就 preventDefault。 -->
  <div class="pop menu-pop" @pointerdown="keepFocus">
    <!-- listEl 是 ensureVisible 的滚动容器：高亮要跟手，全靠量它自己的 scrollTop -->
    <div v-if="props.kind === 'slash'" ref="listEl" class="pop-list">
      <!-- 没有匹配项时也要给一句话：菜单默默消失看起来像按了没反应 -->
      <p v-if="!props.items.length" class="pop-hint pop-empty">{{ t('chat.slashNoMatch') }}</p>
      <button
        v-for="(item, index) in props.items"
        :key="item.source + item.key"
        :data-idx="index"
        type="button"
        class="pop-row"
        :class="{ 'is-hi': index === props.index }"
        @mouseenter="onHover(index, $event)"
        @click="emit('pick', item)"
      >
        <span class="pop-grow">
          <b class="pop-t">/{{ item.name }}</b>
          <span class="pop-d">{{ item.desc }}</span>
        </span>
        <span class="pop-tag" :class="tagClass(item.source)">
          {{ item.source === 'builtin' ? t('chat.slashBuiltin') : t('chat.slashEngine') }}
        </span>
      </button>
      <p class="pop-hint">{{ t('chat.slashHint') }}</p>
    </div>

    <!-- MentionBrowser 的 pick 带两个参数（相对路径 + 展示名），
         模板里的 $event 只能接到第一个，所以走一个具名处理函数。 -->
    <MentionBrowser v-else ref="mentionRef" @pick="onMentionPick" />
  </div>
</template>

<style scoped>
.pop {
  position: absolute;
  left: 0;
  right: 0;
  bottom: calc(100% - var(--wb-sp-2));
  max-height: 340px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  padding: var(--wb-sp-1);
  border: 1px solid var(--wb-line-strong);
  border-radius: var(--wb-r-lg);
  background: var(--wb-surface);
  box-shadow: var(--wb-shadow-lift);
  animation: pop-in var(--wb-dur-fast) var(--wb-ease);
  z-index: 20;
}

@keyframes pop-in {
  from {
    opacity: 0;
    transform: translateY(4px);
  }
}

/* 滚动在列表层而不是弹层上：上面 ensureVisible 要靠它自己的 scrollTop 算可视区 */
.pop-list {
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
}

.pop-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  width: 100%;
  padding: 5px var(--wb-sp-2);
  border: 0;
  border-radius: var(--wb-r-sm);
  background: transparent;
  color: var(--wb-ink-2);
  font: inherit;
  text-align: left;
  cursor: pointer;
  transition:
    background-color var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease);
}

.pop-row:hover,
.pop-row.is-hi {
  background: var(--wb-brand-soft);
  color: var(--wb-brand);
}

.pop-grow {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
  flex: 1;
}

.pop-t {
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-xs);
  font-weight: 650;
  color: inherit;
}

.pop-d {
  font-size: var(--wb-fs-3xs);
  color: var(--wb-mute);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pop-tag {
  flex: none;
  font-size: var(--wb-fs-3xs);
  padding: 0 5px;
  border-radius: var(--wb-r-full);
}

/* 内置与引擎两套来源要能一眼分开：内置是直接执行的动作，
   引擎命令只是把名字填进输入框，两者的后果不同。 */
.pop-tag.is-builtin {
  background: var(--wb-brand-soft);
  color: var(--wb-brand);
}

.pop-tag.is-engine {
  border: 1px solid var(--wb-line);
  color: var(--wb-mute);
}

.pop-hint {
  margin: var(--wb-sp-1) 0 0;
  padding: 2px var(--wb-sp-2) 0;
  border-top: 1px solid var(--wb-line);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  font-family: var(--wb-font-mono);
}

/* 「没有匹配」与底部的提示句不同：它居中，不带那条分隔线 */
.pop-empty {
  margin: 0;
  padding: var(--wb-sp-4) var(--wb-sp-3);
  border-top: 0;
  text-align: center;
  font-family: var(--wb-font-sans);
}

/* @ 菜单有自己的头部与搜索框，不需要 pop-list 那层包裹 */
.menu-pop:has(.mention) {
  padding: var(--wb-sp-1);
}
</style>
