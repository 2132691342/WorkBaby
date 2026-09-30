<script setup lang="ts">
// 工具面板：把「助手现在能干什么」摊开给用户看。
// 模型每轮都在读这批声明，用户却没有任何入口；这里按分类展示用途、风险与参数，
// 并允许按个停用。清单来自 /tools，与模型实际拿到的是同一份注册表投影。
import { computed, onMounted, ref } from 'vue'
import * as api from '../../api'
import type { ToolVO } from '../../types/api'
import { useToastStore } from '../../stores/toast'
import AppIcon from '../common/AppIcon.vue'
import EmptyState from '../common/EmptyState.vue'
import PageState from '../common/PageState.vue'

const tools = ref<ToolVO[]>([])
const loading = ref(true)
const error = ref('')
const busy = ref('')
const toast = useToastStore()

// 分类的展示顺序与中文名：顺序按「用户最常打交道的」排前面。
const CATEGORY_ORDER = ['file', 'shell', 'code', 'web', 'data'] as const
const CATEGORY_META: Record<string, { name: string; icon: string; desc: string }> = {
  file: { name: '文件', icon: 'doc', desc: '读你的文件、列目录、找内容' },
  shell: { name: '命令', icon: 'terminal', desc: '在 Windows 上执行命令' },
  code: { name: '代码', icon: 'braces', desc: '跑内置 Python 做计算与处理' },
  web: { name: '联网', icon: 'globe', desc: '搜网页、抓正文' },
  data: { name: '资料', icon: 'database', desc: '查你自己导入的知识库' },
}

const RISK_META: Record<string, { name: string; cls: string }> = {
  low: { name: '低风险', cls: 'r-low' },
  medium: { name: '中风险', cls: 'r-medium' },
  high: { name: '高风险', cls: 'r-high' },
}

const grouped = computed(() =>
  CATEGORY_ORDER.map((key) => ({
    key,
    meta: CATEGORY_META[key],
    items: tools.value.filter((t) => t.category === key),
  })).filter((g) => g.items.length > 0),
)

const enabledCount = computed(() => tools.value.filter((t) => t.enabled).length)

async function load() {
  loading.value = true
  error.value = ''
  try {
    tools.value = await api.tools.list()
  } catch (e) {
    error.value = (e as Error)?.message || '读取工具清单失败'
  } finally {
    loading.value = false
  }
}

async function toggle(t: ToolVO) {
  busy.value = t.name
  try {
    await api.tools.toggle(t.name, !t.enabled)
    t.enabled = !t.enabled
    toast.ok(t.enabled ? `${t.label} 已启用，下一轮生效` : `${t.label} 已停用，下一轮生效`)
  } catch (e) {
    toast.bad(`切换失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

onMounted(load)
</script>

<template>
  <div class="tp">
    <div class="tp-bar">
      <span class="tp-sum">
        共 <b>{{ tools.length }}</b> 个工具，当前启用 <b>{{ enabledCount }}</b> 个
      </span>
      <span class="tp-tip">助手每轮对话都会看到下面这些能力，停用后下一轮生效</span>
      <button class="btn btn-sm btn-ghost" type="button" :disabled="loading" @click="load">
        <AppIcon name="refresh" size="ic-xs" /> 刷新
      </button>
    </div>

    <PageState v-if="loading" loading />
    <div v-else-if="error" class="alert a-danger">
      <AppIcon name="alert" />
      <div>
        <div class="a-t">读取失败</div>
        <div>{{ error }}</div>
      </div>
    </div>
    <EmptyState v-else-if="!grouped.length" icon="wrench" title="还没有可用工具" sub="检查一下程序目录下的内置资源是否完整。" />

    <section v-for="g in grouped" :key="g.key" class="grp">
      <header class="grp-head">
        <AppIcon :name="g.meta.icon" size="ic-sm" />
        <b>{{ g.meta.name }}</b>
        <span class="grp-desc">{{ g.meta.desc }}</span>
      </header>

      <div class="cards">
        <article v-for="t in g.items" :key="t.name" class="card tcard" :class="{ off: !t.enabled }">
          <div class="tc-top">
            <b class="tc-name">{{ t.label }}</b>
            <code class="tc-code">{{ t.name }}</code>
          </div>
          <p class="tc-desc">{{ t.description }}</p>
          <div class="tc-meta">
            <span class="risk" :class="RISK_META[t.risk]?.cls || 'r-low'">
              {{ RISK_META[t.risk]?.name || '低风险' }}
            </span>
            <span v-if="t.approval" class="tag">先问你</span>
            <span class="tag tag-quiet">{{ t.mode === 'parallel' ? '可并发' : '按顺序' }}</span>
          </div>
          <div v-if="t.params.length" class="tc-params">
            <span v-for="p in t.params" :key="p" class="param">{{ p }}</span>
          </div>
          <div class="tc-foot">
            <button
              class="btn btn-sm"
              :class="t.enabled ? 'btn-outline' : 'btn-primary'"
              type="button"
              :disabled="busy === t.name"
              @click="toggle(t)"
            >
              {{ t.enabled ? '停用' : '启用' }}
            </button>
          </div>
        </article>
      </div>
    </section>
  </div>
</template>

<style scoped>
.tp {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-5);
  min-width: 0;
}
.tp-bar {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  flex-wrap: wrap;
  padding-bottom: var(--wb-sp-1);
}
.tp-sum {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-2);
}
.tp-sum b {
  color: var(--wb-ink);
  font-variant-numeric: tabular-nums;
}
.tp-tip {
  flex: 1;
  min-width: 0;
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.grp {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
  min-width: 0;
}
.grp-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  color: var(--wb-ink-2);
}
.grp-head b {
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}
.grp-desc {
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: var(--wb-sp-3);
  min-width: 0;
}
.tcard {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
  min-width: 0;
}
/* 停用的工具不隐藏：用户要能看见「有这个能力，但我关掉了」 */
.tcard.off {
  opacity: 0.62;
}
.tc-top {
  display: flex;
  align-items: baseline;
  gap: var(--wb-sp-2);
  min-width: 0;
}
.tc-name {
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}
.tc-code {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
  overflow: hidden;
  text-overflow: ellipsis;
}
.tc-desc {
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink-2);
  line-height: var(--wb-lh-base);
}
.tc-meta {
  display: flex;
  gap: var(--wb-sp-2);
  flex-wrap: wrap;
}
.risk,
.tag {
  font-size: var(--wb-fs-xs);
  padding: 1px 7px;
  border-radius: var(--wb-radius-full);
  border: 1px solid var(--wb-line-2);
  color: var(--wb-muted);
  white-space: nowrap;
}
.r-low {
  border-color: var(--wb-success);
  color: var(--wb-success);
}
.r-medium {
  border-color: var(--wb-warning);
  color: var(--wb-warning);
}
.r-high {
  border-color: var(--wb-danger);
  color: var(--wb-danger);
}
.tag-quiet {
  opacity: 0.75;
}
.tc-params {
  display: flex;
  gap: var(--wb-sp-1);
  flex-wrap: wrap;
}
.param {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-xs);
  padding: 1px 6px;
  border-radius: var(--wb-radius-xs);
  background: var(--wb-raise);
  border: 1px solid var(--wb-line);
  color: var(--wb-ink-2);
}
.tc-foot {
  margin-top: auto;
  padding-top: var(--wb-sp-1);
}
</style>
