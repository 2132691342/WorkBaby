<script setup lang="ts">
// 输入区里的两个会话开关：默认模型 + 动手前规矩（原先挂在会话头右侧）。
import { computed, ref } from 'vue'
import * as api from '../../api'
import { useSessionStore } from '../../stores/session'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import { fmtCount } from '../../utils/num'
import AppIcon from '../common/AppIcon.vue'

// 后端 /sessions/:id/permission 只认这三个值，别再引入第四种叫法。
const PERMISSIONS = [
  { key: 'ask', name: '每次问我', desc: '改文件、跑命令前都先问一句，最稳妥' },
  { key: 'auto_edit', name: '自动改文件', desc: '改文件不再打断，跑命令与跑脚本仍会问' },
  { key: 'yolo', name: '全自动', desc: '什么都不问，直接做（只建议在自己电脑上用）' },
] as const

const session = useSessionStore()
const settings = useSettingsStore()
const toast = useToastStore()

const open = ref<'model' | 'perm' | null>(null)
const models = ref<string[]>([])
const providerId = ref('')
// 会话绑定的服务商被删掉时会回退到别的服务，必须显式告知：
// 不声不响换服务商，用户以为换的只是模型，实际换了一个账号在计费。
const providerName = ref('')
const providerMissing = ref(false)
const modelLoading = ref(false)
// 每个模型的能力标签：窗口多大、能不能识图。选模型时一眼看到差别。
const caps = ref<Record<string, { win: string; vision: boolean; tools: boolean }>>({})
// 过滤与截断：上游可能有几百个模型，全量渲染既卡也不好用
const filter = ref('')
const MAX_SHOWN = 30

const filtered = computed(() => {
  const q = filter.value.trim().toLowerCase()
  const hit = q ? models.value.filter((m) => m.toLowerCase().includes(q)) : models.value
  return hit.slice(0, MAX_SHOWN)
})
const hiddenCount = computed(() => {
  const q = filter.value.trim().toLowerCase()
  const hit = q ? models.value.filter((m) => m.toLowerCase().includes(q)) : models.value
  return Math.max(0, hit.length - MAX_SHOWN)
})

const modelName = computed(() => session.current?.model || settings.boot?.default_model || '默认模型')
const permName = computed(() => PERMISSIONS.find((p) => p.key === session.current?.permission)?.name || PERMISSIONS[0].name)

// 模型名从已配置的服务商里选，不让用户背模型 ID
async function loadModels() {
  modelLoading.value = true
  try {
    const list = await api.providers.list()
    const own = list.find((x) => x.id === session.current?.provider_id)
    // 会话自己的服务商优先；被删掉时才退回默认服务，并在下拉里说明这件事。
    const p = own || list.find((x) => x.is_default) || list[0]
    providerMissing.value = !own && !!p
    providerName.value = p?.name || ''
    providerId.value = p?.id || ''
    models.value = p ? [...p.models] : []
    if (models.value.length && providerId.value) {
      // 批量取一次：下拉一次列几十上百个模型，逐个请求是纯浪费。
      try {
        for (const c of await api.models.capabilities(providerId.value, models.value)) {
          caps.value[c.id] = { win: fmtCount(c.context_window), vision: c.vision, tools: c.tool_call }
        }
      } catch {
        // 能力标签拿不到就不显示，不阻塞下拉打开
      }
    }
  } finally {
    modelLoading.value = false
  }
}

function show(kind: 'model' | 'perm') {
  open.value = kind
  filter.value = ''
  if (kind === 'model' && !models.value.length) void loadModels()
}

function toggle(kind: 'model' | 'perm') {
  if (open.value === kind) open.value = null
  else show(kind)
}

// applying 标记正在应用的那一行：切换要等「改会话 + 刷新列表」两次请求，
// 不挂反馈用户会连点，最终状态取决于返回顺序。
const applying = ref('')
async function chooseModel(model: string) {
  if (!providerId.value || applying.value) return
  applying.value = `m:${model}`
  try {
    await session.setModel(providerId.value, model)
    open.value = null
    toast.ok(`本对话已切换到 ${model}`)
  } catch (e) {
    toast.bad(`切换模型失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    applying.value = ''
  }
}

async function choosePermission(key: string) {
  if (applying.value) return
  applying.value = `p:${key}`
  try {
    await session.setPermission(key)
    open.value = null
    const name = PERMISSIONS.find((p) => p.key === key)?.name || key
    toast.ok(`动手前规矩已改为「${name}」，下一轮生效`)
  } catch (e) {
    toast.bad(`设置失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    applying.value = ''
  }
}

defineExpose({ show, close: () => (open.value = null) })
</script>

<template>
  <div class="chips" @pointerdown.stop>
    <button
      class="chip"
      type="button"
      title="换一个模型"
      :class="{ on: open === 'model' }"
      @click="toggle('model')"
    >
      <AppIcon name="cpu" size="ic-xs" />
      <span class="chip-t">{{ modelName }}</span>
    </button>
    <button
      class="chip"
      type="button"
      title="助手动手前要不要先问你一句"
      :class="{ on: open === 'perm' }"
      @click="toggle('perm')"
    >
      <AppIcon name="shield" size="ic-xs" />
      <span class="chip-t">{{ permName }}</span>
    </button>

    <div v-if="open" class="pop chips-pop">
      <template v-if="open === 'model'">
        <div class="menu-label">用哪个模型{{ providerName ? `（来自「${providerName}」）` : '' }}</div>
        <div v-if="modelLoading" class="pop-row muted">正在读取模型列表…</div>
        <div v-else-if="!models.length" class="pop-row muted">还没有配置模型服务</div>
        <template v-else>
          <div v-if="providerMissing" class="pop-row is-warn">
            本对话原来的服务商已被删除，下面是其他服务的模型；选一个即切换过去
          </div>
          <input
            v-model="filter"
            class="input m-filter"
            placeholder="输入名称过滤…"
            autofocus
          />
          <button
            v-for="m in filtered"
            :key="m"
            class="pop-row"
            :class="{ 'is-hi': m === session.current?.model, 'is-loading': applying === `m:${m}` }"
            type="button"
            :disabled="!!applying"
            @click="chooseModel(m)"
          >
            <AppIcon v-if="m === session.current?.model" name="check" size="ic-xs" />
            <span v-else class="pr-gap" />
            <span class="m-name">{{ m }}</span>
            <span v-if="caps[m]" class="m-caps">
              <span class="m-win">{{ caps[m].win }}</span>
              <span v-if="caps[m].vision" class="m-tag">识图</span>
              <span v-if="!caps[m].tools" class="m-tag is-off">无工具</span>
            </span>
          </button>
          <div v-if="hiddenCount > 0" class="pop-row muted">
            还有 {{ hiddenCount }} 个模型，输入名称继续过滤
          </div>
          <div v-else-if="!filtered.length" class="pop-row muted">没有匹配的模型</div>
        </template>
      </template>

      <template v-else>
        <div class="menu-label">助手动手前的规矩</div>
        <button
          v-for="p in PERMISSIONS"
          :key="p.key"
          class="pop-row"
          :class="{ 'is-hi': p.key === session.current?.permission, 'is-loading': applying === `p:${p.key}` }"
          type="button"
          :disabled="!!applying"
          @click="choosePermission(p.key)"
        >
          <span class="grow">
            <b class="pr-t">{{ p.name }}</b>
            <span class="pr-d">{{ p.desc }}</span>
          </span>
        </button>
      </template>
    </div>
  </div>
</template>

<style scoped>
.chips {
  position: relative;
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  min-width: 0;
}
.chip-t {
  min-width: 0;
  max-width: 150px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.chips-pop {
  left: 0;
  bottom: calc(100% + 6px);
  min-width: 264px;
  max-width: 330px;
  max-height: 320px;
  overflow-y: auto;
}
.pr-gap {
  width: 13px;
  flex: none;
}
.grow {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.pop-row.is-warn {
  text-align: left;
  font-size: var(--wb-fs-hint);
  color: var(--wb-warning);
  cursor: default;
}
.pop-row .pr-t {
  font-family: var(--font-sans);
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  color: var(--wb-ink);
}
.pop-row .pr-d {
  display: block;
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.m-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.m-filter {
  height: var(--wb-ctl-h-sm);
  margin-bottom: 4px;
  font-size: var(--wb-fs-sm);
}
.m-caps {
  margin-left: auto;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex: none;
}
.m-win {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
}
.m-tag {
  padding: 0 5px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
  font-size: var(--wb-fs-hint);
}
.m-tag.is-off {
  background: var(--wb-tint);
  color: var(--wb-muted);
}
</style>
