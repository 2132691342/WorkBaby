<script setup lang="ts">
// 引擎提问卡：PI 里的扩展（含内置提问工具）在等用户回答。
// PI 的 RPC 只有四个提问基元：confirm / select / input / editor。
// 多选题与「输入自己的想法」都不是协议特性——扩展会用 select 的哨兵行 + input 依次问，
// 或者直接把编号列表塞进 input 的标题里。所以这里只做一件事：把四个基元渲染到位，
// 不自造复选框（PI 认不出我们编出来的答案）。
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useChatStore, type PendingAsk } from '../../stores/chat'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{ ask: PendingAsk }>()
const { t } = useI18n()
const chat = useChatStore()
const text = ref('')
const busy = ref(false)
const optIndex = ref(0)

const isEditor = computed(() => props.ask.method === 'editor')
const isSelect = computed(() => props.ask.method === 'select')
const isConfirm = computed(() => props.ask.method === 'confirm')
// 标题里带换行的基本都是扩展把选项清单折进了标题（多选题回退路径）
const titleMultiline = computed(() => props.ask.title.includes('\n'))

const methodKey = computed(() => {
  if (isConfirm.value) return 'askKindConfirm'
  if (isSelect.value) return 'askKindSelect'
  return isEditor.value ? 'askKindEditor' : 'askKindInput'
})

// editor 带预填：用户多半只要改两行，不该让他从空白开始重写
watch(
  () => props.ask.id,
  () => {
    text.value = props.ask.prefill || ''
  },
  { immediate: true },
)

const timeoutHint = computed(() => {
  const ms = props.ask.timeoutMs
  if (!ms || ms <= 0) return ''
  return t('chat.askTimeout', { n: Math.max(1, Math.round(ms / 1000)) })
})

async function submit(value: string, confirmed: boolean, cancelled = false) {
  if (busy.value) return
  busy.value = true
  try {
    await chat.answer(props.ask, value, confirmed, cancelled)
  } finally {
    busy.value = false
  }
}

function pick(index: number) {
  const opt = props.ask.options[index]
  if (opt === undefined) return
  optIndex.value = index
  void submit(opt, true)
}

function onKey(e: KeyboardEvent) {
  const n = props.ask.options.length
  if (!n) return
  if (e.key === 'Escape') {
    e.preventDefault()
    void submit('', false, true)
    return
  }
  if (/^[1-9]$/.test(e.key)) {
    // 选项自带「1. …」编号：数字直达与列表看起来一致
    const i = Number(e.key) - 1
    if (i < n) {
      e.preventDefault()
      pick(i)
    }
    return
  }
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    optIndex.value = (optIndex.value + 1) % n
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    optIndex.value = (optIndex.value - 1 + n) % n
  } else if (e.key === 'Enter' && !e.isComposing) {
    e.preventDefault()
    pick(Math.min(optIndex.value, n - 1))
  }
}

async function focusCurrent() {
  await nextTick()
  document
    .querySelector<HTMLElement>(`[data-ask="${props.ask.id}"] .ask-opt.is-hi`)
    ?.scrollIntoView({ block: 'nearest' })
}

onMounted(() => {
  if (isSelect.value) void focusCurrent()
})
</script>

<template>
  <div class="ask" :data-ask="ask.id">
    <div class="ask-head">
      <span class="ask-tile"><AppIcon name="help" size="ic-sm" /></span>
      <div class="ask-tx">
        <div class="ask-meta">
          <span class="ask-kind">{{ t(`chat.${methodKey}`) }}</span>
          <span v-if="timeoutHint" class="ask-timeout">{{ timeoutHint }}</span>
        </div>
        <b class="ask-title" :class="{ 'is-multiline': titleMultiline }">{{ ask.title || t('chat.engineStatus') }}</b>
        <p v-if="ask.message" class="ask-msg">{{ ask.message }}</p>
      </div>
    </div>

    <!-- confirm：两个明确的选择，拒绝永远安全 -->
    <div v-if="isConfirm" class="ask-acts">
      <el-button type="primary" size="small" :loading="busy" :disabled="busy" @click="submit('', true)">
        {{ t('chat.askConfirm') }}
      </el-button>
      <el-button size="small" :disabled="busy" @click="submit('', false)">
        {{ t('chat.askReject') }}
      </el-button>
    </div>

    <!-- select：清单即协议。选项是扩展拼好的整行文字（可能带编号与说明），原样换行显示，
         截断成「…」会让用户看不见自己到底在选什么。↑↓ / 数字 / 回车都可用。 -->
    <div v-else-if="isSelect" class="ask-select">
      <div class="ask-opts" role="listbox" :aria-label="ask.title" tabindex="0" @keydown="onKey">
        <button
          v-for="(opt, i) in ask.options"
          :key="`${i}-${opt}`"
          class="ask-opt"
          :class="{ 'is-hi': i === optIndex }"
          role="option"
          :aria-selected="i === optIndex"
          type="button"
          :disabled="busy"
          @mouseenter="optIndex = i"
          @click="pick(i)"
        >
          <AppIcon name="chevron-right" size="ic-2xs" class="ask-opt-ic" />
          <span class="ask-opt-t">{{ opt }}</span>
        </button>
      </div>
      <div class="ask-acts">
        <el-button size="small" text :disabled="busy" @click="submit('', false, true)">
          {{ t('common.cancel') }}
        </el-button>
      </div>
    </div>

    <!-- input / editor：placeholder 是扩展给的格式说明，必须用上 -->
    <div v-else class="ask-input">
      <el-input
        v-model="text"
        :type="isEditor ? 'textarea' : 'text'"
        :rows="isEditor ? 4 : undefined"
        :placeholder="ask.placeholder || t('chat.askInputPlaceholder')"
        :disabled="busy"
        @keydown.enter.ctrl.prevent="submit(text, true)"
        @keydown.enter.exact.prevent="!isEditor && submit(text, true)"
      />
      <div class="ask-acts">
        <el-button
          type="primary"
          size="small"
          :loading="busy"
          :disabled="busy || (!isEditor && !text.trim())"
          @click="submit(text, true)"
        >
          {{ t('chat.askSubmit') }}
        </el-button>
        <el-button size="small" :disabled="busy" @click="submit('', false, true)">
          {{ t('common.cancel') }}
        </el-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.ask {
  border: 1px solid var(--wb-border);
  border-left: 2px solid var(--wb-primary);
  border-radius: var(--wb-radius-lg);
  background: var(--wb-elev-raised);
  padding: var(--wb-sp-4);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
  box-shadow: var(--wb-card-shadow);
  animation: wb-rise var(--wb-dur) var(--wb-ease);
}
.ask-head {
  display: flex;
  gap: var(--wb-sp-3);
  align-items: flex-start;
}
.ask-tile {
  width: var(--wb-tile-sm);
  height: var(--wb-tile-sm);
  border-radius: var(--wb-radius-sm);
  display: grid;
  place-items: center;
  background: var(--wb-accent-tint);
  color: var(--wb-primary);
  flex: none;
}
.ask-tx {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.ask-meta {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
/* 提问类型：让用户知道自己在看哪一种交互（确认 / 单选 / 填空 / 多行） */
.ask-kind {
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  letter-spacing: 0.06em;
  color: var(--wb-primary);
  background: var(--wb-accent-tint);
  border-radius: var(--wb-radius-xs);
  padding: 1px var(--wb-sp-1);
}
.ask-timeout {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-warning);
  font-weight: 600;
}
.ask-title {
  font-size: var(--wb-fs-sm);
  font-weight: 700;
  color: var(--wb-ink);
  overflow-wrap: anywhere;
}
/* 扩展把编号清单折进标题时按原样保留换行：折成一整行就读不出编号了 */
.ask-title.is-multiline {
  white-space: pre-wrap;
  font-weight: 600;
  line-height: var(--wb-lh-base);
}
.ask-msg {
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-2);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  max-height: calc(220px * var(--wb-vw-scale));
  overflow: auto;
  line-height: var(--wb-lh-base);
}
.ask-acts {
  display: flex;
  gap: var(--wb-sp-2);
}
.ask-select,
.ask-input {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.ask-opts {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: calc(260px * var(--wb-vw-scale));
  overflow: auto;
  border-radius: var(--wb-radius-sm);
  outline: none;
}
.ask-opt {
  display: flex;
  align-items: flex-start;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  border: 1px solid transparent;
  color: var(--wb-ink-2);
  text-align: left;
  font-size: var(--wb-fs-sm);
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    border-color var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.ask-opt:hover:not(:disabled),
.ask-opt.is-hi:not(:disabled) {
  background: var(--wb-accent-tint);
  color: var(--wb-primary);
  border-color: var(--wb-primary-line);
}
.ask-opt:active:not(:disabled) {
  transform: scale(0.99);
}
.ask-opt:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.ask-opt-ic {
  margin-top: calc(2px * var(--wb-vw-scale));
  flex: none;
}
.ask-opt-t {
  flex: 1;
  min-width: 0;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  line-height: var(--wb-lh-base);
}
</style>
