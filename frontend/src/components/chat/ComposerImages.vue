<script setup lang="ts">
// 待发送的图片条：缩略图 + 名字 + 删除，超过一屏就横向滚动，不顶高输入框。
import { useI18n } from 'vue-i18n'
import AppIcon from '../common/AppIcon.vue'
import type { PendingImage } from '../../utils/composerMenu'

defineProps<{
  images: PendingImage[]
}>()

const emit = defineEmits<{ remove: [id: number] }>()

const { t } = useI18n()
</script>

<template>
  <div v-if="images.length" class="img-row">
    <div
      v-for="img in images"
      :key="img.id"
      class="thumb"
      :title="`${img.name} · ${img.width}×${img.height}`"
    >
      <img :src="`data:${img.mime};base64,${img.base64}`" :alt="img.name" />
      <button
        class="thumb-x"
        type="button"
        :title="t('chat.attachRemove')"
        @click="emit('remove', img.id)"
      >
        <AppIcon name="close" size="ic-2xs" />
      </button>
    </div>
    <span class="img-tip">{{ t('chat.attachImageCount', { n: images.length }) }}</span>
  </div>
</template>

<style scoped>
.img-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3) 0;
  overflow-x: auto;
  overscroll-behavior: contain;
  scrollbar-width: thin;
}
.thumb {
  position: relative;
  flex: none;
  width: calc(52px * var(--wb-vw-scale));
  height: calc(52px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-sm);
  border: 1px solid var(--wb-border);
  overflow: hidden;
  background: var(--wb-elev-hover);
  transition:
    border-color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.thumb:hover {
  border-color: var(--wb-border-strong);
  transform: scale(1.04);
}
.thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
.thumb-x {
  position: absolute;
  top: 1px;
  right: 1px;
  display: grid;
  place-items: center;
  width: calc(16px * var(--wb-vw-scale));
  height: calc(16px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-full);
  background: var(--wb-overlay-strong);
  color: var(--wb-on-primary);
  opacity: 0;
  transition:
    opacity var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.thumb:hover .thumb-x,
.thumb-x:focus-visible {
  opacity: 1;
}
.thumb-x:hover {
  transform: scale(1.12);
}
.thumb-x:active {
  transform: scale(var(--wb-btn-scale-press));
}
.img-tip {
  flex: none;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
@media (prefers-reduced-motion: reduce) {
  .thumb:hover,
  .thumb-x:hover,
  .thumb-x:active {
    transform: none;
  }
}
</style>
