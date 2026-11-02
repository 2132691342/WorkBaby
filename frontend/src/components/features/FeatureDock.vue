<script setup lang="ts">
// 功能活状态抽屉：待办 / 目标 / 计划 三个结构化面板，贴在对话主区右侧。
// 这三个是「这一轮在推进什么」的唯一可信来源，但都是长清单：铺进消息流会淹没正文。
//
// 有内容就出现：不能只在收到 widget 事件时才显示——重启应用、切回旧会话时没有事件，
// 盘上明明有清单却看不到 tab，等于功能不存在。widget 只当「有新变化」的信号，
// 出现与否由一次端点探测决定，正文一律拉结构化 VO。
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useChatStore } from '../../stores/chat'
import { useSessionStore } from '../../stores/session'
import TodoPanel from './TodoPanel.vue'
import GoalPanel from './GoalPanel.vue'
import PlanPanel from './PlanPanel.vue'
import AppIcon from '../common/AppIcon.vue'

const { t } = useI18n()
const chat = useChatStore()
const session = useSessionStore()

/** 三个面板的 tab；空串 = 收起 */
const tab = ref<'' | 'todo' | 'goal' | 'plan'>('')

const todoRef = ref<InstanceType<typeof TodoPanel> | null>(null)
const goalRef = ref<InstanceType<typeof GoalPanel> | null>(null)
const planRef = ref<InstanceType<typeof PlanPanel> | null>(null)

/** 盘上有什么：探测结果，与 widget 事件无关 */
const has = ref({ todo: false, goal: false, plan: false })

/** 探测三处状态：有内容就露出对应 tab。失败静默——面板本身会报错。 */
async function probe() {
  const [todo, goal, plan] = await Promise.allSettled([
    api.todo.list(),
    api.goal.get(),
    api.plan.get(),
  ])
  has.value = {
    todo: todo.status === 'fulfilled' && (todo.value?.resp_items?.length ?? 0) > 0,
    goal: goal.status === 'fulfilled' && !!goal.value?.resp_goal,
    plan: plan.status === 'fulfilled' && !!plan.value?.resp_plan,
  }
}

const anyPresent = computed(() => has.value.todo || has.value.goal || has.value.plan)
const open = computed(() => !!tab.value)

/** 当前 tab 失效（清单被清空）就自动收起，不留一个空面板 */
watch(has, (v) => {
  if (!tab.value) return
  if (!v[tab.value]) tab.value = ''
})

/** widget 变了就重拉对应面板，并顺带重探一次（清单可能刚被建/被清空） */
watch(
  () => chat.engineWidgets,
  () => {
    void probe()
    if (tab.value === 'todo') todoRef.value?.reload()
    if (tab.value === 'plan') planRef.value?.reload()
  },
  { deep: true },
)
watch(
  () => chat.engineStatus.goal,
  () => {
    void probe()
    if (tab.value === 'goal') goalRef.value?.reload()
  },
)

/** 一轮跑完：模型可能刚建了目标 / 计划，探测一次让 tab 及时出现 */
watch(
  () => chat.running,
  (running, was) => {
    if (was && !running) void probe()
  },
)

/** 切会话 / 切工作区后盘上状态会变：重新探测 */
watch(() => session.currentId, () => void probe())

function pick(next: 'todo' | 'goal' | 'plan') {
  tab.value = tab.value === next ? '' : next
}

/** 打开时立刻拉一次：探测只回答「有没有」，正文要现拉 */
watch(tab, (v) => {
  if (v === 'todo') todoRef.value?.reload()
  if (v === 'goal') goalRef.value?.reload()
  if (v === 'plan') planRef.value?.reload()
})

onMounted(() => {
  void probe()
})

defineExpose({ probe })
</script>

<template>
  <div v-if="anyPresent" class="fdock">
    <div class="fdock-tabs">
      <button
        v-if="has.todo"
        class="wb-btn ftab"
        :class="{ 'is-on': tab === 'todo' }"
        type="button"
        @click="pick('todo')"
      >
        <AppIcon name="list" size="ic-xs" />
        <span>{{ t('featurePanel.tabTodo') }}</span>
      </button>
      <button
        v-if="has.goal"
        class="wb-btn ftab"
        :class="{ 'is-on': tab === 'goal' }"
        type="button"
        @click="pick('goal')"
      >
        <AppIcon name="target" size="ic-xs" />
        <span>{{ t('featurePanel.tabGoal') }}</span>
      </button>
      <button
        v-if="has.plan"
        class="wb-btn ftab"
        :class="{ 'is-on': tab === 'plan' }"
        type="button"
        @click="pick('plan')"
      >
        <AppIcon name="flag" size="ic-xs" />
        <span>{{ t('featurePanel.tabPlan') }}</span>
      </button>
    </div>

    <Transition name="fdock-slide">
      <aside v-if="open && tab" class="fdock-panel wb-glass">
        <header class="fdock-head">
          <h3 class="fdock-title">
            {{ t(`featurePanel.tab${tab.charAt(0).toUpperCase()}${tab.slice(1)}`) }}
          </h3>
          <button
            class="wb-btn fdock-x"
            type="button"
            :title="t('common.close')"
            @click="tab = ''"
          >
            <AppIcon name="close" size="ic-xs" />
          </button>
        </header>
        <div class="fdock-body">
          <TodoPanel v-show="tab === 'todo'" ref="todoRef" />
          <GoalPanel v-show="tab === 'goal'" ref="goalRef" />
          <PlanPanel v-show="tab === 'plan'" ref="planRef" />
        </div>
      </aside>
    </Transition>
  </div>
</template>

<style scoped>
.fdock {
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.fdock-tabs {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-1);
  padding: 0 var(--wb-sp-4) var(--wb-sp-2);
}
.ftab {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  height: calc(22px * var(--wb-vw-scale));
  padding: 0 var(--wb-sp-2);
  border-radius: var(--wb-radius-full);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-raised);
  font-size: var(--wb-fs-2xs);
  font-weight: 600;
  color: var(--wb-ink-2);
  transition:
    background-color var(--wb-dur) var(--wb-ease),
    border-color var(--wb-dur) var(--wb-ease),
    color var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.ftab:hover {
  border-color: var(--wb-border-strong);
  transform: scale(1.04);
}
.ftab:active {
  transform: scale(var(--wb-btn-scale-press));
}
.ftab.is-on {
  background: var(--wb-accent-tint);
  border-color: var(--wb-primary);
  color: var(--wb-primary);
}
.fdock-panel {
  display: flex;
  flex-direction: column;
  max-height: calc(340px * var(--wb-vw-scale));
  margin: 0 var(--wb-sp-4) var(--wb-sp-2);
  border-radius: var(--wb-radius);
  overflow: hidden;
}
.fdock-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-bottom: 1px solid var(--wb-border);
  background: var(--wb-elev-hover);
}
.fdock-title {
  flex: 1;
  min-width: 0;
  margin: 0;
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  letter-spacing: 0.02em;
  color: var(--wb-ink);
}
.fdock-x {
  flex: none;
  width: calc(20px * var(--wb-vw-scale));
  height: calc(20px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-xs);
  color: var(--wb-muted);
}
.fdock-x:hover {
  background: var(--wb-elev-raised);
  color: var(--wb-ink);
}
.fdock-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--wb-sp-3);
}
.fdock-slide-enter-active,
.fdock-slide-leave-active {
  transition:
    opacity var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur) var(--wb-ease);
}
.fdock-slide-enter-from,
.fdock-slide-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}
</style>
