<script setup lang="ts">
// 待发送附件列表：相对工作区的文件标绿、区外文件标红，图片按名称识别。
import { useI18n } from 'vue-i18n'
import AppIcon from '../common/AppIcon.vue'
import type { PendingFile } from '../../utils/composerMenu'

defineProps<{
  files: PendingFile[]
  /** 区外文件条数：>0 时提交按钮禁用 */
  outsideCount: number
}>()

const emit = defineEmits<{ remove: [path: string] }>()

const { t } = useI18n()
</script>

<template>
  <div v-if="files.length" class="attach-row">
    <span
      v-for="(f, i) in files"
      :key="`${f.path}-${i}`"
      class="attach"
      :class="{ 'is-bad': f.outside }"
      :title="f.outside ? t('chat.attachOutside') : f.image ? f.name : f.path"
    >
      <AppIcon :name="f.image ? 'image' : 'doc'" size="ic-2xs" />
      <span class="attach-n">{{ f.name }}</span>
      <button class="attach-x" type="button" :title="t('chat.attachRemove')" @click="emit('remove', f.path)">
        <AppIcon name="close" size="ic-2xs" />
      </button>
    </span>
    <span class="attach-tip">{{ files.length }}</span>
  </div>
</template>

<style scoped>
/* 数量不设上限：超过两行就在本区内部滚动，不把输入框顶出屏 */
.attach-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3) 0;
  max-height: calc(var(--wb-ctl-h-sm) * 2 + var(--wb-sp-4));
  overflow-y: auto;
  overscroll-behavior: contain;
}
.attach {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  height: var(--wb-ctl-h-sm);
  padding: 0 var(--wb-sp-1) 0 var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  border: 1px solid var(--wb-border);
  background: var(--wb-elev-hover);
  color: var(--wb-ink-2);
  font-size: var(--wb-fs-2xs);
  font-weight: 600;
  max-width: calc(220px * var(--wb-vw-scale));
}
.attach.is-bad {
  border-color: var(--wb-danger);
  color: var(--wb-danger);
  background: var(--wb-danger-soft);
}
.attach-n {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.attach-x {
  display: grid;
  place-items: center;
  width: calc(18px * var(--wb-vw-scale));
  height: calc(18px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-full);
  color: var(--wb-muted);
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.attach-x:hover {
  background: var(--wb-tint-lg);
  color: var(--wb-danger);
  transform: scale(1.06);
}
.attach-x:active {
  transform: scale(var(--wb-btn-scale-press));
}
.attach-tip {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
</style>
