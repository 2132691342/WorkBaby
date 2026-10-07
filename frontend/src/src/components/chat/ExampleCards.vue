<script setup lang="ts">
// 首屏引导：点一下就把例句发出去，降低「不知道说什么」的门槛。
import AppIcon from '../common/AppIcon.vue'

const emit = defineEmits<{ pick: [text: string] }>()

const examples = [
  { icon: 'doc', title: '帮我读文件', text: '帮我看看当前工作目录里有哪些文件，挑最重要的一个给我讲讲' },
  { icon: 'table', title: '处理表格', text: '把桌面上的 Excel 按月份汇总一下，生成一份统计报告' },
  { icon: 'edit-pen', title: '写点东西', text: '帮我写一封请假邮件，语气礼貌一点' },
  { icon: 'search', title: '查资料', text: '帮我查一下今天的新闻，挑三条有意思的讲给我听' },
]
</script>

<template>
  <div class="ex-grid">
    <button v-for="ex in examples" :key="ex.title" class="ex-card" type="button" @click="emit('pick', ex.text)">
      <span class="ex-ic"><AppIcon :name="ex.icon" size="ic-lg" /></span>
      <span class="ex-t">{{ ex.title }}</span>
      <span class="ex-s">{{ ex.text }}</span>
    </button>
  </div>
</template>

<style scoped>
/* 例句卡按案例的卡片语言：圆角大一点、图标成「瓦片」、标题用主色。
   五态齐全：默认 / 悬停上浮 / 按下回落 / 焦点环 / 禁用（继承浏览器默认禁用）。 */
.ex-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--wb-sp-3);
  width: min(640px, 100%);
}
.ex-card {
  text-align: left;
  display: grid;
  gap: 3px;
  padding: var(--wb-sp-4);
  border-radius: var(--wb-radius-lg);
  background: var(--wb-surface);
  border: 1px solid var(--wb-border);
  cursor: pointer;
  transition:
    border-color var(--wb-dur) var(--wb-ease),
    background var(--wb-dur) var(--wb-ease),
    box-shadow var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
  animation: ex-rise var(--wb-dur-slow) var(--wb-ease) backwards;
}
.ex-card:nth-child(2) {
  animation-delay: 60ms;
}
.ex-card:nth-child(3) {
  animation-delay: 120ms;
}
.ex-card:nth-child(4) {
  animation-delay: 180ms;
}
@keyframes ex-rise {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
}
.ex-card:hover:not(:disabled) {
  border-color: var(--wb-primary-line);
  background: var(--wb-surface);
  box-shadow: var(--wb-shadow-2);
  transform: translateY(-2px) scale(1.02);
}
.ex-card:active:not(:disabled) {
  transform: scale(0.98);
}
.ex-card:focus-visible {
  outline: 2px solid var(--wb-primary);
  outline-offset: 2px;
}
.ex-ic {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  margin-bottom: var(--wb-sp-2);
  border-radius: var(--wb-radius);
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
}
.ex-t {
  font-weight: 600;
  color: var(--wb-primary);
  font-size: var(--wb-fs-md);
}
.ex-s {
  color: var(--wb-muted);
  font-size: var(--wb-fs-sm);
  line-height: var(--wb-lh-base);
}
</style>
