<script setup lang="ts">
// 待发送附件条：图片给缩略图，文件给名字，每条可单独移除。
// 单独一个组件：它有自己的视觉规则（缩略图尺寸、换行、截断），
// 不该和「怎么发」挤在输入区里。
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import type { PendingAttachment } from '@/composables/useAttachments'

defineProps<{ items: PendingAttachment[] }>()
const emit = defineEmits<{ remove: [id: string] }>()

const { t } = useI18n()
</script>

<template>
  <div v-if="items.length" class="attach-strip">
    <div
      v-for="item in items"
      :key="item.id"
      class="attach-chip"
      :title="item.payload.file_path || item.name"
    >
      <img v-if="item.preview" :src="item.preview" class="attach-thumb" :alt="item.name" />
      <AppIcon v-else name="file" :size="12" />
      <span class="attach-name">{{ item.name }}</span>
      <button class="attach-x" :title="t('common.remove')" @click="emit('remove', item.id)">×</button>
    </div>
  </div>
</template>

<style scoped>
/* 横向铺开，超出换行。缩略图 30px 够认出是哪张截图。 */
.attach-strip {
  display: flex;
  flex-wrap: wrap;
  gap: var(--wb-sp-2);
}

.attach-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  max-width: 15em;
  padding: 2px 4px 2px 2px;
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-sm);
  background: var(--wb-window);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-ink-2);
}

.attach-thumb {
  width: 30px;
  height: 30px;
  object-fit: cover;
  border-radius: var(--wb-r-xs);
}

.attach-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--wb-font-mono);
}

.attach-x {
  border: 0;
  background: transparent;
  padding: 0 2px;
  color: var(--wb-mute);
  font-size: 1.1em;
  line-height: 1;
  cursor: pointer;
}

.attach-x:hover {
  color: var(--wb-bad);
}
</style>
