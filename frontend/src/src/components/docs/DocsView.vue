<script setup lang="ts">
/**
 * 文档视图：左侧内置教程目录 + 右侧 Markdown 正文。
 *
 * <p>加载中显示骨架屏，失败显示可重试的错误条，无文档时显示空状态。
 */
import { computed, onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { RefreshCw, ScrollText } from '@/components/common/icons'
import { useDocsStore } from '@/stores/docs'
import MarkdownRenderer from '@/components/chat/MarkdownRenderer.vue'
import Skeleton from '@/components/common/Skeleton.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { t } from '@/i18n'

const docStore = useDocsStore()
const { docs, selected, error, loading } = storeToRefs(docStore)
const { open } = docStore

/** 当前选中文档名，作为 el-menu 的高亮项。 */
const activeName = computed(() => selected.value?.name ?? '')

onMounted(() => {
  void docStore.load()
})
</script>

<template>
  <div class="scroll wb-ui">
    <div class="wrap" style="max-width: 1000px">
      <!-- Hero（原型 19 屏） -->
      <header class="hero">
        <div class="tile"><ScrollText class="ic" /></div>
        <div>
          <h1>{{ t('docs.title') }}</h1>
          <p>{{ t('docs.subtitle') }}</p>
        </div>
      </header>

      <!-- 错误条（显眼 + 重试） -->
      <div v-if="error" class="alert a-danger">
        <span>{{ error }}</span>
        <button class="btn btn-sm" @click="docStore.load()">
          <RefreshCw class="ic ic-sm" />
          {{ t('common.retry') }}
        </button>
      </div>

      <div class="grid grid-cols-1 gap-5 lg:grid-cols-5">
        <!-- 左侧列表 -->
        <aside class="card p-3 lg:col-span-1">
          <el-menu :default-active="activeName" class="wb-doc-menu border-r-0!" @select="open">
            <el-menu-item v-for="d in docs" :key="d.name" :index="d.name">
              <span class="truncate">{{ d.title }}</span>
            </el-menu-item>
          </el-menu>
          <div v-if="!loading && docs.length === 0 && !error" class="py-4 text-center text-xs text-wb-muted">
            {{ t('docs.empty') }}
          </div>
        </aside>

        <!-- 右侧详情 -->
        <section class="card p-5 lg:col-span-4">
          <!-- Loading 骨架 -->
          <Skeleton v-if="loading" :lines="8" />

          <!-- 已选中：渲染 Markdown -->
          <template v-else-if="selected">
            <h2 class="mb-3 text-lg font-semibold text-wb-ink">{{ selected.title }}</h2>
            <MarkdownRenderer :content="selected.content" :streaming="false" />
          </template>

          <!-- 空状态（无任何文档且无错误） -->
          <EmptyState
            v-else-if="!error"
            :title="t('docs.emptyTitle')"
            :subtitle="t('docs.emptyHint')"
          />
        </section>
      </div>
    </div>
  </div>
</template>

<style scoped>
.wb-doc-menu {
  --el-menu-bg-color: transparent;
  --el-menu-text-color: var(--wb-muted);
  --el-menu-active-color: var(--wb-primary-strong);
  --el-menu-hover-bg-color: color-mix(in srgb, var(--wb-primary) 6%, transparent);
  --el-menu-item-height: 36px;
  --el-menu-base-level-padding: 8px;
  background: transparent;
}
.wb-doc-menu :deep(.el-menu-item) {
  border-radius: 8px;
  font-size: 13px;
}
</style>