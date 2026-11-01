<script setup lang="ts">
// 对话页左侧面板：新对话 + 会话列表 + 引擎状态卡。与设置 / 帮助页共用 SidePanel 的 sub 档
// （rail 已占位，二级目录再宽就读作两根侧栏）；折叠由顶栏按钮控制，外层只为宽度过渡可见。
import { useI18n } from 'vue-i18n'
import { useSessionPanel } from '../../composables/useSessionPanel'
import AppIcon from '../common/AppIcon.vue'
import SidePanel from '../layout/SidePanel.vue'
import EngineCard from './EngineCard.vue'
import SessionList from './SessionList.vue'

const props = defineProps<{ creating?: boolean }>()
const emit = defineEmits<{ newSession: [] }>()

const { t } = useI18n()
const { collapsed } = useSessionPanel()
</script>

<template>
  <div class="side-wrap" :class="{ 'is-collapsed': collapsed }">
    <SidePanel width="sub" :title="t('nav.menu')">
      <template #head>
        <el-button
          size="small"
          type="primary"
          :loading="props.creating"
          :disabled="props.creating"
          @click="emit('newSession')"
        >
          <AppIcon v-if="!props.creating" name="plus" size="ic-2xs" />
          <span class="ml-1">{{ t('chat.newSession') }}</span>
        </el-button>
      </template>

      <SessionList @new-session="emit('newSession')" />

      <template #foot>
        <EngineCard />
      </template>
    </SidePanel>
  </div>
</template>

<style scoped>
/* 折叠用宽度归零：display 切换无法过渡，负边距会连带消息列抖动一下 */
.side-wrap {
  display: flex;
  flex: none;
  width: var(--wb-w-subside);
  min-height: 0;
  overflow: hidden;
  transition: width var(--wb-dur-slow) var(--wb-ease);
}
/* 窄窗口里二级面板再收一档：188px 主区就只剩半个屏幕，
   消息列优先。用容器查询（不是媒体查询）——判断的是内容区宽度，不是窗口宽度。 */
@container app (max-width: 1080px) {
  .side-wrap:not(.is-collapsed) {
    width: calc(160px * var(--wb-vw-scale));
  }
}
/* 更窄时直接让位：面板与消息列抢空间时，消息列永远是主角 */
@container app (max-width: 860px) {
  .side-wrap:not(.is-collapsed) {
    width: 0;
    pointer-events: none;
  }
}
.side-wrap.is-collapsed {
  width: 0;
  pointer-events: none;
}
.side-wrap :deep(.side-panel) {
  width: 100%;
}
/* 窄档里底部卡片的留白让给内容：188px 再扣两圈 padding，读数会先被挤掉 */
.side-wrap :deep(.sp-foot) {
  padding: var(--wb-sp-2);
}
.ml-1 {
  margin-left: var(--wb-sp-1);
}
</style>
