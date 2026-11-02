<script setup lang="ts">
// 引擎扩展的状态条（setStatus）：贴输入框上沿排一行。
// 它们是引擎在报「我正在干什么」，属于运行环境的一部分，不该混进消息流里当正文。
// 结构化小部件（待办 / 计划）由 FeatureDock 的面板渲染，这里只留一行状态文字。
import { computed } from 'vue'
import { useChatStore } from '../../stores/chat'

const chat = useChatStore()

const statusRows = computed(() => Object.entries(chat.engineStatus))
const visible = computed(() => statusRows.value.length > 0)
</script>

<template>
  <Transition name="efx-fade">
    <div v-if="visible" class="efx-bar">
      <span v-for="[k, text] in statusRows" :key="`s-${k}`" class="efx-chip" :title="text">
        <span class="efx-dot" aria-hidden="true" />
        <span class="efx-txt">{{ text }}</span>
      </span>
    </div>
  </Transition>
</template>

<style scoped>
.efx-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--wb-sp-2);
  padding: 0 var(--wb-sp-4) var(--wb-sp-2);
  min-width: 0;
}
.efx-chip {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-2);
  max-width: 100%;
  height: calc(22px * var(--wb-vw-scale));
  padding: 0 var(--wb-sp-2);
  border-radius: var(--wb-radius-full);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-hover);
  font-size: var(--wb-fs-2xs);
  font-weight: 600;
  color: var(--wb-ink-2);
  min-width: 0;
}
.efx-dot {
  width: 5px;
  height: 5px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-live);
  animation: efx-breathe 1.6s var(--wb-ease) infinite;
  flex: none;
}
@keyframes efx-breathe {
  0%,
  100% {
    opacity: 0.4;
  }
  50% {
    opacity: 1;
  }
}
.efx-txt {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
.efx-fade-enter-active,
.efx-fade-leave-active {
  transition:
    opacity var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.efx-fade-enter-from,
.efx-fade-leave-to {
  opacity: 0;
  transform: translateY(4px);
}
</style>
