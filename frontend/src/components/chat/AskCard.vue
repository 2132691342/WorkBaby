<script setup lang="ts">
// 引擎提问卡：PI 里的扩展（含内置提问工具）在等用户回答。
// 四种方法各有对应的交互：确认 / 单选列表 / 填空 / 多行编辑；全部走按钮五态基元。
import { computed, nextTick, onMounted, ref } from 'vue'
import { useChatStore, type PendingAsk } from '../../stores/chat'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{ ask: PendingAsk }>()
const chat = useChatStore()
const text = ref('')
const busy = ref(false)
// select 的键盘焦点：↑↓ 移动、回车选中，长列表可滚
const optIndex = ref(0)
const optList = ref<HTMLElement | null>(null)

const isEditor = computed(() => props.ask.method === 'editor')

async function submit(value: string, confirmed: boolean, cancelled = false) {
  if (busy.value) return
  busy.value = true
  try {
    await chat.answer(props.ask, value, confirmed, cancelled)
  } finally {
    busy.value = false
  }
}

function pickOption(opt: string) {
  optIndex.value = Math.max(0, props.ask.options.indexOf(opt))
  void submit(opt, true)
}

function onOptionKey(e: KeyboardEvent) {
  const n = props.ask.options.length
  if (!n) return
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    optIndex.value = (optIndex.value + 1) % n
    void scrollOption()
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    optIndex.value = (optIndex.value - 1 + n) % n
    void scrollOption()
  } else if (e.key === 'Enter' && !e.isComposing) {
    e.preventDefault()
    void submit(props.ask.options[Math.min(optIndex.value, n - 1)], true)
  }
}

async function scrollOption() {
  await nextTick()
  const el = optList.value?.querySelector<HTMLElement>(`.opt.is-hi`)
  el?.scrollIntoView({ block: 'nearest' })
}

onMounted(() => {
  if (props.ask.method === 'select') optIndex.value = 0
})
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

    <!-- select：选项列表（长列表可滚 + 键盘 ↑↓ + 回车），跳过永远可用 -->
    <div v-else-if="ask.method === 'select'" class="ak-select">
      <div ref="optList" class="opt-list" role="listbox" :aria-label="ask.title || '选项'" tabindex="0" @keydown="onOptionKey">
        <button
          v-for="(opt, i) in ask.options"
          :key="opt"
          class="opt"
          :class="{ 'is-hi': i === optIndex }"
          role="option"
          :aria-selected="i === optIndex"
          type="button"
          :disabled="busy"
          @mouseenter="optIndex = i"
          @click="pickOption(opt)"
        >
          <AppIcon name="chevron-right" size="ic-2xs" />
          <span class="opt-t">{{ opt }}</span>
        </button>
      </div>
      <div class="ak-acts">
        <button class="btn btn-sm ghost" type="button" :disabled="busy" @click="submit('', false, true)">跳过</button>
      </div>
    </div>

    <!-- editor：多行文本域（自动撑高），input：单行 -->
    <div v-else class="ak-input" :class="{ 'is-editor': isEditor }">
      <textarea
        v-if="isEditor"
        v-model="text"
        class="input ak-ta"
        rows="3"
        :placeholder="ask.method === 'editor' ? '写下你的回答，Ctrl+Enter 提交' : ''"
        @keydown.enter.ctrl.prevent="submit(text, true)"
      />
      <input
        v-else
        v-model="text"
        class="input"
        placeholder="输入回答后回车"
        @keydown.enter.prevent="submit(text, true)"
      />
      <div class="ak-acts">
        <button class="btn btn-sm btn-primary" type="button" :class="{ 'is-loading': busy }" :disabled="busy || !text.trim()" @click="submit(text, true)">
          回答
        </button>
        <button class="btn btn-sm ghost" type="button" :disabled="busy" @click="submit('', false, true)">取消</button>
      </div>
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
.ak-select {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.opt-list {
  display: flex;
  flex-direction: column;
  gap: 3px;
  max-height: 240px;
  overflow: auto;
  border-radius: var(--wb-radius-sm);
}
.opt {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: 7px var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  border: 1px solid transparent;
  color: var(--wb-ink-2);
  text-align: left;
  font-size: var(--wb-fs-sm);
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    border-color var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease);
}
.opt:hover:not(:disabled),
.opt.is-hi:not(:disabled) {
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
  border-color: var(--wb-primary-line);
}
.opt:active:not(:disabled) {
  transform: scale(0.99);
}
.opt:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.opt .ic {
  opacity: 0.6;
}
.opt-t {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ak-input {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.ak-input .input {
  width: 100%;
  height: var(--wb-ctl-h);
}
.ak-input.is-editor .ak-ta {
  height: auto;
  min-height: calc(72px * var(--wb-vw-scale));
  resize: vertical;
  line-height: var(--wb-lh-base);
  font-family: var(--font-sans);
}
</style>
