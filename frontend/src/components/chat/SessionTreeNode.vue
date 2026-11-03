<script setup lang="ts">
// 会话树的单个节点：自己渲染自己（递归组件）。
// 树的深度不可预知——写死三层嵌套的话，分到第四层就画不出来了。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useSessionStore } from '../../stores/session'
import { useChatStore } from '../../stores/chat'
import { useToastStore } from '../../stores/toast'
import type { SessionTreeNodeVO } from '../../types/api'
import { errorText } from '../../utils/errors'

const props = defineProps<{ node: SessionTreeNodeVO }>()
const emit = defineEmits<{ done: [] }>()

const { t } = useI18n()
const session = useSessionStore()
const chat = useChatStore()
const toast = useToastStore()

const busy = computed(() => chat.running || chat.finalizing)
/** 叶子判定：没有子节点的就是末端，可以切回去。 */
const isLeaf = computed(() => props.node.node_children.length === 0)

/** 从这里另开一支：旧的那段保留，随时能切回来。 */
async function forkHere() {
  if (busy.value) {
    toast.info(t('chat.busyNewSession'))
    return
  }
  try {
    await session.branchFrom(props.node.node_entry_id)
    emit('done')
  } catch (e) {
    toast.bad(errorText(e))
  }
}

/** 切回某个旧分支末端。 */
async function gotoLeaf() {
  if (busy.value) {
    toast.info(t('chat.busyNewSession'))
    return
  }
  try {
    await session.switchToLeaf(props.node.node_entry_id)
    emit('done')
  } catch (e) {
    toast.bad(errorText(e))
  }
}
</script>

<template>
  <li class="tree-item">
    <div class="tree-row">
      <span class="tree-dot" :class="{ 'is-path': node.node_on_path, 'is-leaf': isLeaf }" />
      <span class="tree-tx" :title="node.node_preview">
        {{ node.node_preview || t('tree.emptyMessage') }}
      </span>
      <span v-if="node.node_on_path" class="tree-badge">{{ t('tree.current') }}</span>
      <button
        v-else-if="isLeaf"
        class="tree-act wb-press"
        type="button"
        :disabled="busy"
        @click="gotoLeaf"
      >
        {{ t('tree.switchTo') }}
      </button>
      <button
        v-else
        class="tree-act wb-press"
        type="button"
        :disabled="busy"
        @click="forkHere"
      >
        {{ t('tree.forkHere') }}
      </button>
    </div>
    <ul v-if="node.node_children.length" class="tree-list is-child">
      <SessionTreeNode
        v-for="c in node.node_children"
        :key="c.node_entry_id"
        :node="c"
        @done="emit('done')"
      />
    </ul>
  </li>
</template>

<style scoped>
.tree-item {
  position: relative;
}
.tree-list {
  margin: 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
}
/* 子层缩进 + 左侧竖线：一眼看出「这是从上面分出来的」 */
.tree-list.is-child {
  margin-left: calc(13px * var(--wb-vw-scale));
  padding-left: calc(10px * var(--wb-vw-scale));
  border-left: 1px solid var(--wb-border);
}
.tree-row {
  position: relative;
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-1) 0;
  min-width: 0;
}
.tree-dot {
  flex: none;
  width: calc(7px * var(--wb-vw-scale));
  height: calc(7px * var(--wb-vw-scale));
  border-radius: 50%;
  background: var(--wb-border-strong);
}
/* 当前路径上的节点用主色：主干与分支一眼分得开 */
.tree-dot.is-path {
  background: var(--wb-brand-1);
}
.tree-dot.is-leaf {
  background: var(--wb-muted);
}
.tree-tx {
  flex: 1;
  min-width: 0;
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tree-badge {
  flex: none;
  padding: 0 var(--wb-sp-2);
  border-radius: var(--wb-radius-xs);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  color: var(--wb-primary);
  background: var(--wb-accent-tint);
}
.tree-act {
  flex: none;
  padding: 0 var(--wb-sp-2);
  height: calc(20px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-xs);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  color: var(--wb-primary);
  border: 1px solid var(--wb-border);
}
.tree-act:hover:not(:disabled) {
  border-color: var(--wb-brand-2);
  background: var(--wb-accent-tint);
}
.tree-act:disabled {
  opacity: 0.5;
  cursor: default;
}
</style>
