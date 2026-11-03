<script setup lang="ts">
// 会话树：把 JSONL 里的分叉结构画成树，让人看得见「我走过哪条路」。
// 后端早已把整棵树给出来（detail_tree）；缺的是把它画出来——
// 只能靠气泡上的分叉按钮盲目回溯，是分叉能力形同虚设的主要原因。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useSessionStore } from '../../stores/session'
import AppIcon from '../common/AppIcon.vue'
import SessionTreeNode from './SessionTreeNode.vue'

defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: [] }>()

const { t } = useI18n()
const session = useSessionStore()

/** 树根：虚拟根时它的 children 就是各条主干。 */
const root = computed(() => session.treeRoot)
</script>

<template>
  <div v-if="open" class="tree-wrap">
    <div class="tree-hd">
      <span class="wb-label">{{ t('tree.title') }}</span>
      <button class="tree-x" type="button" :title="t('common.close')" @click="emit('close')">
        <AppIcon name="close" size="ic-xs" />
      </button>
    </div>

    <p class="tree-hint">{{ t('tree.hint') }}</p>

    <div v-if="!root || !root.node_children.length" class="tree-empty">{{ t('tree.empty') }}</div>

    <ul v-else class="tree-list">
      <SessionTreeNode
        v-for="n in root.node_children"
        :key="n.node_entry_id || 'root'"
        :node="n"
        @done="emit('close')"
      />
    </ul>
  </div>
</template>

<style scoped>
.tree-wrap {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-3);
  border-radius: var(--wb-radius-lg);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-raised);
  box-shadow: var(--wb-card-shadow);
  animation: tree-in var(--wb-dur) var(--wb-ease);
  max-height: calc(60vh);
  overflow: auto;
}
@keyframes tree-in {
  from {
    opacity: 0;
    transform: translateY(-6px);
  }
}
.tree-hd {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.tree-x {
  display: grid;
  place-items: center;
  width: calc(22px * var(--wb-vw-scale));
  height: calc(22px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-xs);
  color: var(--wb-muted);
  transition: color var(--wb-dur-fast) var(--wb-ease);
}
.tree-x:hover {
  color: var(--wb-ink);
}
.tree-hint {
  margin: 0;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  line-height: 1.4;
}
.tree-empty {
  padding: var(--wb-sp-2);
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.tree-list {
  margin: 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
}
</style>
