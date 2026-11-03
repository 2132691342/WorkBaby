<script setup lang="ts">
// 对话页页头：回答「在跟谁说话」。模型 / 工作区 / 水位这些参数在输入区底栏，
// 页头只留会话名、会话级动作（压缩上下文）与运行灯。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useChatStore } from '../../stores/chat'
import { useSessionStore } from '../../stores/session'
import { useToastStore } from '../../stores/toast'
import AppIcon from '../common/AppIcon.vue'
import SessionTree from './SessionTree.vue'

const { t } = useI18n()
const chat = useChatStore()
const session = useSessionStore()
const toast = useToastStore()

const busy = ref(false)
/** 会话树开关：只在有会话且有分叉时才渲染入口——没分叉时这颗按钮无从点击。 */
const treeOpen = ref(false)
const hasTree = computed(
  () => !!session.currentId && (session.altLeafIds.length > 0 || session.forkEntryIds.length > 0),
)

const title = computed(() => session.current?.session_title || t('chat.newSession'))
const modelLine = computed(() => {
  const s = session.current
  if (!s?.session_model) return ''
  return [s.session_provider, s.session_model].filter(Boolean).join(' · ')
})

/** 手动压缩：空闲会话才允许；压缩中按钮进 loading 态，不给重复点。 */
async function compactNow() {
  const id = session.currentId
  if (!id || busy.value || chat.running) return
  busy.value = true
  try {
    const ok = await chat.compact(id)
    if (ok) toast.ok(t('chat.compactDone'))
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <header class="chat-head">
    <div class="ch-left">
      <h1 class="ch-title" :title="title">{{ title }}</h1>
      <span v-if="modelLine" class="ch-sub" :title="modelLine">{{ modelLine }}</span>
    </div>
    <div class="ch-right">
      <el-tooltip v-if="hasTree" :content="t('tree.title')" placement="bottom" :show-after="300">
        <el-button
          size="small"
          class="ch-act"
          :class="{ 'is-on': treeOpen }"
          :disabled="!session.currentId"
          @click="treeOpen = !treeOpen"
        >
          <AppIcon name="git-branch" size="ic-2xs" />
          <span class="ml-1">{{ t('tree.button') }}</span>
        </el-button>
      </el-tooltip>
      <el-tooltip :content="t('chat.compactHint')" placement="bottom" :show-after="300">
        <el-button
          size="small"
          class="ch-act"
          :loading="busy || chat.compacting"
          :disabled="busy || chat.compacting || chat.running || !session.currentId"
          @click="compactNow"
        >
          <AppIcon v-if="!busy && !chat.compacting" name="refresh" size="ic-2xs" />
          <span class="ml-1">{{ t('chat.compact') }}</span>
        </el-button>
      </el-tooltip>
    </div>
    <!-- 抽屉挂在页头下方：绝对定位，不挤压消息流高度 -->
    <div v-if="treeOpen" class="ch-tree">
      <SessionTree :open="treeOpen" @close="treeOpen = false" />
    </div>
  </header>
</template>

<style scoped>
.chat-head {
  position: relative;
  flex: none;
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  height: var(--wb-h-pagehead);
  padding: 0 var(--wb-sp-5);
  border-bottom: 1px solid var(--wb-border);
  background: var(--wb-elev-workspace);
  min-width: 0;
}
/* 会话树抽屉：浮在消息流上方，不改变布局高度 */
.ch-tree {
  position: absolute;
  top: calc(var(--wb-h-pagehead) - 4px);
  right: var(--wb-sp-5);
  z-index: 30;
  width: calc(340px * var(--wb-vw-scale));
  max-height: calc(60vh);
  overflow: auto;
}
.ch-left {
  display: flex;
  align-items: baseline;
  gap: var(--wb-sp-2);
  min-width: 0;
  flex: 1;
}
.ch-title {
  margin: 0;
  font-size: var(--wb-fs-lg);
  font-weight: 700;
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
.ch-sub {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: calc(320px * var(--wb-vw-scale));
  flex: none;
}
.ch-right {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  flex: none;
}
/* 树抽屉开着时按钮保持选中底：否则看不出抽屉是从这里开的 */
.ch-act.is-on {
  color: var(--wb-primary);
  background: var(--wb-accent-tint);
  border-color: var(--wb-brand-2);
}
</style>
