<script setup lang="ts">
// 引擎提问卡：PI 里的扩展（含内置提问工具）在等用户回答。
// 三种方法各有对应的交互：确认 / 单选 / 填空。
import { ref } from 'vue'
import { useChatStore, type PendingAsk } from '../../stores/chat'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{ ask: PendingAsk }>()
const chat = useChatStore()
const text = ref('')
const busy = ref(false)

async function submit(value: string, confirmed: boolean, cancelled = false) {
  if (busy.value) return
  busy.value = true
  try {
    await chat.answer(props.ask, value, confirmed, cancelled)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="ask">
    <div class="ak-head">
      <span class="ak-ic"><AppIcon name="help" size="ic-sm" /></span>
      <div class="ak-title">
        <b>{{ ask.title || '助手在问你一件事' }}</b>
        <span v-if="ask.message" class="ak-msg">{{ ask.message }}</span>
      </div>
    </div>

    <!-- confirm：两个明确的选择，拒绝永远安全 -->
    <div v-if="ask.method === 'confirm'" class="ak-acts">
      <button class="btn btn-sm btn-primary" type="button" :class="{ 'is-loading': busy }" :disabled="busy" @click="submit('', true)">
        允许
      </button>
      <button class="btn btn-sm" type="button" :disabled="busy" @click="submit('', false)">拒绝</button>
    </div>

    <!-- select：选项本身就是答案 -->
    <div v-else-if="ask.method === 'select'" class="ak-acts wrap">
      <button
        v-for="opt in ask.options"
        :key="opt"
        class="btn btn-sm"
        type="button"
        :disabled="busy"
        @click="submit(opt, true)"
      >
        {{ opt }}
      </button>
      <button class="btn btn-sm ghost" type="button" :disabled="busy" @click="submit('', false, true)">跳过</button>
    </div>

    <!-- input / editor：一个输入框 + 提交 -->
    <div v-else class="ak-input">
      <input
        v-model="text"
        class="input"
        :placeholder="ask.method === 'editor' ? '写下你的回答' : '输入回答后回车'"
        @keydown.enter.prevent="submit(text, true)"
      />
      <button class="btn btn-sm btn-primary" type="button" :class="{ 'is-loading': busy }" :disabled="busy || !text.trim()" @click="submit(text, true)">
        回答
      </button>
      <button class="btn btn-sm ghost" type="button" :disabled="busy" @click="submit('', false, true)">取消</button>
    </div>
  </div>
</template>

<style scoped>
.ask {
  border: 1px solid var(--wb-border);
  border-left: 2px solid var(--wb-primary);
  border-radius: var(--wb-radius);
  background: var(--wb-surface);
  padding: var(--wb-sp-3) var(--wb-sp-4);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
  box-shadow: var(--wb-shadow-card);
}
.ak-head {
  display: flex;
  gap: var(--wb-sp-3);
  align-items: flex-start;
}
.ak-ic {
  width: var(--wb-tile-sm);
  height: var(--wb-tile-sm);
  border-radius: var(--wb-radius-sm);
  display: grid;
  place-items: center;
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
  flex: none;
}
.ak-title {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}
.ak-title b {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
  font-weight: 600;
}
.ak-msg {
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-2);
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 180px;
  overflow: auto;
}
.ak-acts {
  display: flex;
  gap: var(--wb-sp-2);
}
.ak-acts.wrap {
  flex-wrap: wrap;
}
.ak-input {
  display: flex;
  gap: var(--wb-sp-2);
  align-items: center;
}
.ak-input .input {
  flex: 1;
  height: var(--wb-ctl-h);
}
</style>
