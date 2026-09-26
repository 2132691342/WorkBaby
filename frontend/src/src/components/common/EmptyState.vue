<script setup lang="ts">
import { computed } from 'vue'
import { t } from '@/i18n'

/**
 * 空状态组件：内置多套 SVG 插画，按 variant 选择。全应用空态的唯一实现
 * （列表页经 PageState 复用，侧栏用 size="sm"）。
 * 用法：`<EmptyState variant="empty-chat" title="还没有对话" subtitle="开始聊天吧~" />`
 */

export type EmptyStateVariant =
  | 'empty-chat'
  | 'empty-files'
  | 'empty-search'
  | 'empty-knowledge'
  | 'error-404'
  | 'error-general'
  | 'loading'

const props = withDefaults(
  defineProps<{
    variant?: EmptyStateVariant
    title?: string
    subtitle?: string
    animated?: boolean
    /** 尺寸：sm 用于侧栏/面板内的紧凑空态（插画缩至 64px、上下留白收窄）。 */
    size?: 'sm' | 'md'
    /**
     * 是否展示插画。false 用于窄容器（侧栏、表格内）：
     * 120px 插画在 240px 宽的侧栏里会盖住文案，且插图在「只是没有数据」时是噪音。
     */
    illustration?: boolean
  }>(),
  {
    variant: 'empty-chat',
    title: undefined,
    subtitle: undefined,
    animated: true,
    size: 'md',
    illustration: true
  }
)

/** 默认文案走 i18n（EN 环境不应出现中文兜底）。 */
const defaultTitles = computed<Record<EmptyStateVariant, string>>(() => ({
  'empty-chat': t('empty.chat.title'),
  'empty-files': t('empty.files.title'),
  'empty-search': t('empty.search.title'),
  'empty-knowledge': t('empty.knowledge.title'),
  'error-404': t('empty.notFound.title'),
  'error-general': t('empty.error.title'),
  loading: t('empty.loading.title')
}))

const defaultSubtitles = computed<Record<EmptyStateVariant, string>>(() => ({
  'empty-chat': t('empty.chat.subtitle'),
  'empty-files': t('empty.files.subtitle'),
  'empty-search': t('empty.search.subtitle'),
  'empty-knowledge': t('empty.knowledge.subtitle'),
  'error-404': t('empty.notFound.subtitle'),
  'error-general': t('empty.error.subtitle'),
  loading: t('empty.loading.subtitle')
}))

const displayTitle = computed(() => props.title ?? defaultTitles.value[props.variant])
const displaySubtitle = computed(() => props.subtitle ?? defaultSubtitles.value[props.variant])
</script>

<template>
  <div class="wb-empty" :class="size === 'sm' ? 'is-sm' : ''">
    <!-- 空状态 SVG 插画 -->
    <div v-if="illustration" :class="[animated ? 'wb-float' : '', size === 'sm' ? 'wb-empty-sm' : '']">
      <!-- 空聊天 -->
      <svg
        v-if="variant === 'empty-chat'"
        width="120"
        height="120"
        viewBox="0 0 120 120"
        fill="none"
        class="mx-auto"
      >
        <!-- 背景圆 -->
        <circle cx="60" cy="60" r="55" fill="var(--wb-primary)" fill-opacity="0.1" />
        <!-- 小猫咪身体 -->
        <ellipse cx="60" cy="70" rx="28" ry="24" fill="#ffb7c9" />
        <!-- 小猫咪头 -->
        <circle cx="60" cy="45" r="20" fill="#ffc8d6" />
        <!-- 左耳 -->
        <polygon points="45,30 50,42 38,38" fill="#ffb7c9" />
        <!-- 右耳 -->
        <polygon points="75,30 70,42 82,38" fill="#ffb7c9" />
        <!-- 左耳内 -->
        <polygon points="46,33 50,40 41,37" fill="#ff8fab" />
        <!-- 右耳内 -->
        <polygon points="74,33 70,40 79,37" fill="#ff8fab" />
        <!-- 左眼 -->
        <ellipse cx="52" cy="44" rx="3" ry="4" fill="#4a3f45" />
        <!-- 右眼 -->
        <ellipse cx="68" cy="44" rx="3" ry="4" fill="#4a3f45" />
        <!-- 眼睛高光 -->
        <circle cx="53" cy="42" r="1" fill="white" />
        <circle cx="69" cy="42" r="1" fill="white" />
        <!-- 鼻子 -->
        <ellipse cx="60" cy="50" rx="2" ry="1.5" fill="#ff8fab" />
        <!-- 嘴巴 -->
        <path d="M56,53 Q60,57 64,53" stroke="#4a3f45" stroke-width="1.5" fill="none" stroke-linecap="round" />
        <!-- 胡须 -->
        <line x1="35" y1="48" x2="48" y2="50" stroke="#d4a5b0" stroke-width="1" />
        <line x1="35" y1="52" x2="48" y2="52" stroke="#d4a5b0" stroke-width="1" />
        <line x1="72" y1="50" x2="85" y2="48" stroke="#d4a5b0" stroke-width="1" />
        <line x1="72" y1="52" x2="85" y2="52" stroke="#d4a5b0" stroke-width="1" />
        <!-- 尾巴 -->
        <path d="M85,75 Q95,65 90,55" stroke="#ffb7c9" stroke-width="6" fill="none" stroke-linecap="round" />
        <!-- 对话气泡（表面色，随主题） -->
        <rect x="72" y="20" width="24" height="16" rx="8" fill="var(--wb-surface)" stroke="var(--wb-primary)" stroke-width="1.5" />
        <text x="84" y="31" text-anchor="middle" font-size="10" fill="var(--wb-primary)">?</text>
      </svg>

      <!-- 空文件 -->
      <svg
        v-else-if="variant === 'empty-files'"
        width="120"
        height="120"
        viewBox="0 0 120 120"
        fill="none"
        class="mx-auto"
      >
        <circle cx="60" cy="60" r="55" fill="var(--wb-sky)" fill-opacity="0.15" />
        <!-- 文件夹 -->
        <path d="M35,45 L35,80 C35,83 38,85 41,85 L79,85 C82,85 85,83 85,80 L85,50 C85,47 82,45 79,45 L55,45 L50,40 L41,40 C38,40 35,42 35,45 Z" fill="#9bd0f5" />
        <!-- 文件夹标签 -->
        <rect x="40" y="38" width="16" height="6" rx="2" fill="#64b5f6" />
        <!-- 小兔子 -->
        <ellipse cx="60" cy="68" rx="12" ry="10" fill="#e8b04b" />
        <circle cx="60" cy="58" r="8" fill="#f0c060" />
        <!-- 兔耳 -->
        <ellipse cx="55" cy="48" rx="3" ry="8" fill="#f0c060" />
        <ellipse cx="65" cy="48" rx="3" ry="8" fill="#f0c060" />
        <ellipse cx="55" cy="48" rx="1.5" ry="5" fill="#e8a030" />
        <ellipse cx="65" cy="48" rx="1.5" ry="5" fill="#e8a030" />
        <!-- 眼睛 -->
        <circle cx="56" cy="57" r="2" fill="#4a3f45" />
        <circle cx="64" cy="57" r="2" fill="#4a3f45" />
        <circle cx="56.5" cy="56" r="0.7" fill="white" />
        <circle cx="64.5" cy="56" r="0.7" fill="white" />
        <!-- 鼻子 -->
        <ellipse cx="60" cy="60" rx="1.5" ry="1" fill="#e8a030" />
      </svg>

      <!-- 空搜索 -->
      <svg
        v-else-if="variant === 'empty-search'"
        width="120"
        height="120"
        viewBox="0 0 120 120"
        fill="none"
        class="mx-auto"
      >
        <circle cx="60" cy="60" r="55" fill="var(--wb-lavender)" fill-opacity="0.15" />
        <!-- 放大镜 -->
        <circle cx="55" cy="50" r="20" stroke="#cfc4ff" stroke-width="6" fill="white" />
        <line x1="70" y1="65" x2="88" y2="83" stroke="#cfc4ff" stroke-width="6" stroke-linecap="round" />
        <!-- 放大镜内问号 -->
        <text x="55" y="55" text-anchor="middle" font-size="16" fill="#cfc4ff">?</text>
        <!-- 小机器人 -->
        <rect x="78" y="72" width="20" height="18" rx="4" fill="#bfeee0" />
        <rect x="82" y="68" width="12" height="6" rx="2" fill="#8fe3c0" />
        <circle cx="85" cy="76" r="2" fill="#4a3f45" />
        <circle cx="91" cy="76" r="2" fill="#4a3f45" />
        <line x1="88" y1="80" x2="94" y2="80" stroke="#4a3f45" stroke-width="1.5" />
      </svg>

      <!-- 空知识库 -->
      <svg
        v-else-if="variant === 'empty-knowledge'"
        width="120"
        height="120"
        viewBox="0 0 120 120"
        fill="none"
        class="mx-auto"
      >
        <circle cx="60" cy="60" r="55" fill="var(--wb-mint)" fill-opacity="0.15" />
        <!-- 书本 -->
        <rect x="35" y="35" width="50" height="55" rx="4" fill="#8fe3c0" />
        <rect x="38" y="38" width="22" height="49" rx="2" fill="white" />
        <rect x="62" y="38" width="20" height="49" rx="2" fill="white" />
        <!-- 书页线条 -->
        <line x1="42" y1="48" x2="55" y2="48" stroke="#c8e6c9" stroke-width="2" />
        <line x1="42" y1="55" x2="55" y2="55" stroke="#c8e6c9" stroke-width="2" />
        <line x1="42" y1="62" x2="52" y2="62" stroke="#c8e6c9" stroke-width="2" />
        <line x1="66" y1="48" x2="76" y2="48" stroke="#c8e6c9" stroke-width="2" />
        <line x1="66" y1="55" x2="76" y2="55" stroke="#c8e6c9" stroke-width="2" />
        <line x1="66" y1="62" x2="73" y2="62" stroke="#c8e6c9" stroke-width="2" />
        <!-- 书脊 -->
        <line x1="60" y1="35" x2="60" y2="90" stroke="#4caf50" stroke-width="2" />
      </svg>

      <!-- 404 错误 -->
      <svg
        v-else-if="variant === 'error-404'"
        width="120"
        height="120"
        viewBox="0 0 120 120"
        fill="none"
        class="mx-auto"
      >
        <circle cx="60" cy="60" r="55" fill="var(--wb-primary)" fill-opacity="0.08" />
        <!-- 404 文字 -->
        <text x="60" y="55" text-anchor="middle" font-size="28" font-weight="bold" fill="var(--wb-primary)">404</text>
        <!-- 小云朵（表面色，随主题） -->
        <ellipse cx="40" cy="75" rx="15" ry="8" fill="var(--wb-surface)" />
        <ellipse cx="50" cy="72" rx="10" ry="7" fill="var(--wb-surface)" />
        <ellipse cx="80" cy="80" rx="12" ry="6" fill="var(--wb-surface)" />
        <!-- 雨滴 -->
        <line x1="40" y1="85" x2="40" y2="92" stroke="var(--wb-sky)" stroke-width="2" stroke-linecap="round" />
        <line x1="50" y1="88" x2="50" y2="95" stroke="var(--wb-sky)" stroke-width="2" stroke-linecap="round" />
        <line x1="75" y1="88" x2="75" y2="95" stroke="var(--wb-sky)" stroke-width="2" stroke-linecap="round" />
        <line x1="85" y1="85" x2="85" y2="92" stroke="var(--wb-sky)" stroke-width="2" stroke-linecap="round" />
      </svg>

      <!-- 通用错误 -->
      <svg
        v-else-if="variant === 'error-general'"
        width="120"
        height="120"
        viewBox="0 0 120 120"
        fill="none"
        class="mx-auto"
      >
        <!-- 错误语义走 danger token（明暗主题自适应；固定浅红在暗色下会刺眼） -->
        <circle cx="60" cy="60" r="55" fill="var(--wb-danger)" fill-opacity="0.12" />
        <!-- 警告三角 -->
        <polygon points="60,25 95,85 25,85" fill="var(--wb-danger)" fill-opacity="0.55" />
        <polygon points="60,32 88,82 32,82" fill="var(--wb-danger)" fill-opacity="0.25" />
        <!-- 感叹号 -->
        <rect x="57" y="45" width="6" height="22" rx="3" fill="var(--wb-danger)" />
        <circle cx="60" cy="75" r="3" fill="var(--wb-danger)" />
      </svg>

      <!-- 加载中 -->
      <svg
        v-else-if="variant === 'loading'"
        width="120"
        height="120"
        viewBox="0 0 120 120"
        fill="none"
        class="mx-auto"
      >
        <circle cx="60" cy="60" r="55" fill="var(--wb-primary)" fill-opacity="0.08" />
        <!-- 旋转的小花 -->
        <g class="wb-spin" style="transform-origin: 60px 60px">
          <circle cx="60" cy="35" r="8" fill="#ffb7c9" />
          <circle cx="60" cy="85" r="8" fill="#ffb7c9" />
          <circle cx="35" cy="60" r="8" fill="#ffb7c9" />
          <circle cx="85" cy="60" r="8" fill="#ffb7c9" />
          <circle cx="60" cy="60" r="12" fill="var(--wb-primary)" />
          <circle cx="60" cy="60" r="6" fill="white" />
        </g>
      </svg>
    </div>

    <!-- 标题和副标题 -->
    <h3 :class="['font-medium text-wb-ink', illustration ? 'mt-4' : '', size === 'sm' ? 'text-sm' : 'text-base']">
      {{ displayTitle }}
    </h3>
    <p v-if="displaySubtitle" class="mt-1 text-xs text-wb-muted sm:text-sm">{{ displaySubtitle }}</p>

    <!-- 自定义内容插槽 -->
    <div class="mt-4">
      <slot />
    </div>
  </div>
</template>

<style scoped>
/* 空态留白随尺寸收敛：md 保留呼吸感，sm 用于侧栏与表格内，不制造大面积空白 */
.wb-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
  padding: var(--wb-sp-10) var(--wb-sp-4);
}
.wb-empty.is-sm {
  padding: var(--wb-sp-5) var(--wb-sp-3);
}

/* 紧凑尺寸：插画缩到 64px（CSS 覆盖 SVG 的 width/height 属性） */
.wb-empty-sm svg {
  width: 64px;
  height: 64px;
}

.wb-spin {
  animation: spin 2s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
