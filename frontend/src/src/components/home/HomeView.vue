<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'
import { Check, MessageSquare, BookOpen, Server, LayoutDashboard, Rocket, X, Sparkles } from '@/components/common/icons'
import { useHomeStore } from '@/stores/home'
import { useChatStore } from '@/stores/chat'
import { t } from '@/i18n'
import { formatDateTime } from '@/utils/time'

/**
 * 概览页（照 prd/WorkBaby-UI-Prototype.html 01 屏）：
 * 欢迎区 + 四步快速上手（可点可关，localStorage 记忆）+ 快捷入口 + 最近会话 + 本地状态。
 * 数据全部来自真实 store：最近会话 useHomeStore，模型 / 工作区 / 权限 useChatStore。
 */
const home = useHomeStore()
const { sessions, loading } = storeToRefs(home)
const chat = useChatStore()
const router = useRouter()

function go(to: string): void {
  void router.push(to)
}

/** 新建会话并进入聊天。 */
async function newSession(): Promise<void> {
  const s = await chat.createSession(chat.selectedModelID)
  void router.push(`/chat/${s.id}`)
}

const quickCards = [
  { icon: MessageSquare, titleKey: 'home.quick.newChat', descKey: 'home.quick.newChatDesc', action: () => void newSession() },
  { icon: BookOpen, titleKey: 'home.quick.importDocs', descKey: 'home.quick.importDocsDesc', to: '/kdocs' },
  { icon: Server, titleKey: 'home.quick.setupMcp', descKey: 'home.quick.setupMcpDesc', to: '/mcp' },
  { icon: LayoutDashboard, titleKey: 'home.quick.viewUsage', descKey: 'home.quick.viewUsageDesc', to: '/dashboard' }
] as const

// ===== 快速上手引导（四步；「连接模型」以已接入模型自动判完成） =====
const GUIDE_KEY = 'workbaby.guide.onboarded'
const showGuide = ref(false)

const guideSteps = [
  { to: '/settings?tab=models', titleKey: 'home.guide.step1', descKey: 'home.guide.step1Desc', done: () => chat.models.length > 0 },
  { to: '/folders', titleKey: 'home.guide.step2', descKey: 'home.guide.step2Desc', done: () => false },
  { to: '/kdocs', titleKey: 'home.guide.step3', descKey: 'home.guide.step3Desc', done: () => false },
  { to: '/cron', titleKey: 'home.guide.step4', descKey: 'home.guide.step4Desc', done: () => false }
] as const

const guideDone = computed(() => guideSteps.map((s) => s.done()))

function dismissGuide(): void {
  showGuide.value = false
  try {
    localStorage.setItem(GUIDE_KEY, '1')
  } catch {
    // ignore
  }
}

// ===== 本地状态卡（真实值：工作区 / 默认模型 / 会话数） =====
const localState = computed(() => [
  {
    labelKey: 'home.state.workspace',
    value: sessions.value[0]?.workspace_path || t('chat.workspace.default'),
    mono: true
  },
  { labelKey: 'home.state.model', value: currentModelName(), mono: true },
  { labelKey: 'home.state.sessions', value: String(sessions.value.length) }
])

function currentModelName(): string {
  if (!chat.selectedModelID) return t('chat.auto')
  const m = chat.models.find((x) => x.id === chat.selectedModelID)
  return m ? m.alias || m.model : t('chat.auto')
}

function fmt(iso: string | number | null): string {
  return formatDateTime(iso)
}

onMounted(() => {
  void home.load()
  try {
    showGuide.value = localStorage.getItem(GUIDE_KEY) !== '1'
  } catch {
    showGuide.value = true
  }
})
</script>

<template>
  <div class="scroll wb-ui">
    <div class="wrap" style="max-width: 880px">
      <!-- 欢迎区 -->
      <header class="center" style="padding: 18px 0 4px">
        <div class="tile tile-xl flex items-center justify-center" style="margin: 0 auto 14px">
          <Sparkles class="ic" />
        </div>
        <h1 class="h-xl">{{ t('home.welcome') }}</h1>
        <p class="muted fs13 mt6">{{ t('home.subtitle') }}</p>
      </header>

      <!-- 快速上手 -->
      <div v-if="showGuide" class="card p-sm">
        <div class="flex-r mb10">
          <div class="mini-tile"><Rocket class="ic" /></div>
          <div>
            <h3>{{ t('home.guide.title') }}</h3>
            <p class="fs11 muted">{{ t('home.guide.subtitle') }}</p>
          </div>
          <span class="sp" />
          <button class="btn-icon" :title="t('home.guide.dismiss')" @click="dismissGuide">
            <X class="ic ic-sm" />
          </button>
        </div>
        <div class="steps">
          <div v-for="(s, i) in guideSteps" :key="s.to" class="step" :class="{ done: guideDone[i] }" @click="go(s.to)">
            <div class="num">
              <Check v-if="guideDone[i]" class="h-3 w-3" />
              <template v-else>{{ i + 1 }}</template>
            </div>
            <h5>{{ t(s.titleKey) }}</h5>
            <p>{{ t(s.descKey) }}</p>
          </div>
        </div>
      </div>

      <!-- 快捷入口 -->
      <div class="qgrid">
        <button v-for="c in quickCards" :key="c.titleKey" class="qcard" @click="'to' in c ? go(c.to) : c.action()">
          <div class="tile"><component :is="c.icon" class="ic" /></div>
          <h5>{{ t(c.titleKey) }}</h5>
          <p>{{ t(c.descKey) }}</p>
        </button>
      </div>

      <div class="grid2">
        <!-- 最近会话 -->
        <div class="card p-sm">
          <div class="flex-r mb8">
            <h3>{{ t('home.recentSessions') }}</h3>
            <span class="sp" />
            <button class="btn btn-sm" @click="go('/chat')">{{ t('home.openChat') }}</button>
          </div>
          <div class="rowlist">
            <div v-if="loading" class="empty">{{ t('ui.status.loading') }}</div>
            <div v-else-if="sessions.length === 0" class="empty">{{ t('chat.noSessions') }}</div>
            <template v-else>
              <button
                v-for="s in sessions.slice(0, 4)"
                :key="s.id"
                class="rli"
                style="width: 100%; text-align: left"
                @click="go(`/chat/${s.id}`)"
              >
              <div class="mini-tile" style="background: var(--wb-surface-hover); color: var(--wb-muted)">
                <MessageSquare class="ic" />
              </div>
              <div class="grow">
                <h5>{{ s.name || t('chat.unnamed') }}</h5>
                <p>{{ t('chat.message_count', s.message_count ?? 0) }} · {{ fmt(s.last_message_at) }}</p>
              </div>
              </button>
            </template>
          </div>
        </div>

        <!-- 本地状态 -->
        <div class="card p-sm">
          <div class="flex-r mb8"><h3>{{ t('home.localState') }}</h3></div>
          <dl class="kv">
            <template v-for="row in localState" :key="row.labelKey">
              <dt>{{ t(row.labelKey) }}</dt>
              <dd :class="row.mono ? 'mono' : ''" style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap">
                {{ row.value }}
              </dd>
            </template>
          </dl>
        </div>
      </div>
    </div>
  </div>
</template>
