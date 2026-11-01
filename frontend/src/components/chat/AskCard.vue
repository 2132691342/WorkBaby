<script setup lang="ts">
// 内核提问卡：内核在等用户回答。协议只有 confirm / select / input / editor 四个基元，
// 多选与自定义答案是扩展用哨兵行拼的——这里只把四个基元渲染到位，不自造复选框。
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useChatStore } from '../../stores/chat'
import type { PendingAsk } from '../../types/chatEvents'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{ ask: PendingAsk }>()
const { t } = useI18n()
const chat = useChatStore()
const text = ref('')
const busy = ref(false)
const optIndex = ref(0)
const custom = ref(false)

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

// 扩展常常把一张 ASCII 对照图塞进 message：必须用等宽字体，否则框线全错位。
const diagram = computed(() => /[\u2500-\u257f]/.test(props.ask.message || ''))

interface Opt {
  raw: string
  no: number | null
  title: string
  body: string
  custom: boolean
}

// 「Type something.」是扩展写在选项里的自定义入口，不是真的选项：单独接成输入行。
const CUSTOM_ROW = /^(type something\.?|其他|自定义.*|other.*)$/i

const options = computed<Opt[]>(() =>
  (props.ask.options ?? []).map((raw, i) => {
    const lines = String(raw ?? '').split('\n')
    const first = (lines[0] ?? '').trim()
    const m = first.match(/^(\d+)\s*[.、)]\s*(.*)$/)
    const title = m ? m[2].trim() : first
    return {
      raw,
      no: m ? Number(m[1]) : i + 1,
      title,
      body: lines.slice(1).join('\n').trim(),
      custom: CUSTOM_ROW.test(title),
    }
  }),
)

const realOptions = computed(() => options.value.filter((o) => !o.custom))
const hasCustomRow = computed(() => options.value.some((o) => o.custom))

// editor / input 带预填：用户多半只要改两行，不该让他从空白开始重写
watch(
  () => props.ask.id,
  () => {
    text.value = props.ask.prefill || ''
    optIndex.value = 0
    custom.value = false
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
  const opt = realOptions.value[index]
  if (!opt) return
  optIndex.value = index
  void submit(opt.raw, true)
}

async function openCustom() {
  custom.value = true
  await nextTick()
  ;(document.querySelector(`[data-ask="${props.ask.id}"] .ask-custom input`) as HTMLInputElement | null)?.focus()
}

function onKey(e: KeyboardEvent) {
  if (custom.value) return
  const n = realOptions.value.length
  if (!n) return
  if (e.key === 'Escape') {
    e.preventDefault()
    void submit('', false, true)
    return
  }
  if (/^[1-9]$/.test(e.key)) {
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
        <b class="ask-title" :class="{ 'is-multiline': titleMultiline }">
          {{ ask.title || t('chat.engineStatus') }}
        </b>
        <div v-if="ask.message" class="ask-msg" :class="{ 'is-diagram': diagram }">{{ ask.message }}</div>
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

    <!-- select：清单即协议。选项是扩展拼好的整行文字，原样换行显示，
         截断成「…」会让用户看不见自己到底在选什么。↑↓ / 数字 / 回车都可用。 -->
    <div v-else-if="isSelect" class="ask-select">
      <div class="ask-opts" role="listbox" :aria-label="ask.title" tabindex="0" @keydown="onKey">
        <button
          v-for="(opt, i) in realOptions"
          :key="`${i}-${opt.raw}`"
          class="ask-opt"
          :class="{ 'is-hi': i === optIndex }"
          role="option"
          :aria-selected="i === optIndex"
          type="button"
          :disabled="busy"
          @mouseenter="optIndex = i"
          @click="pick(i)"
        >
          <span class="ask-no">{{ opt.no ?? i + 1 }}</span>
          <span class="ask-opt-tx">
            <span class="ask-opt-t">{{ opt.title }}</span>
            <span v-if="opt.body" class="ask-opt-b">{{ opt.body }}</span>
          </span>
        </button>
        <button v-if="hasCustomRow" class="ask-opt is-custom" type="button" :disabled="busy" @click="openCustom">
          <span class="ask-no"><AppIcon name="pencil" size="ic-2xs" /></span>
          <span class="ask-opt-tx">
            <span class="ask-opt-t">{{ t('chat.askCustom') }}</span>
            <span class="ask-opt-b">{{ t('chat.askCustomHint') }}</span>
          </span>
        </button>
      </div>

      <el-input
        v-if="custom"
        v-model="text"
        class="ask-custom"
        size="small"
        :placeholder="ask.placeholder || t('chat.askInputPlaceholder')"
        :disabled="busy"
        @keydown.enter.exact.prevent="text.trim() && submit(text.trim(), true)"
      />

      <div class="ask-acts">
        <el-button
          v-if="custom"
          type="primary"
          size="small"
          :loading="busy"
          :disabled="busy || !text.trim()"
          @click="submit(text.trim(), true)"
        >
          {{ t('chat.askSubmit') }}
        </el-button>
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
  flex: 1;
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
  line-height: var(--wb-lh-base);
}
/* 扩展把编号清单折进标题时按原样保留换行：折成一整行就读不出编号了 */
.ask-title.is-multiline {
  white-space: pre-wrap;
  font-weight: 600;
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
/* ASCII 对照图：框线必须在等宽字体下才对得齐，否则看起来像一堆乱码 */
.ask-msg.is-diagram {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  line-height: 1.45;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-hover);
  overflow: auto;
}
.ask-acts {
  display: flex;
  gap: var(--wb-sp-2);
  align-items: center;
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
  max-height: calc(280px * var(--wb-vw-scale));
  overflow: auto;
  border-radius: var(--wb-radius-sm);
  outline: none;
  overscroll-behavior: contain;
}
/* 选项行：编号 + 标题 + 说明。选中态沿用全站的「淡底 + 左侧竖条 + 主色」。 */
.ask-opt {
  position: relative;
  display: flex;
  align-items: flex-start;
  gap: var(--wb-sp-3);
  width: 100%;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-surface);
  color: var(--wb-ink-2);
  text-align: left;
  font-size: var(--wb-fs-sm);
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    border-color var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.ask-opt::before {
  content: '';
  position: absolute;
  left: 0;
  top: 6px;
  bottom: 6px;
  width: 2px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-primary);
  opacity: 0;
  transition: opacity var(--wb-dur-fast) var(--wb-ease);
}
.ask-opt:hover:not(:disabled),
.ask-opt.is-hi:not(:disabled) {
  background: var(--wb-accent-tint);
  border-color: var(--wb-accent-border);
  color: var(--wb-primary);
}
.ask-opt.is-hi:not(:disabled)::before {
  opacity: 1;
}
.ask-opt:active:not(:disabled) {
  transform: scale(var(--wb-row-scale-press));
}
.ask-opt:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.ask-no {
  flex: none;
  display: grid;
  place-items: center;
  min-width: calc(20px * var(--wb-vw-scale));
  height: calc(20px * var(--wb-vw-scale));
  padding: 0 5px;
  border-radius: var(--wb-radius-xs);
  background: var(--wb-elev-hover);
  color: var(--wb-muted);
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease);
}
.ask-opt:hover:not(:disabled) .ask-no,
.ask-opt.is-hi:not(:disabled) .ask-no {
  background: var(--wb-primary);
  color: #fff;
}
.ask-opt-tx {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.ask-opt-t {
  font-weight: 600;
  overflow-wrap: anywhere;
  line-height: var(--wb-lh-base);
}
.ask-opt-b {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  line-height: var(--wb-lh-base);
}
.ask-opt.is-custom {
  border-style: dashed;
  background: transparent;
}
</style>
