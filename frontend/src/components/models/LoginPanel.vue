<script setup lang="ts">
// 服务商登录：哪些已登录、哪些可用、点一下发起登录。
// 只有支持界面登录的引擎才渲染（pi 的登录在终端里做），判据在父组件按 Caps.login。
// 默认只展开「已登录 + 可登录」，其余收进「展开全部」——八十行同名行没法找。
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import type { LoginProvider } from '@/types/api'

const props = defineProps<{
  logins: LoginProvider[]
  /** 正在登录的服务商 id（空表示没有） */
  loggingIn: string
  /** 已登录的数量，显示在标题上 */
  authenticated: number
}>()

const emit = defineEmits<{ login: [provider: LoginProvider] }>()

const { t } = useI18n()

const expanded = ref(false)
const filter = ref('')

/** 折叠时最多显示几条可登录的：已登录的也全算在内。 */
const COLLAPSED = 6

/**
 * 折叠时显示的条目：已登录的全留，可登录的只留前几条。
 *
 * 引擎会报回八九十条可登录服务商。全铺出来会把下面的「可用模型」
 * 推到几屏之外，而这一节最常见的用途是「我登过谁」。
 */
const visible = computed(() => {
  const q = filter.value.trim().toLowerCase()
  if (q) {
    return props.logins.filter(
      (x) => x.name.toLowerCase().includes(q) || x.id.toLowerCase().includes(q),
    )
  }
  // 已登录的排前面：这一节最常见的用途是「我登过谁」。
  const sorted = [...props.logins].sort((a, b) => Number(b.authenticated) - Number(a.authenticated))
  if (expanded.value) return sorted
  const authed = sorted.filter((x) => x.authenticated)
  const others = sorted.filter((x) => !x.authenticated)
  return [...authed, ...others.slice(0, Math.max(0, COLLAPSED - authed.length))]
})

/** 折叠时被藏起来的条数。 */
const hidden = computed(() => {
  if (expanded.value || filter.value.trim()) return 0
  return props.logins.length - visible.value.length
})

function tone(item: LoginProvider): string {
  if (item.authenticated) return 'ok'
  return item.available ? '' : 'warn'
}

function label(item: LoginProvider): string {
  if (item.authenticated) return t('models.loggedIn')
  return item.available ? t('models.notLoggedIn') : t('models.unavailable')
}
</script>

<template>
  <section>
    <div class="wb-sec-head">
      <span class="wb-sec-title">
        {{ t('models.login') }}
        <span class="count">{{ props.authenticated }}/{{ props.logins.length }}</span>
      </span>
    </div>
    <p class="hint">{{ t('models.loginHint') }}</p>

    <div v-if="props.logins.length" class="login-tools">
      <div class="login-filter-row">
        <AppIcon name="search" :size="12" class="login-filter-ic" />
        <input
          v-model="filter"
          class="login-filter"
          :placeholder="t('models.loginFilter')"
          spellcheck="false"
        />
        <button v-if="filter" class="login-filter-clear" :title="t('common.clear')" @click="filter = ''">
          <AppIcon name="close" :size="11" />
        </button>
      </div>
      <button v-if="hidden || expanded" class="wb-btn text sm" @click="expanded = !expanded">
        {{ expanded ? t('models.loginCollapse') : t('models.loginExpand', { n: hidden }) }}
      </button>
    </div>

    <div v-if="visible.length" class="login-grid">
      <div v-for="item in visible" :key="item.id" class="login-row">
        <span class="login-dot" :class="item.authenticated ? 'is-ok' : item.available ? '' : 'is-dim'" />
        <div class="login-main">
          <span class="login-name">{{ item.name }}</span>
          <span class="login-id mono">{{ item.id }}</span>
        </div>
        <span class="tag" :class="tone(item)">{{ label(item) }}</span>
        <button
          class="wb-btn sm"
          :class="{ 'is-loading': props.loggingIn === item.id }"
          :disabled="!item.available || Boolean(props.loggingIn)"
          @click="emit('login', item)"
        >
          {{ item.authenticated ? t('models.relogin') : t('models.login') }}
        </button>
      </div>
    </div>
    <p v-else-if="filter" class="hint">{{ t('models.loginNoMatch') }}</p>
    <p v-else class="hint">{{ t('chat.engineDown') }}</p>
  </section>
</template>

<style scoped>
.login-tools {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  margin-bottom: var(--wb-sp-2);
}

.login-filter-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-1);
  flex: 1;
  min-width: 0;
  max-width: 320px;
  height: var(--wb-ctl-sm);
  padding: 0 var(--wb-sp-2);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-sm);
  background: var(--wb-surface);
  transition: border-color var(--wb-dur-fast) var(--wb-ease);
}

.login-filter-row:focus-within {
  border-color: var(--wb-brand-2);
}

.login-filter-ic {
  flex: none;
  color: var(--wb-faint);
}

.login-filter {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: none;
  background: transparent;
  color: var(--wb-ink);
  font-family: inherit;
  font-size: var(--wb-fs-2xs);
}

.login-filter::placeholder {
  color: var(--wb-faint);
}

.login-filter-clear {
  display: grid;
  place-items: center;
  width: 16px;
  height: 16px;
  flex: none;
  border: 0;
  border-radius: var(--wb-r-xs);
  background: transparent;
  color: var(--wb-faint);
}

.login-filter-clear:hover {
  background: var(--wb-hover);
  color: var(--wb-ink);
}

.login-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: var(--wb-sp-1) var(--wb-sp-3);
}

.login-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-1) var(--wb-sp-2);
  border-radius: var(--wb-r-sm);
  transition: background-color var(--wb-dur-fast) var(--wb-ease);
}

.login-row:hover {
  background: var(--wb-hover);
}

/* 状态灯：已登录实心绿、可登录空心中性、不可用淡灰 */
.login-dot {
  width: 6px;
  height: 6px;
  flex: none;
  border-radius: 50%;
  border: 1px solid var(--wb-line-strong);
  background: transparent;
}

.login-dot.is-ok {
  border-color: var(--wb-ok);
  background: var(--wb-ok);
}

.login-dot.is-dim {
  border-color: var(--wb-line);
}

.login-main {
  display: flex;
  flex-direction: column;
  gap: 0;
  flex: 1;
  min-width: 0;
}

.login-name {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.login-id {
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

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
