<script setup lang="ts">
// 概览页：一屏回答「现在什么状态、能接着做什么」。
// 三段编排：能力总览（功能/技能/工具/MCP 计数）→ 最近会话 → 快捷入口。
// 数字全部来自各自清单接口，不另存一份账本。
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import * as api from '../api'
import type { FeatureVO, SessionVO, SkillVO, ToolVO } from '../types/api'
import AppIcon from '../components/common/AppIcon.vue'
import PageHeader from '../components/layout/PageHeader.vue'

const { t } = useI18n()
const router = useRouter()

const features = ref<FeatureVO[]>([])
const skills = ref<SkillVO[]>([])
const tools = ref<ToolVO[]>([])
const sessions = ref<SessionVO[]>([])
const mcpCount = ref(0)
const loading = ref(true)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [warm, sess, mcp] = await Promise.all([
      api.warmup(),
      api.sessions.list(),
      api.mcp.list().catch(() => null),
    ])
    features.value = warm.features ?? []
    skills.value = warm.skills ?? []
    tools.value = warm.tools ?? []
    sessions.value = sess ?? []
    mcpCount.value = mcp?.resp_servers?.length ?? 0
  } catch (e) {
    error.value = (e as Error)?.message || t('errors.generic')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void load()
})

const enabledFeatures = computed(() => features.value.filter((f) => f.feature_enabled))
const enabledSkills = computed(() => skills.value.filter((s) => s.skill_enabled))
const enabledTools = computed(() => tools.value.filter((x) => x.tool_enabled))
const recentSessions = computed(() =>
  [...sessions.value]
    .sort((a, b) => (b.session_updated_at || 0) - (a.session_updated_at || 0))
    .slice(0, 6),
)

// 四张能力卡：数字是「开了几个」，分母是总数；没启用的能力仍是可见的一等公民
const capabilityCards = computed(() => [
  {
    key: 'features',
    icon: 'zap',
    label: t('overview.capFeatures'),
    enabled: enabledFeatures.value.length,
    total: features.value.length,
    path: '/features',
  },
  {
    key: 'skills',
    icon: 'sparkles',
    label: t('overview.capSkills'),
    enabled: enabledSkills.value.length,
    total: skills.value.length,
    path: '/skills',
  },
  {
    key: 'tools',
    icon: 'terminal',
    label: t('overview.capTools'),
    enabled: enabledTools.value.length,
    total: tools.value.length,
    path: '/settings/tools',
  },
  {
    key: 'mcp',
    icon: 'globe',
    label: t('overview.capMcp'),
    enabled: mcpCount.value,
    total: mcpCount.value,
    path: '/settings/mcp',
  },
])

function openSession(id: string) {
  void router.push({ path: '/', query: { session: id } })
}

function fmtTime(ts: number): string {
  if (!ts) return ''
  const d = new Date(ts)
  const diff = Date.now() - ts
  const min = Math.floor(diff / 60_000)
  if (min < 1) return t('overview.justNow')
  if (min < 60) return t('overview.minutesAgo', { n: min })
  const hour = Math.floor(min / 60)
  if (hour < 24) return t('overview.hoursAgo', { n: hour })
  const day = Math.floor(hour / 24)
  if (day < 30) return t('overview.daysAgo', { n: day })
  return d.toLocaleDateString()
}
</script>

<template>
  <div class="view">
    <PageHeader :title="t('overview.title')" :subtitle="t('overview.subtitle')">
      <template #actions>
        <el-button size="small" :loading="loading" @click="load">
          <AppIcon v-if="!loading" name="refresh" size="ic-xs" />
          <span class="ml-1">{{ t('common.refresh') }}</span>
        </el-button>
        <el-button size="small" type="primary" @click="router.push('/')">
          {{ t('chat.newSession') }}
        </el-button>
      </template>
    </PageHeader>

    <div class="body">
      <div v-if="error" class="wb-card empty-box">
        <span class="wb-tile wb-tile--lg wb-tile--neutral"><AppIcon name="alert" size="ic-xl" /></span>
        <b>{{ t('errors.generic') }}</b>
        <p>{{ error }}</p>
        <el-button size="small" @click="load">{{ t('common.retry') }}</el-button>
      </div>

      <template v-else>
        <!-- 能力总览：四张卡，点进去就是对应的管理页 -->
        <div class="grid grid-cols-2 xl:grid-cols-4 gap-4">
          <button
            v-for="c in capabilityCards"
            :key="c.key"
            type="button"
            class="wb-card cap-card"
            @click="router.push(c.path)"
          >
            <span class="wb-tile wb-tile--lg"><AppIcon :name="c.icon" size="ic-lg" /></span>
            <span class="cap-count">
              <b>{{ c.enabled }}</b>
              <span class="cap-total">/ {{ c.total }}</span>
            </span>
            <span class="cap-label">{{ c.label }}</span>
          </button>
        </div>

        <!-- 最近会话：继续干活的入口摆在同一屏 -->
        <section class="wb-card card-block mt-4">
          <header class="card-head">
            <div>
              <h3 class="card-title">{{ t('overview.recent') }}</h3>
              <p class="card-sub">{{ t('overview.recentHint') }}</p>
            </div>
            <el-button size="small" text @click="router.push('/sessions')">
              {{ t('overview.allSessions') }}
            </el-button>
          </header>

          <div v-if="loading && !sessions.length" class="skeleton-rows">
            <el-skeleton v-for="i in 3" :key="i" animated :rows="1" />
          </div>

          <div v-else-if="!recentSessions.length" class="card-empty">
            <p>{{ t('overview.noSessions') }}</p>
            <el-button size="small" type="primary" @click="router.push('/')">
              {{ t('chat.newSession') }}
            </el-button>
          </div>

          <ul v-else class="session-list">
            <li v-for="s in recentSessions" :key="s.session_id">
              <button type="button" class="session-row" @click="openSession(s.session_id)">
                <span class="session-name">
                  {{ s.session_title || s.session_id }}
                </span>
                <span class="session-meta">
                  {{ s.session_model || t('common.unknown') }}
                  <span class="dot">·</span>
                  {{ fmtTime(s.session_updated_at) }}
                </span>
                <AppIcon name="chevron-right" size="ic-xs" class="session-go" />
              </button>
            </li>
          </ul>
        </section>
      </template>
    </div>
  </div>
</template>

<style scoped>
.view {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
  min-height: 0;
}
.body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--wb-sp-5);
}
.card-block {
  padding: var(--wb-sp-5);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}
.card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--wb-sp-3);
}
.card-title {
  font-size: var(--wb-fs-md);
  font-weight: 700;
  color: var(--wb-ink);
}
.card-sub {
  margin-top: 2px;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
/* 能力卡：整卡可点，hover 抬升 + 放大 */
.cap-card {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-4);
  text-align: left;
  cursor: pointer;
  transition:
    transform var(--wb-dur-fast) var(--wb-ease),
    box-shadow var(--wb-dur) var(--wb-ease),
    border-color var(--wb-dur) var(--wb-ease);
}
.cap-card:hover {
  transform: scale(1.03);
  box-shadow: var(--wb-shadow-2);
  border-color: var(--wb-border-strong);
}
.cap-card:active {
  transform: scale(0.98);
}
.cap-count {
  display: flex;
  align-items: baseline;
  gap: 4px;
  font-variant-numeric: tabular-nums;
}
.cap-count b {
  font-family: var(--font-display);
  font-size: var(--wb-fs-xl);
  font-weight: 700;
  color: var(--wb-ink);
}
.cap-total {
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.cap-label {
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.session-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.session-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  width: 100%;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  text-align: left;
  cursor: pointer;
  transition:
    background var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.session-row:hover {
  background: var(--wb-elev-hover);
  transform: translateX(2px);
}
.session-row:active {
  transform: translateX(2px) scale(0.99);
}
.session-name {
  flex: 1;
  min-width: 0;
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.session-meta {
  flex: none;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.dot {
  margin: 0 4px;
}
.session-go {
  flex: none;
  color: var(--wb-muted);
}
.skeleton-rows {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}
.card-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-6) 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.empty-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-10) var(--wb-sp-6);
  text-align: center;
}
.empty-box b {
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}
.empty-box p {
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
  max-width: calc(420px * var(--wb-vw-scale));
}
.ml-1 {
  margin-left: var(--wb-sp-1);
}
</style>
