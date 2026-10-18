<script setup lang="ts">
// 引擎提问卡：PI 里的扩展（含内置提问工具）在等用户回答。
// 四种方法各有对应交互：确认 / 单选列表 / 填空 / 多行编辑；拒绝与跳过永远可用。
import { computed, nextTick, onMounted, ref } from 'vue'
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
  document.querySelector<HTMLElement>(`[data-ask="${props.ask.id}"] .ask-opt.is-hi`)?.scrollIntoView({ block: 'nearest' })
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
        <b class="ask-title">{{ ask.title || t('chat.engineStatus') }}</b>
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

    <!-- select：选项列表，↑↓ 移动、回车选中 -->
    <div v-else-if="isSelect" class="ask-select">
      <div class="ask-opts" role="listbox" :aria-label="ask.title" tabindex="0" @keydown="onKey">
        <button
          v-for="(opt, i) in ask.options"
          :key="opt"
          class="ask-opt"
          :class="{ 'is-hi': i === optIndex }"
          role="option"
          :aria-selected="i === optIndex"
          type="button"
          :disabled="busy"
          @mouseenter="optIndex = i"
          @click="pick(i)"
        >
          <AppIcon name="chevron-right" size="ic-2xs" />
          <span class="ask-opt-t">{{ opt }}</span>
        </button>
      </div>
      <div class="ask-acts">
        <el-button size="small" text :disabled="busy" @click="submit('', false, true)">
          {{ t('common.cancel') }}
        </el-button>
      </div>
    </div>

    <!-- editor / input：填空 -->
    <div v-else class="ask-input">
      <el-input
        v-model="text"
        :type="isEditor ? 'textarea' : 'text'"
        :rows="isEditor ? 3 : undefined"
        :placeholder="isEditor ? t('chat.askInputPlaceholder') : t('chat.askInputPlaceholder')"
        :disabled="busy"
        @keydown.enter.ctrl.prevent="submit(text, true)"
        @keydown.enter.exact.prevent="!isEditor && submit(text, true)"
      />
      <div class="ask-acts">
        <el-button
          type="primary"
          size="small"
          :loading="busy"
          :disabled="busy || !text.trim()"
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
  background: var(--wb-surface);
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
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
  flex: none;
}
.ask-tx {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.ask-title {
  font-size: var(--wb-fs-sm);
  font-weight: 700;
  color: var(--wb-ink);
}
.ask-msg {
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-2);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  max-height: calc(180px * var(--wb-vw-scale));
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
  max-height: calc(240px * var(--wb-vw-scale));
  overflow: auto;
  border-radius: var(--wb-radius-sm);
}
.ask-opt {
  display: flex;
  align-items: center;
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
  background: var(--wb-primary-soft);
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
.ask-opt-t {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
