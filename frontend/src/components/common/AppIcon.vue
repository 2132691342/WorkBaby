<script setup lang="ts">
// 图标：内联 SVG，24 网格。禁止用 Unicode 字符或 emoji 冒充图标
// （字体不同机器上渲染不同，且无法跟随 currentColor 与笔画重量）。
export type IconName =
  | 'chat'
  | 'clock'
  | 'cpu'
  | 'spark'
  | 'plug'
  | 'chart'
  | 'gear'
  | 'plus'
  | 'send'
  | 'stop'
  | 'close'
  | 'check'
  | 'alert'
  | 'refresh'
  | 'folder'
  | 'trash'
  | 'edit'
  | 'download'
  | 'chevron'
  | 'dice'
  | 'brain'
  | 'book'
  | 'search'
  | 'file'
  | 'arrow-left'
  | 'git'
  | 'branch'
  | 'commit'
  | 'workspace'
  | 'box'
  | 'gauge'
  | 'server'
  | 'eye'
  | 'link'
  | 'terminal'

withDefaults(defineProps<{ name: IconName; size?: number }>(), { size: 16 })
</script>

<template>
  <svg
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    :stroke-width="1.7"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
    class="icon"
  >
    <template v-if="name === 'chat'">
      <path d="M4 6.5A2.5 2.5 0 0 1 6.5 4h11A2.5 2.5 0 0 1 20 6.5v8A2.5 2.5 0 0 1 17.5 17H9l-4 3.2z" />
      <path d="M8.5 9h7M8.5 12.5h4" />
    </template>
    <template v-else-if="name === 'clock'">
      <circle cx="12" cy="12" r="8" />
      <path d="M12 7.5V12l3 2" />
    </template>
    <template v-else-if="name === 'cpu'">
      <rect x="6.5" y="6.5" width="11" height="11" rx="2" />
      <path d="M10 3v3M14 3v3M10 18v3M14 18v3M3 10h3M3 14h3M18 10h3M18 14h3" />
    </template>
    <template v-else-if="name === 'spark'">
      <path d="M12 3.5 13.9 9l5.6 1.9-5.6 1.9L12 18.4l-1.9-5.6L4.5 10.9 10.1 9z" />
    </template>
    <template v-else-if="name === 'plug'">
      <path d="M9 3v5M15 3v5" />
      <path d="M6.5 8h11v3.5a5.5 5.5 0 0 1-11 0z" />
      <path d="M12 17v4" />
    </template>
    <template v-else-if="name === 'chart'">
      <path d="M4 20V4M4 20h16" />
      <path d="M8 16v-4M12.5 16V8M17 16v-6" />
    </template>
    <template v-else-if="name === 'gear'">
      <circle cx="12" cy="12" r="3" />
      <path d="M12 3.5v2M12 18.5v2M20.5 12h-2M5.5 12h-2M18 6l-1.4 1.4M7.4 16.6 6 18M18 18l-1.4-1.4M7.4 7.4 6 6" />
    </template>
    <template v-else-if="name === 'plus'">
      <path d="M12 5v14M5 12h14" />
    </template>
    <template v-else-if="name === 'send'">
      <path d="M4.5 12 20 4.5 15.5 20l-3.7-6.4z" />
    </template>
    <template v-else-if="name === 'stop'">
      <rect x="7" y="7" width="10" height="10" rx="2" />
    </template>
    <template v-else-if="name === 'close'">
      <path d="M6 6l12 12M18 6 6 18" />
    </template>
    <template v-else-if="name === 'check'">
      <path d="M5 12.5 9.5 17 19 7" />
    </template>
    <template v-else-if="name === 'alert'">
      <path d="M12 4.5 21 19H3z" />
      <path d="M12 10v4M12 16.6v.4" />
    </template>
    <template v-else-if="name === 'refresh'">
      <path d="M20 12a8 8 0 1 1-2.6-5.9" />
      <path d="M20 4v4h-4" />
    </template>
    <template v-else-if="name === 'folder'">
      <path d="M3.5 7.5A2 2 0 0 1 5.5 5.5H10l2 2.5h6.5a2 2 0 0 1 2 2v6a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2z" />
    </template>
    <template v-else-if="name === 'trash'">
      <path d="M4.5 7h15M9.5 7V5h5v2M6.5 7l1 12.5h9L17.5 7" />
      <path d="M10 11v5M14 11v5" />
    </template>
    <template v-else-if="name === 'edit'">
      <path d="M4 20h4l10.5-10.5a2.1 2.1 0 0 0-3-3L5 17z" />
      <path d="M14 6.5 17.5 10" />
    </template>
    <template v-else-if="name === 'download'">
      <path d="M12 4v10M8 10.5 12 15l4-4.5" />
      <path d="M4.5 19.5h15" />
    </template>
    <template v-else-if="name === 'chevron'">
      <path d="M9 6l6 6-6 6" />
    </template>
    <template v-else-if="name === 'dice'">
      <rect x="4.5" y="4.5" width="15" height="15" rx="3" />
      <circle cx="9" cy="9" r="1.1" fill="currentColor" stroke="none" />
      <circle cx="15" cy="15" r="1.1" fill="currentColor" stroke="none" />
      <circle cx="15" cy="9" r="1.1" fill="currentColor" stroke="none" />
      <circle cx="9" cy="15" r="1.1" fill="currentColor" stroke="none" />
    </template>
    <template v-else-if="name === 'brain'">
      <path d="M9.5 4.5a2.6 2.6 0 0 0-2.6 2.6c0 .5.1 1 .4 1.4A2.9 2.9 0 0 0 5.5 11a2.9 2.9 0 0 0 1.7 2.6 2.7 2.7 0 0 0 2.3 4.3V4.5z" />
      <path d="M14.5 4.5a2.6 2.6 0 0 1 2.6 2.6c0 .5-.1 1-.4 1.4A2.9 2.9 0 0 1 18.5 11a2.9 2.9 0 0 1-1.7 2.6 2.7 2.7 0 0 1-2.3 4.3V4.5z" />
    </template>
    <template v-else-if="name === 'book'">
      <path d="M4.5 5.5A2 2 0 0 1 6.5 4H19v15H6.5a2 2 0 0 0-2 2z" />
      <path d="M4.5 5.5V19" />
    </template>
    <template v-else-if="name === 'search'">
      <circle cx="10.5" cy="10.5" r="6" />
      <path d="M15 15l4.5 4.5" />
    </template>
    <template v-else-if="name === 'file'">
      <path d="M6.5 3.5h7L18 8v12.5h-11.5z" />
      <path d="M13 3.5V8h5" />
    </template>
    <template v-else-if="name === 'arrow-left'">
      <path d="M15 5.5 8.5 12l6.5 6.5" />
    </template>
    <template v-else-if="name === 'git'">
      <circle cx="7" cy="6" r="2" />
      <circle cx="7" cy="18" r="2" />
      <circle cx="17" cy="12" r="2" />
      <path d="M7 8v8M9 6.8h4.2A3.8 3.8 0 0 1 17 10.2" />
    </template>
    <template v-else-if="name === 'branch'">
      <circle cx="7" cy="6" r="2" />
      <circle cx="7" cy="18" r="2" />
      <circle cx="17" cy="8" r="2" />
      <path d="M7 8v8M17 10v1a4 4 0 0 1-4 4H9" />
    </template>
    <template v-else-if="name === 'commit'">
      <circle cx="12" cy="12" r="3.4" />
      <path d="M3 12h5.6M15.4 12H21" />
    </template>
    <template v-else-if="name === 'box'">
      <path d="M12 3.5 20 8v8l-8 4.5L4 16V8z" />
      <path d="M4 8l8 4.5L20 8M12 12.5V20" />
    </template>
    <template v-else-if="name === 'workspace'">
      <rect x="3.5" y="4.5" width="17" height="15" rx="2" />
      <path d="M3.5 9.5h17M9 9.5v10" />
    </template>
    <template v-else-if="name === 'gauge'">
      <path d="M4 17a8 8 0 1 1 16 0" />
      <path d="M12 17l4-5" />
      <circle cx="12" cy="17" r="1.2" fill="currentColor" stroke="none" />
    </template>
    <template v-else-if="name === 'server'">
      <rect x="3.5" y="4" width="17" height="6" rx="1.6" />
      <rect x="3.5" y="14" width="17" height="6" rx="1.6" />
      <path d="M7 7h.01M7 17h.01" />
    </template>
    <template v-else-if="name === 'eye'">
      <path d="M2.5 12S6 6.5 12 6.5 21.5 12 21.5 12 18 17.5 12 17.5 2.5 12 2.5 12z" />
      <circle cx="12" cy="12" r="2.6" />
    </template>
    <template v-else-if="name === 'link'">
      <path d="M10 13.5a3.5 3.5 0 0 0 5 0l3-3a3.5 3.5 0 0 0-5-5l-1.2 1.2" />
      <path d="M14 10.5a3.5 3.5 0 0 0-5 0l-3 3a3.5 3.5 0 0 0 5 5l1.2-1.2" />
    </template>
    <template v-else-if="name === 'terminal'">
      <rect x="2.5" y="4" width="19" height="16" rx="2" />
      <path d="M7 9.5l3 2.5-3 2.5" />
      <path d="M12.5 15h5" />
    </template>
  </svg>
</template>

<style scoped>
.icon {
  flex: none;
  display: block;
}
</style>
