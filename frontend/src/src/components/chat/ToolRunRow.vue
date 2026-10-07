<script setup lang="ts">
// 工具执行卡：折叠态显示一行摘要，展开看参数与输出。
import { ref } from 'vue'
import { summarizeArgs } from '../../utils/md'
import AppIcon from '../common/AppIcon.vue'

defineProps<{
  label: string
  args?: Record<string, unknown>
  running?: boolean
  ok?: boolean
  title?: string
  output?: string
  duration_ms?: number
}>()

const open = ref(false)
</script>

<template>
  <div class="tool" :class="{ open }">
    <button class="tool-hd" type="button" @click="open = !open">
      <span class="st" :class="running ? 'is-run' : ok === false ? 'is-bad' : 'is-ok'">
        <AppIcon v-if="running" name="loader" size="ic-xs" spin />
        <AppIcon v-else-if="ok === false" name="close" size="ic-xs" />
        <AppIcon v-else name="check" size="ic-xs" />
      </span>
      <span class="nm">{{ label }}</span>
      <span class="arg">{{ summarizeArgs(args) }}</span>
      <span v-if="duration_ms" class="ms">{{ Math.round(duration_ms / 100) / 10 }}s</span>
      <AppIcon class="chev" name="chevron-right" size="ic-sm" />
    </button>
    <div v-if="open" class="tool-bd">
      <template v-if="args && Object.keys(args).length">
        <div class="lb">参数</div>
        <pre><code>{{ JSON.stringify(args, null, 2) }}</code></pre>
      </template>
      <template v-if="output">
        <div class="lb">{{ title || '输出' }}</div>
        <pre :class="{ 'is-bad': ok === false }"><code>{{ output }}</code></pre>
      </template>
      <div v-if="!output && !running" class="lb is-dim">没有输出</div>
    </div>
  </div>
</template>

<style scoped>
.st {
  width: 16px;
  height: 16px;
  border-radius: var(--wb-radius-full);
  display: grid;
  place-items: center;
  font-size: var(--wb-fs-xs);
  flex: none;
}
.st.is-run {
  color: var(--wb-live);
  background: var(--wb-live-soft);
  animation: pulse 1.1s ease-in-out infinite;
}
.st.is-ok {
  color: var(--wb-success);
  background: var(--wb-success-soft);
}
.st.is-bad {
  color: var(--wb-danger);
  background: var(--wb-danger-soft);
}
@keyframes pulse {
  50% { opacity: 0.35; }
}
.is-dim { color: var(--wb-muted); }
pre.is-bad { color: var(--wb-danger); }
.tool.open .chev { transform: rotate(90deg); }
</style>
