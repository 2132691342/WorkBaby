<script setup lang="ts">
// 功能活状态抽屉：待办 / 目标 / 计划 三个结构化面板，贴在对话主区右侧。
// 这四项是「这一轮在推进什么」的唯一可信来源，但都是长清单：铺进消息流会淹没正文。
// widget 事件只当「有新变化」的信号，正文一律拉结构化 VO。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useChatStore } from '../../stores/chat'
import TodoPanel from './TodoPanel.vue'
import GoalPanel from './GoalPanel.vue'
import PlanPanel from './PlanPanel.vue'
import AppIcon from '../common/AppIcon.vue'

const { t } = useI18n()
const chat = useChatStore()

/** 三个面板的 tab；空串 = 收起 */
const tab = ref<'' | 'todo' | 'goal' | 'plan'>('')

const todoRef = ref<InstanceType<typeof TodoPanel> | null>(null)
const goalRef = ref<InstanceType<typeof GoalPanel> | null>(null)
const planRef = ref<InstanceType<typeof PlanPanel> | null>(null)

/** 哪些 tab 有内容：widget 事件的 key 就是功能码 */
const present = computed(() => ({
  todo: !!chat.engineWidgets.todo,
  goal: !!chat.engineStatus.goal,
  plan: !!chat.engineWidgets.plan,
}))

const anyPresent = computed(() => present.value.todo || present.value.goal || present.value.plan)
const open = computed(() => !!tab.value)

/** widget 变了就重拉对应面板：widget 只报「变了」，正文以 VO 为准 */
watch(
  () => chat.engineWidgets,
  () => {
    if (tab.value === 'todo') todoRef.value?.reload()
    if (tab.value === 'plan') planRef.value?.reload()
  },
  { deep: true },
)
watch(
  () => chat.engineStatus.goal,
  () => {
    if (tab.value === 'goal') goalRef.value?.reload()
  },
)

function pick(next: 'todo' | 'goal' | 'plan') {
  tab.value = tab.value === next ? '' : next
}

/** 打开时立刻拉一次：widget 可能是上一轮推的，内容已变 */
watch(tab, (v) => {
  if (v === 'todo') todoRef.value?.reload()
  if (v === 'goal') goalRef.value?.reload()
  if (v === 'plan') planRef.value?.reload()
})
</script>

<template>
  <div v-if="anyPresent" class="fdock">
    <div class="fdock-tabs">
      <button
        v-if="present.todo"
        class="wb-btn ftab"
        :class="{ 'is-on': tab === 'todo' }"
        type="button"
        @click="pick('todo')"
      >
        <AppIcon name="list" size="ic-xs" />
        <span>{{ t('featurePanel.tabTodo') }}</span>
      </button>
      <button
        v-if="present.goal"
        class="wb-btn ftab"
        :class="{ 'is-on': tab === 'goal' }"
        type="button"
        @click="pick('goal')"
      >
        <AppIcon name="target" size="ic-xs" />
        <span>{{ t('featurePanel.tabGoal') }}</span>
      </button>
      <button
        v-if="present.plan"
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
    color var(--wb-dur) var(--wb-ease);
}
.ftab:hover {
  border-color: var(--wb-border-strong);
}
.ftab.is-on {
  background: var(--wb-accent-tint);
  border-color: var(--wb-primary);
  color: var(--wb-primary);
}
.fdock-panel {
  display: flex;
  flex-direction: column;
  max-height: calc(300px * var(--wb-vw-scale));
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
