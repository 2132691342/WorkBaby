<script setup lang="ts">
// 差异抽屉：从工具卡点进来，看这个文件在工作区里的真实差异。
// 不用引擎给的那份：那是它当时的改动，用户可能又改过或已提交——
// 对不上的时候，真实差异才是用户要的。
import { computed, ref, watch } from 'vue'
import DiffView from '@/components/workspace/DiffView.vue'
import { api } from '@/api/http'
import { useAppStore } from '@/stores/app'
import { useFilePeek } from '@/composables/useFilePeek'
import type { GitDiff } from '@/types/api'

const app = useAppStore()
const peek = useFilePeek()

const diff = ref<GitDiff | null>(null)
const loading = ref(false)
const error = ref('')

/**
 * 问 git 要用的路径。
 *
 * 工作区内的折成相对路径（界面上短，也和别处显示的一致）；
 * 工作区外的**原样用绝对路径**——git 本身认绝对路径，
 * 而引擎常在别的项目目录里改东西，那种时候把入口藏掉等于没法看。
 *
 * 判断「在不在工作区」只是为了让界面显示得短一点，不是能不能查的判据。
 */
function toRel(target: string): string {
  const root = (app.workspace || '').replace(/[\\/]+$/, '')
  if (!root) return target
  const key = (s: string) => s.replace(/\//g, '\\').toLowerCase()
  if (!key(target).startsWith(key(root) + '\\')) return target
  return target.slice(root.length + 1)
}

const relPath = computed(() => (peek.path.value ? toRel(peek.path.value) : ''))

async function load() {
  const rel = relPath.value
  loading.value = true
  error.value = ''
  try {
    diff.value = await api.gitDiff(rel)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    diff.value = null
  } finally {
    loading.value = false
  }
}

watch(peek.path, () => {
  if (peek.path.value) void load()
  else diff.value = null
}, { immediate: true })

/** 抽屉标题：文件名优先，路径太长时只留末两段。 */
const heading = computed(() => {
  if (peek.title.value) return peek.title.value
  const p = peek.path.value
  if (!p) return ''
  const parts = p.split(/[\\/]/).filter(Boolean)
  return parts.length > 2 ? '…/' + parts.slice(-2).join('/') : p
})
</script>

<template>
  <el-drawer
    :model-value="Boolean(peek.path.value)"
    direction="rtl"
    size="46%"
    :title="heading"
    class="peek-drawer"
    @update:model-value="(v: boolean) => { if (!v) peek.close() }"
  >
    <p v-if="error" class="peek-err">{{ error }}</p>
    <DiffView
      v-else
      :diff="diff"
      :path="relPath"
      :loading="loading"
    />
  </el-drawer>
</template>

<style scoped>
.peek-err {
  margin: 0;
  padding: var(--wb-sp-4);
  font-size: var(--wb-fs-sm);
  color: var(--wb-bad);
  line-height: 1.7;
}
</style>
