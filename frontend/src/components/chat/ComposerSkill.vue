<script setup lang="ts">
// 技能芯片：一条消息最多挂一个技能，选中后正文保持干净，技能在这里显形。
// 芯片只记名字；正文由后端在发请求时按需读取，界面上不囤技能全文。
import { useI18n } from 'vue-i18n'
import AppIcon from '../common/AppIcon.vue'

defineProps<{
  /** 已挂上的技能名；空串 = 没挂技能 */
  skill: string
  /** 技能清单里查得到的描述（查不到时只显示名字） */
  description?: string
}>()

const emit = defineEmits<{ clear: [] }>()

const { t } = useI18n()
</script>

<template>
  <!-- 没有技能时整块不占位：空芯片比没有芯片更让人困惑 -->
  <Transition name="chip-in">
    <div v-if="skill" class="skill-row">
      <span class="skill-chip" :title="description || skill">
        <AppIcon name="sparkles" size="ic-2xs" class="chip-ic" />
        <b class="chip-n">{{ skill }}</b>
        <span v-if="description" class="chip-d">{{ description }}</span>
        <button
          class="chip-x"
          type="button"
          :title="t('chat.skillDetach')"
          @click="emit('clear')"
        >
          <AppIcon name="close" size="ic-2xs" />
        </button>
      </span>
      <span class="skill-hint">{{ t('chat.skillAttachedHint') }}</span>
    </div>
  </Transition>
</template>

<style scoped>
.skill-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3) 0;
  min-width: 0;
}
.skill-chip {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-1);
  min-width: 0;
  max-width: calc(320px * var(--wb-vw-scale));
  height: var(--wb-ctl-h-sm);
  padding: 0 var(--wb-sp-1) 0 var(--wb-sp-2);
  border-radius: var(--wb-radius-full);
  border: 1px solid var(--wb-accent-tint);
  background: var(--wb-accent-tint);
  color: var(--wb-primary);
  font-size: var(--wb-fs-2xs);
  transition:
    border-color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.skill-chip:hover {
  border-color: var(--wb-primary);
  transform: scale(1.02);
}
.chip-ic {
  flex: none;
}
.chip-n {
  font-weight: 700;
  white-space: nowrap;
}
.chip-d {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--wb-muted);
}
.chip-x {
  display: grid;
  place-items: center;
  flex: none;
  width: calc(18px * var(--wb-vw-scale));
  height: calc(18px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-full);
  color: var(--wb-primary);
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.chip-x:hover {
  background: var(--wb-tint-lg);
  transform: scale(1.1);
}
.chip-x:active {
  transform: scale(var(--wb-btn-scale-press));
}
.skill-hint {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  white-space: nowrap;
}
.chip-in-enter-active {
  transition:
    opacity var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.chip-in-leave-active {
  transition: opacity var(--wb-dur-fast) var(--wb-ease);
}
.chip-in-enter-from,
.chip-in-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
@media (prefers-reduced-motion: reduce) {
  .skill-chip:hover,
  .chip-x:hover,
  .chip-x:active {
    transform: none;
  }
}
</style>
