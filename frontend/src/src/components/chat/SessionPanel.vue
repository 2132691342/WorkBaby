<script setup lang="ts">
// 对话页会话面板：新对话 + 分组会话列表；折叠由标题栏按钮控制（useSessionPanel）。
import { useSessionPanel } from '../../composables/useSessionPanel'
import AppIcon from '../common/AppIcon.vue'
import SessionList from './SessionList.vue'

const props = defineProps<{ creating?: boolean }>()
const emit = defineEmits<{ newSession: [] }>()

const { collapsed } = useSessionPanel()
</script>

<template>
  <!-- 折叠走 class 而不是 v-show：负边距滑出的过渡才看得见（v-show 的 display 切换无法过渡） -->
  <aside class="side" :class="{ collapsed }">
    <button
      class="btn btn-lav side-btn"
      type="button"
      :class="{ 'is-loading': props.creating }"
      :disabled="props.creating"
      @click="emit('newSession')"
    >
      <AppIcon name="plus" /> 新对话
    </button>
    <div class="sess">
      <SessionList />
    </div>
  </aside>
</template>

<style scoped>
.side-btn {
  margin: var(--wb-sp-3) var(--wb-sp-3) var(--wb-sp-2);
  width: calc(100% - var(--wb-sp-6));
  justify-content: flex-start;
  gap: var(--wb-sp-2);
}
.sess {
  flex: 1 1 40%;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
</style>
