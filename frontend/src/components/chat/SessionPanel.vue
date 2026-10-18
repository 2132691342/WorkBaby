<script setup lang="ts">
// 对话页左侧面板：新对话 + 搜索 + 会话列表 + 引擎状态卡。
// 折叠由顶栏按钮控制（useSessionPanel），走 class 让过渡可见。
import { useI18n } from 'vue-i18n'
import { useSessionPanel } from '../../composables/useSessionPanel'
import AppIcon from '../common/AppIcon.vue'
import EngineCard from './EngineCard.vue'
import SessionList from './SessionList.vue'

const props = defineProps<{ creating?: boolean }>()
const emit = defineEmits<{ newSession: [] }>()

const { t } = useI18n()
const { collapsed } = useSessionPanel()
</script>

<template>
  <aside class="side-panel" :class="{ 'is-collapsed': collapsed }">
    <div class="side-in">
      <div class="side-head">
        <span class="wb-label">{{ t('nav.menu') }}</span>
      </div>

      <el-button class="side-new" type="primary" :loading="props.creating" :disabled="props.creating" @click="emit('newSession')">
        <AppIcon v-if="!props.creating" name="plus" size="ic-xs" />
        <span>{{ t('chat.newSession') }}</span>
      </el-button>

      <div class="side-list">
        <SessionList @new-session="emit('newSession')" />
      </div>

      <div class="side-foot">
        <EngineCard />
      </div>
    </div>
  </aside>
</template>

<style scoped>
.side-panel {
  width: var(--wb-w-side);
  flex: none;
  min-height: 0;
  display: flex;
  background: var(--wb-surface);
  border-right: 1px solid var(--wb-border);
  overflow: hidden;
  /* 折叠用负边距 + 宽度归零：display 切换无法过渡 */
  transition:
    width var(--wb-dur-slow) var(--wb-ease),
    margin-left var(--wb-dur-slow) var(--wb-ease),
    border-color var(--wb-dur-slow) var(--wb-ease);
}
.side-panel.is-collapsed {
  width: 0;
  margin-left: calc(-1px * var(--wb-vw-scale));
  border-right-color: transparent;
  pointer-events: none;
}
.side-in {
  width: var(--wb-w-side);
  flex: none;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-4) var(--wb-sp-2) var(--wb-sp-3);
}
.side-head {
  padding: 0 var(--wb-sp-2);
}
.side-new {
  width: 100%;
  height: var(--wb-ctl-h-lg);
  gap: var(--wb-sp-1);
}
.side-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 0 var(--wb-sp-1);
}
.side-foot {
  flex: none;
  padding: 0 var(--wb-sp-1);
}
</style>
