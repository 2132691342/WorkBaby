<script setup lang="ts">
// 可用模型表（只读）：引擎报上来的「现在能选哪些」，不是配置文件的声明
// （两者会不一致，用户要看的是前者）。不提供编辑入口：要改去服务商那节。
import { useI18n } from 'vue-i18n'
import type { ModelRow } from '@/types/api'

const props = defineProps<{
  models: ModelRow[]
  /** 引擎没起来：表格空着会让人以为是「没有模型」，所以要把原因说出来 */
  engineMissing: boolean
}>()

const { t } = useI18n()
</script>

<template>
  <section>
    <div class="wb-sec-head">
      <span class="wb-sec-title">{{ t('chat.model') }} <span class="count">{{ props.models.length }}</span></span>
    </div>
    <p v-if="props.engineMissing" class="hint">{{ t('chat.engineDown') }} — {{ t('models.emptyHint') }}</p>
    <el-table v-else :data="props.models" size="small" max-height="420">
      <el-table-column prop="provider" :label="t('models.providers')" width="130" />
      <el-table-column :label="t('models.modelId')" min-width="200">
        <template #default="{ row }">
          <span class="wb-mono selectable">{{ row.id }}</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('models.contextWindow')" width="120" align="right">
        <template #default="{ row }">
          <span class="wb-mono">{{ row.context_window ? row.context_window.toLocaleString() : '—' }}</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('models.maxTokens')" width="100" align="right">
        <template #default="{ row }">
          <span class="wb-mono">{{ row.max_tokens ? row.max_tokens.toLocaleString() : '—' }}</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('models.reasoning')" width="92" align="center">
        <template #default="{ row }">
          <span v-if="row.reasoning" class="tag ok">{{ t('common.yes') }}</span>
          <span v-else class="tag">—</span>
        </template>
      </el-table-column>
      <el-table-column :label="t('models.input')" width="120">
        <template #default="{ row }">
          <span class="wb-mono">{{ (row.input ?? []).join(',') || 'text' }}</span>
        </template>
      </el-table-column>
    </el-table>
    <p class="hint">{{ t('models.testHint') }}</p>
  </section>
</template>

<style scoped>
.count {
  font-family: var(--wb-font-mono);
  color: var(--wb-mute);
  letter-spacing: 0;
  text-transform: none;
}

.hint {
  margin: var(--wb-sp-2) 0 0;
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}
</style>
