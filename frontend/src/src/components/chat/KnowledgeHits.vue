<script setup lang="ts">
/**
 * 知识库检索来源卡（RAG 收敛）：knowledge_search 的结构化命中列表渲染成
 * 「[n] 文档 · 标题 · 段位置 + 相关度 + 内容摘要」的可展开卡片，替代原始文本墙。
 * 点击卡片展开/收起该条命中的完整内容——出处一目了然，不必读整段工具结果。
 */
import { computed, ref } from 'vue'
import { BookOpen, ChevronDown } from '@/components/common/icons'
import { t } from '@/i18n'

export interface KnowledgeHit {
  docID?: string
  docName?: string
  chunkID?: string
  chunkIdx?: number
  content?: string
  score?: number
  source?: string
  meta?: { title?: string } | null
}

const props = defineProps<{ hits: KnowledgeHit[] }>()

const openIdx = ref<number | null>(null)
function toggle(i: number): void {
  openIdx.value = openIdx.value === i ? null : i
}

/** 相关度百分比（score 语义由 rag 层保证单调；无值不显示）。 */
function pct(score: number | undefined): string {
  if (score == null || Number.isNaN(score)) return ''
  const v = Math.round(score * 100)
  return v > 0 ? `${Math.min(100, v)}%` : ''
}

const items = computed(() =>
  props.hits.map((h, i) => ({
    i,
    doc: h.docName || h.docID || t('kdoc.hitUnknownDoc'),
    title: h.meta?.title ?? '',
    chunk: typeof h.chunkIdx === 'number' ? t('kdoc.hitChunkLabel', h.chunkIdx + 1) : '',
    score: pct(h.score),
    content: h.content ?? ''
  }))
)
</script>

<template>
  <div class="wb-khits">
    <div class="wb-khits-head">
      <BookOpen class="h-3 w-3 text-wb-lavender" />
      <span>{{ t('kdoc.hitSourcesTitle', hits.length) }}</span>
    </div>
    <button
      v-for="it in items"
      :key="it.i"
      type="button"
      class="wb-khit"
      :class="{ open: openIdx === it.i }"
      @click="toggle(it.i)"
    >
      <span class="wb-khit-top">
        <span class="wb-khit-idx">[{{ it.i + 1 }}]</span>
        <span class="wb-khit-doc" :title="it.doc">{{ it.doc }}</span>
        <span v-if="it.title" class="wb-khit-title">{{ it.title }}</span>
        <span v-if="it.chunk" class="wb-khit-chunk">{{ it.chunk }}</span>
        <span v-if="it.score" class="wb-khit-score">{{ it.score }}</span>
        <ChevronDown class="wb-khit-chev" :class="{ rot: openIdx === it.i }" />
      </span>
      <span class="wb-khit-snippet" :class="{ clamp: openIdx !== it.i }">{{ it.content }}</span>
    </button>
  </div>
</template>

<style scoped>
.wb-khits {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.wb-khits-head {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 10.5px;
  color: var(--wb-muted);
}
.wb-khit {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 6px 8px;
  border: 1px solid var(--wb-border);
  border-radius: 8px;
  background: color-mix(in srgb, var(--wb-lavender) 4%, transparent);
  text-align: left;
  cursor: pointer;
  transition: border-color 0.15s ease, background 0.15s ease;
}
.wb-khit:hover,
.wb-khit.open {
  border-color: color-mix(in srgb, var(--wb-lavender) 45%, var(--wb-border));
  background: color-mix(in srgb, var(--wb-lavender) 8%, transparent);
}
.wb-khit-top {
  display: flex;
  align-items: baseline;
  gap: 6px;
  min-width: 0;
}
.wb-khit-idx {
  flex: none;
  font-family: var(--font-mono);
  font-size: 10.5px;
  color: var(--wb-lavender);
}
.wb-khit-doc {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 11.5px;
  font-weight: 500;
  color: var(--wb-ink);
}
.wb-khit-title {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 10.5px;
  color: var(--wb-muted);
}
.wb-khit-chunk {
  flex: none;
  font-size: 10px;
  color: var(--wb-muted);
}
.wb-khit-score {
  flex: none;
  border-radius: 4px;
  background: color-mix(in srgb, var(--wb-mint) 12%, transparent);
  padding: 0 4px;
  font-family: var(--font-mono);
  font-size: 10px;
  color: var(--wb-mint);
}
.wb-khit-chev {
  flex: none;
  width: 11px;
  height: 11px;
  margin-left: auto;
  color: var(--wb-muted);
  transition: transform 0.18s ease;
}
.wb-khit-chev.rot {
  transform: rotate(180deg);
}
.wb-khit-snippet {
  font-size: 11px;
  line-height: 1.65;
  color: var(--wb-muted);
  white-space: pre-wrap;
  word-break: break-word;
}
.wb-khit-snippet.clamp {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
