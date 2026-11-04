<script setup lang="ts">
// 引擎作用域条。
//
// 本壳只有一个引擎，所以它不再是「选哪个引擎」，而是一句状态说明：
// 这些清单属于沙箱里的那一个 pi。留着这个组件是为了让页面结构不为
// 「只有一个」长出分支。
import { useI18n } from 'vue-i18n'
import type { EngineScope } from '@/composables/useEngineScope'

defineProps<{
  scope: EngineScope
}>()

const emit = defineEmits<{ pick: [scope: EngineScope] }>()

const { t } = useI18n()
</script>

<template>
  <div class="scope-bar">
    <span class="scope-label">{{ t('nav.engine') }}</span>
    <span class="scope-chip" :title="t('common.engineScoped')">pi</span>
    <button
      v-if="scope !== 'pi'"
      class="wb-btn text sm"
      @click="emit('pick', 'pi')"
    >
      {{ t('common.reset') }}
    </button>
  </div>
</template>

<style scoped>
.scope-bar {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-2);
}

.scope-label {
  font-size: var(--wb-fs-3xs);
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--wb-faint);
}

.scope-chip {
  padding: 1px 8px;
  border: 1px solid var(--wb-brand-3);
  border-radius: var(--wb-r-full);
  background: var(--wb-brand-soft);
  color: var(--wb-brand);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  font-weight: 700;
}
</style>
