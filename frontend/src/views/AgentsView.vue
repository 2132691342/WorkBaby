<script setup lang="ts">
// 子代理页：左栏活跃清单 + 右侧过程详情。
// 子代理只由 task 工具派生，本页不提供手动派发入口——那是模型的决定，不是用户的。
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAgentsStore } from '../stores/agents'
import AgentDetail from '../components/agents/AgentDetail.vue'
import AgentList from '../components/agents/AgentList.vue'
import AppIcon from '../components/common/AppIcon.vue'
import PageHeader from '../components/layout/PageHeader.vue'

const { t } = useI18n()
const agents = useAgentsStore()

const loading = computed(() => !agents.loaded)

/** 手动刷新：断线或刚进页时对一次活跃表。 */
async function refresh() {
  await agents.refresh()
}

onMounted(() => {
  void agents.refresh()
})
</script>

<template>
  <div class="page">
    <PageHeader :title="t('agents.title')" :subtitle="t('agents.desc')">
      <template #actions>
        <span v-if="agents.liveCount" class="live-badge">
          <span class="live-dot" />
          {{ t('agents.liveCount', { n: agents.liveCount }) }}
        </span>
        <el-button size="small" :loading="loading" @click="refresh">
          <AppIcon name="refresh" size="ic-xs" />
          <span class="ml-1">{{ t('common.refresh') }}</span>
        </el-button>
      </template>
    </PageHeader>

    <div class="agent-wrap">
      <aside class="agent-side">
        <AgentList
          :rows="agents.list"
          :selected="agents.current?.agent_session_id || ''"
          :loading="loading"
          @select="agents.select"
        />
      </aside>
      <section class="agent-main">
        <Transition name="wb-fade" mode="out-in">
          <AgentDetail :key="agents.current?.agent_session_id || 'none'" :agent="agents.current" />
        </Transition>
      </section>
    </div>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
  min-height: 0;
}
.live-badge {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-2);
  height: var(--wb-ctl-h-sm);
  padding: 0 var(--wb-sp-3);
  border-radius: var(--wb-radius-full);
  border: 1px solid color-mix(in srgb, var(--wb-live) 40%, var(--wb-border));
  background: var(--wb-live-soft);
  color: var(--wb-live);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
}
.live-dot {
  width: 6px;
  height: 6px;
  border-radius: var(--wb-radius-full);
  background: currentColor;
  animation: live-breathe 1.2s var(--wb-ease) infinite;
}
@keyframes live-breathe {
  50% {
    opacity: 0.3;
  }
}
.agent-wrap {
  display: flex;
  flex: 1;
  min-height: 0;
  gap: var(--wb-sp-3);
  padding: 0 var(--wb-sp-5) var(--wb-sp-5);
}
.agent-side {
  width: calc(320px * var(--wb-vw-scale));
  flex: none;
  min-height: 0;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius-xl);
  background: var(--wb-elev-raised);
  box-shadow: var(--wb-card-shadow);
  overflow: hidden;
}
.agent-main {
  flex: 1;
  min-width: 0;
  min-height: 0;
  overflow-y: auto;
  border: 1px solid var(--wb-border);
  border-radius: var(--wb-radius-xl);
  background: var(--wb-elev-raised);
  box-shadow: var(--wb-card-shadow);
}
@media (max-width: 1100px) {
  .agent-side {
    width: calc(260px * var(--wb-vw-scale));
  }
}
@media (prefers-reduced-motion: reduce) {
  .live-dot {
    animation: none;
  }
}
</style>
