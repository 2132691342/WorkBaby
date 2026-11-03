<script setup lang="ts">
// 引擎作用域切换器：omp / pi / 全部。
// 只影响本页读哪个引擎的配置，不碰对话引擎。
// 用三个分段不用下拉：三个选项下拉要多一次点击，还看不出「全部」存在。
import { useI18n } from 'vue-i18n'
import type { EngineScope } from '@/composables/useEngineScope'

defineProps<{
  scope: EngineScope
  /** 已安装的引擎：没装的引擎不给选项，省得点进去看到空表 */
  installed: string[]
}>()

const emit = defineEmits<{ pick: [scope: EngineScope] }>()

const { t } = useI18n()

const all: { key: EngineScope; label: string }[] = [
  { key: 'omp', label: 'omp' },
  { key: 'pi', label: 'pi' },
  { key: 'all', label: '' },
]
</script>

<template>
  <div class="seg scope-seg" role="tablist" :title="t('scope.hint')">
    <button
      v-for="item in all"
      :key="item.key"
      class="seg-btn"
      :class="{ active: scope === item.key }"
      :disabled="item.key !== 'all' && !installed.includes(item.key)"
      :title="item.key !== 'all' && !installed.includes(item.key) ? t('scope.notInstalled', { engine: item.key }) : undefined"
      role="tab"
      @click="emit('pick', item.key)"
    >
      {{ item.key === 'all' ? t('scope.all') : item.label }}
    </button>
  </div>
</template>

<style scoped>
.scope-seg {
  flex: none;
}
</style>
