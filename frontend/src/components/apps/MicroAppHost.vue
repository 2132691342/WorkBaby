<script setup lang="ts">
// 微前端容器：一个应用一个挂载点，只有激活时才加载 qiankun 与外部应用代码。
// 容器随选中项切换而重建：qiankun 不允许同一个容器同时挂两个应用。
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppsStore, type MicroApp } from '../../stores/apps'
import AppIcon from '../common/AppIcon.vue'
import { errorText } from '../../utils/errors'

const props = defineProps<{
  /** 要挂载的应用；为空时空态 */
  app: MicroApp | null
}>()

const emit = defineEmits<{
  /** 挂载状态变化：父组件据此更新列表上的指示灯 */
  state: [name: string, mounted: boolean]
}>()

const { t } = useI18n()
const apps = useAppsStore()

const host = ref<HTMLDivElement | null>(null)
/** qiankun 的 MicroApp 句柄：卸载时必须走它自己的 unmount（返回 null 也是成功） */
let handle: {
  unmount: () => Promise<unknown>
  mount: () => Promise<unknown>
} | null = null

/** 容器选择器：每个应用一个，避免与页面里其它 DOM 撞名。 */
function containerOf(name: string): string {
  return `#micro-slot-${name.replace(/[^\w-]/g, '_')}`
}

async function mount() {
  const app = props.app
  if (!app || !host.value) return
  apps.setState(app.app_name, { loading: true, error: '' })
  try {
    // qiankun 体积不小且只在微前端页用到：动态引入，别进首屏包
    const { loadMicroApp } = await import('qiankun')
    handle = loadMicroApp(
      {
        name: app.app_name,
        entry: app.app_entry,
        container: app.app_container || containerOf(app.app_name),
      },
      { sandbox: { experimentalStyleIsolation: true } },
      {
        // 生命周期回调必须是 async：qiankun 按 Promise 等待，返回 void 会被判成类型不符
        beforeLoad: async () => {
          apps.setState(app.app_name, { loading: true })
        },
        afterMount: async () => {
          apps.setState(app.app_name, { loading: false, mounted: true })
          emit('state', app.app_name, true)
        },
        afterUnmount: async () => {
          apps.setState(app.app_name, { mounted: false })
          emit('state', app.app_name, false)
        },
      },
    )
    await handle?.mount()
  } catch (e) {
    // 外部应用的错不该把整页拖死：标出是哪一步失败
    apps.setState(app.app_name, {
      loading: false,
      mounted: false,
      error: errorText(e) || String(e),
    })
    emit('state', app.app_name, false)
  }
}

async function unmount() {
  const app = props.app
  if (!handle) return
  try {
    await handle.unmount()
  } catch {
    // 已卸载或从未挂上：忽略，容器马上要重建
  }
  handle = null
  if (app) apps.setState(app.app_name, { mounted: false, loading: false })
}

onMounted(mount)
watch(
  () => props.app?.app_name,
  async () => {
    await unmount()
    await mount()
  },
)
onBeforeUnmount(() => {
  void unmount()
})

const state = () => (props.app ? apps.stateOf(props.app.app_name) : null)
</script>

<template>
  <div v-if="!app" class="me-empty">
    <span class="wb-tile wb-tile--lg"><AppIcon name="zap" size="ic-xl" /></span>
    <b>{{ t('apps.noSelection') }}</b>
    <p>{{ t('apps.noSelectionHint') }}</p>
  </div>

  <div v-else class="me-wrap">
    <div class="me-bar">
      <span class="me-name">{{ app.app_name }}</span>
      <span class="me-entry" :title="app.app_entry">{{ app.app_entry }}</span>
      <span class="me-sp" />
      <span v-if="state()?.loading" class="me-tag is-load">{{ t('apps.loading') }}</span>
      <span v-else-if="state()?.error" class="me-tag is-bad">{{ t('apps.loadFailed') }}</span>
      <span v-else-if="state()?.mounted" class="me-tag is-ok">{{ t('apps.mounted') }}</span>
    </div>

    <div v-if="state()?.error" class="me-error">
      <AppIcon name="alert" size="ic-sm" />
      <p>{{ state()?.error }}</p>
    </div>

    <div ref="host" class="me-host">
      <!-- qiankun 会把应用挂进这个容器：容器必须真实存在，不能用 v-if 跳过 -->
      <div :id="containerOf(app.app_name).slice(1)" class="me-slot" />
    </div>
  </div>
</template>

<style scoped>
.me-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-12) var(--wb-sp-6);
  text-align: center;
}
.me-empty b {
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}
.me-empty p {
  max-width: calc(440px * var(--wb-vw-scale));
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
  line-height: var(--wb-lh-base);
}
.me-wrap {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}
.me-bar {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-bottom: 1px solid var(--wb-border);
  flex: none;
}
.me-name {
  font-size: var(--wb-fs-sm);
  font-weight: 700;
  color: var(--wb-ink);
  flex: none;
}
.me-entry {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 50%;
}
.me-sp {
  flex: 1;
}
.me-tag {
  padding: 1px var(--wb-sp-2);
  border-radius: var(--wb-radius-full);
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  flex: none;
}
.me-tag.is-load {
  background: var(--wb-live-soft);
  color: var(--wb-live);
}
.me-tag.is-ok {
  background: var(--wb-success-soft);
  color: var(--wb-success);
}
.me-tag.is-bad {
  background: var(--wb-danger-soft);
  color: var(--wb-danger);
}
.me-error {
  display: flex;
  align-items: flex-start;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-3);
  border-bottom: 1px solid var(--wb-border);
  background: var(--wb-danger-soft);
  color: var(--wb-danger);
  font-size: var(--wb-fs-sm);
  flex: none;
}
.me-error p {
  margin: 0;
  line-height: var(--wb-lh-base);
  overflow-wrap: anywhere;
}
.me-host {
  flex: 1;
  min-height: 0;
  position: relative;
  overflow: auto;
}
.me-slot {
  width: 100%;
  min-height: 100%;
}
</style>
