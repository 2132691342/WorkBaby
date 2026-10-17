<script setup lang="ts">
// 输入区的两枚会话开关：用哪个模型、动手前问不问。
// 模型面板是两级级联（厂商 / 该厂商模型），模型数超过 24 才浮现「只看最常用的」过滤。
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as api from '../../api'
import { useSessionStore } from '../../stores/session'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import { fmtCount } from '../../utils/num'
import type { ProviderVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'

// 后端只认这三个值（specs/19），别再引入第四种叫法。
const PERMISSIONS = [
  { key: 'ask_write', name: '先问我', desc: '写文件、跑命令前都先问一句，最稳妥' },
  { key: 'auto', name: '全自动', desc: '什么都不问，直接做（只建议在自己电脑上用）' },
  { key: 'readonly', name: '只读', desc: '只看不改：写文件与跑命令一律拒绕' },
] as const

const session = useSessionStore()
const settings = useSettingsStore()
const toast = useToastStore()

const open = ref<'model' | 'perm' | null>(null)
const providers = ref<ProviderVO[]>([])
const providerLoading = ref(false)
// activeVendor 是右列正在展示哪个厂商的模型；默认落在会话自己的厂商上。
const activeVendor = ref('')
// 会话绑定的服务商被删掉时给一句提示：不声不响换服务商，用户以为换的只是模型，
// 实际换了一个账号在计费。
const providerMissing = computed(
  () => !!session.current?.provider_id && !providers.value.some((p) => p.id === session.current?.provider_id),
)
// 模型数多了才在右列顶部露一条「关键字过滤」；少于或等于这个数直接给全列表
const FILTER_THRESHOLD = 24
const filter = ref('')
const filterInputRef = ref<HTMLInputElement | null>(null)
// 当前面板实例对应的 chat:user 时刻会话 id：会话切换后旧面板关掉、避免给已断开的会话改模型
const ownerSessionId = ref('')

// 每个厂商的能力标签按厂商缓存，切厂商不重复请求。
const caps = ref<Record<string, Record<string, { win: string; vision: boolean; tools: boolean }>>>({})

// chip 上显示的模型：后端 model 字段可能是「厂商前缀/模型名」（多厂商聚合网关的写法），
// 直接整串显示会把一个 chip 撑成两倍宽。斜杠只留后一段，前缀归到左列的服务商列表。
const modelName = computed(() => {
  const raw = session.current?.model || settings.boot?.default_model || ''
  const slash = raw.lastIndexOf('/')
  return slash >= 0 ? raw.slice(slash + 1) : raw
})
const vendorName = computed(() => {
  const pid = session.current?.provider_id || ''
  const p = providers.value.find((x) => x.id === pid)
  return p?.name || ''
})
const permName = computed(() => PERMISSIONS.find((p) => p.key === session.current?.permission)?.name || PERMISSIONS[0].name)

// 左列厂商按「当前会话厂商排最前」排序，其余保持后端返回顺序（默认服务在前）。
const vendorList = computed(() => {
  const cur = session.current?.provider_id || ''
  return [...providers.value].sort((a, b) => {
    const aCur = a.id === cur ? 1 : 0
    const bCur = b.id === cur ? 1 : 0
    if (aCur !== bCur) return bCur - aCur
    const aDef = a.is_default ? 1 : 0
    const bDef = b.is_default ? 1 : 0
    return bDef - aDef
  })
})
const activeProvider = computed(() => providers.value.find((p) => p.id === activeVendor.value) || null)
const vendorHasSelected = (id: string) =>
  !!session.current && session.current.provider_id === id && !!session.current.model

// 右列只展示已启用的服务商下的模型；空模型给「在设置里同步一下」指引
const showFilterBar = computed(() => (activeProvider.value?.models.length || 0) > FILTER_THRESHOLD)

const shownModels = computed(() => {
  const list = activeProvider.value?.models || []
  const q = filter.value.trim().toLowerCase()
  if (!q) return list
  return list.filter((m) => m.toLowerCase().includes(q))
})

const loadCap = async (pid: string, models: string[]) => {
  if (!pid || caps.value[pid] || !models.length) return
  caps.value[pid] = {} // 先占位，避免同一厂商被 hover/点击反复触发批量查询
  try {
    for (const c of await api.models.capabilities(pid, models)) {
      caps.value[pid][c.id] = { win: fmtCount(c.context_window), vision: c.vision, tools: c.tool_call }
    }
  } catch {
    delete caps.value[pid] // 拿不到就清掉占位，下次进来还能重试
  }
}

async function loadProviders() {
  providerLoading.value = true
  try {
    providers.value = await api.providers.list()
    // 每次打开都重拉后，落在「会话自己的厂商」上；会话厂商已删就落到第一个可用厂商。
    const cur = session.current?.provider_id || ''
    const pick = providers.value.find((p) => p.id === cur) || providers.value.find((p) => p.is_default) || providers.value[0]
    activeVendor.value = pick?.id || ''
    if (pick) void loadCap(pick.id, pick.models)
  } finally {
    providerLoading.value = false
  }
}

function focusVendor(id: string) {
  if (activeVendor.value === id) return
  activeVendor.value = id
  filter.value = ''
  const p = providers.value.find((x) => x.id === id)
  if (p) void loadCap(p.id, p.models)
}

function show(kind: 'model' | 'perm') {
  open.value = kind
  filter.value = ''
  ownerSessionId.value = session.currentId || ''
  // 每次打开都重拉：设置页刚加的厂商 / 模型要立刻能选上。
  if (kind === 'model') void loadProviders()
}

function toggle(kind: 'model' | 'perm') {
  if (open.value === kind) open.value = null
  else show(kind)
}

function close() {
  open.value = null
  filter.value = ''
}

// 关闭逻辑：点击面板外、Esc、切换会话三种情况都要能正常收口
function onDocPointerDown(e: PointerEvent) {
  const root = rootEl.value
  if (!root || root.contains(e.target as Node)) return
  if (open.value) close()
}
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && open.value) {
    e.stopPropagation()
    close()
  }
}

// applying 标记正在应用的那一行：切换要等「改会话 + 刷新列表」两次请求，
// 不挂反馈用户会连点，最终状态取决于返回顺序。
const applying = ref('')
async function chooseModel(providerId: string, model: string) {
  if (!providerId || applying.value) return
  // 切换期间会话被换走，不要再把它发出去：旧会话的「已选」标记会跟着错位。
  if (ownerSessionId.value && session.currentId !== ownerSessionId.value) {
    close()
    return
  }
  applying.value = `${providerId}:${model}`
  try {
    await session.setModel(providerId, model)
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

const rootEl = ref<HTMLElement | null>(null)

// 打开模型面板后给右列首屏一个微微的入场错峰，hover 厂商切换时不再叠
const key = ref(0)
watch(open, (v) => {
  if (v) key.value++
  // 切到模型面板时如需要过滤，下一拍自动 focus 过滤框
  if (v === 'model') {
    nextTick(() => {
      if (showFilterBar.value) filterInputRef.value?.focus()
    })
  }
})

// 会话被切走时同步关掉面板：旧面板还开着会让人误以为给新会话也改了
watch(() => session.currentId, (id) => {
  if (ownerSessionId.value && ownerSessionId.value !== id && open.value) close()
})

onMounted(() => {
  document.addEventListener('pointerdown', onDocPointerDown)
  document.addEventListener('keydown', onKey)
})
onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onDocPointerDown)
  document.removeEventListener('keydown', onKey)
})

defineExpose({ show, close })
</script>

<template>
  <div ref="rootEl" class="chips" @pointerdown.stop>
    <button
      class="chip"
      type="button"
      :title="'换一个模型（所有服务商都列在这里）'"
      :class="{ on: open === 'model' }"
      @click="toggle('model')"
    >
      <AppIcon name="cpu" size="ic-xs" />
      <span v-if="vendorName" class="chip-vendor">{{ vendorName }}</span>
      <span class="chip-t">{{ modelName || '默认模型' }}</span>
      <AppIcon name="chevron-up" size="ic-xs" class="chip-chev" />
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
      <AppIcon name="chevron-up" size="ic-xs" class="chip-chev" />
    </button>

    <!-- 进入动画：内容用 key 强制重挂触发淡入；左列用 step-delay 错峰 -->
    <Transition name="pop">
      <div v-if="open === 'model'" :key="key" class="pop chips-pop no-pop-anim" @pointerdown.stop>
        <div class="menu-label">
          <AppIcon name="cpu" size="ic-xs" />
          <span>用哪个模型</span>
          <span class="menu-tip">左选服务商，右选模型</span>
        </div>
        <div v-if="providerLoading" class="pop-row muted pop-loading">
          <span class="spinner" />
          正在读取服务商列表…
        </div>
        <div v-else-if="!providers.length" class="pop-row muted">
          还没有配置模型服务，到「设置 → 模型服务」添加一个
        </div>
        <template v-else>
          <div v-if="providerMissing" class="pop-row is-warn">
            <AppIcon name="alert-triangle" size="ic-xs" />
            本对话原来的服务商已被删除，从下面任意一个服务商重新选一个即可切换过去
          </div>
          <div class="cascade">
            <!-- 左列：全部服务商，hover 立即切到右列；当前会话所在服务商打点 -->
            <div class="col-vendor" role="listbox" aria-label="服务商">
              <button
                v-for="(p, i) in vendorList"
                :key="p.id"
                class="v-row"
                :class="{ 'is-cur': p.id === activeVendor, 'is-sel': vendorHasSelected(p.id), 'is-disabled': !p.enabled }"
                :style="{ animationDelay: `${Math.min(i * 18, 140)}ms` }"
                type="button"
                role="option"
                :aria-selected="p.id === activeVendor"
                :title="p.enabled ? p.base_url : '已停用'"
                @mouseenter="focusVendor(p.id)"
                @focus="focusVendor(p.id)"
                @click="focusVendor(p.id)"
              >
                <span class="v-glyph" :class="{ 'is-sel': vendorHasSelected(p.id) }">
                  <span v-if="vendorHasSelected(p.id)" class="v-tick">
                    <AppIcon name="check" size="ic-2xs" />
                  </span>
                  <span v-else class="v-initial">{{ (p.name || '?').slice(0, 1) }}</span>
                </span>
                <span class="v-meta">
                  <span class="v-name">{{ p.name }}</span>
                  <span class="v-sub">{{ p.models.length }} 个模型</span>
                </span>
                <span v-if="p.is_default" class="v-tag">默认</span>
                <AppIcon name="chevron-right" size="ic-xs" class="v-chev" />
              </button>
            </div>
            <!-- 右列：当前服务商的模型；多于阈值才显示过滤 -->
            <div class="col-model">
              <div class="m-head">
                <span class="m-vendor">
                  <span class="m-vendor-dot" :style="{ background: vendorHasSelected(activeVendor) ? 'var(--wb-success)' : 'var(--wb-primary)' }" />
                  {{ activeProvider?.name || '服务商' }}
                </span>
                <div v-if="showFilterBar" class="m-filter">
                  <AppIcon name="search" size="ic-xs" class="m-filter-ic" />
                  <input
                    ref="filterInputRef"
                    v-model="filter"
                    class="m-filter-in"
                    placeholder="过滤模型"
                    spellcheck="false"
                  />
                  <button v-if="filter" class="m-filter-clr" type="button" aria-label="清空" @click="filter = ''">
                    <AppIcon name="close" size="ic-2xs" />
                  </button>
                </div>
              </div>
              <div class="m-list" role="listbox" aria-label="模型">
                <div v-if="!activeProvider" class="pop-row muted">选择左侧一个服务商</div>
                <div v-else-if="!activeProvider.models.length" class="pop-row muted">
                  这个服务商还没有模型，到「设置 → 模型服务」点「同步模型」拉一下
                </div>
                <div v-else-if="!shownModels.length" class="pop-row muted">没有匹配的模型</div>
                <template v-else>
                  <button
                    v-for="m in shownModels"
                    :key="m"
                    class="m-row"
                    :class="{
                      'is-hi': m === session.current?.model && activeVendor === session.current?.provider_id,
                      'is-loading': applying === `${activeVendor}:${m}`,
                    }"
                    type="button"
                    role="option"
                    :aria-selected="m === session.current?.model && activeVendor === session.current?.provider_id"
                    :disabled="!!applying"
                    @click="chooseModel(activeVendor, m)"
                  >
                    <span class="m-glyph">
                      <AppIcon v-if="m === session.current?.model && activeVendor === session.current?.provider_id" name="check" size="ic-2xs" />
                    </span>
                    <span class="m-name">{{ m }}</span>
                    <span v-if="caps[activeVendor] && caps[activeVendor][m]" class="m-caps">
                      <span class="m-win">{{ caps[activeVendor][m].win }}</span>
                      <span v-if="caps[activeVendor][m].vision" class="m-tag tag-vision">识图</span>
                      <span v-if="!caps[activeVendor][m].tools" class="m-tag tag-off">无工具</span>
                    </span>
                    <span v-else-if="applying === `${activeVendor}:${m}`" class="m-loading">
                      <span class="spinner spinner-sm" />
                    </span>
                  </button>
                </template>
              </div>
            </div>
          </div>
        </template>
      </div>
    </Transition>

    <Transition name="pop">
      <div v-if="open === 'perm'" :key="`perm-${key}`" class="pop chips-pop chips-pop-perm no-pop-anim" @pointerdown.stop>
        <div class="menu-label">
          <AppIcon name="shield" size="ic-xs" />
          <span>助手动手前的规矩</span>
        </div>
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
          <span v-if="applying === `p:${p.key}`" class="m-loading">
            <span class="spinner spinner-sm" />
          </span>
          <AppIcon v-else-if="p.key === session.current?.permission" name="check" size="ic-xs" class="pr-check" />
        </button>
      </div>
    </Transition>
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
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
/* 厂商名与模型名分层：厂商压成前缀小字，模型名才是 chip 的主角。
   后端 model 字段带「厂商/模型」斜杠前缀时，前半段在这里出现一次就够 */
.chip-vendor {
  max-width: 72px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--wb-fs-2xs);
  font-weight: 500;
  color: var(--wb-muted);
  letter-spacing: 0.02em;
  flex: none;
}
.chip-vendor::after {
  content: '/';
  margin-left: 3px;
  opacity: 0.5;
}
.chip-chev {
  margin-left: 2px;
  opacity: 0.55;
  transition: transform var(--wb-dur) var(--wb-ease), opacity var(--wb-dur) var(--wb-ease);
}
.chip.on .chip-chev {
  transform: rotate(180deg);
  opacity: 1;
  color: var(--wb-primary);
}

/* 弹层基础（与 wb-ui.css 中的 .pop 衔接，仅在 chips 内补差异） */
.chips-pop {
  left: 0;
  bottom: calc(100% + 8px);
  min-width: 280px;
  max-width: 360px;
  max-height: 380px;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  padding: 0;
  transform-origin: bottom left;
}
.chips-pop-perm {
  min-width: 280px;
}
/* 模型级联面板：宽一档，左右两栏各管一段，内部滚动而非整块滚 */
.chips-pop:not(.chips-pop-perm) {
  width: min(560px, 92vw);
  min-width: 0;
  max-width: none;
}

/* 弹层进入 / 退出：上移 + 淡入 + 轻微缩放 */
.pop-enter-active,
.pop-leave-active {
  transition:
    transform var(--wb-dur) var(--wb-ease),
    opacity var(--wb-dur-fast) var(--wb-ease);
}
.pop-enter-from,
.pop-leave-to {
  transform: translateY(6px) scale(0.98);
  opacity: 0;
}

.menu-label {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 10px 12px 8px;
  font-size: var(--wb-fs-xs);
  font-weight: 700;
  color: var(--wb-muted);
  text-transform: uppercase;
  letter-spacing: var(--wb-ls-plate);
  border-bottom: 1px solid var(--wb-line);
  flex: none;
}
.menu-label .menu-tip {
  margin-left: auto;
  font-weight: 500;
  text-transform: none;
  letter-spacing: 0;
  color: var(--wb-muted);
  font-size: var(--wb-fs-hint);
}
.pop-loading {
  display: flex;
  align-items: center;
  gap: 8px;
}

.cascade {
  display: flex;
  min-height: 0;
  flex: 1;
  gap: 0;
}

/* === 左列：服务商 === */
.col-vendor {
  width: 188px;
  flex: none;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 6px 6px 8px;
  border-right: 1px solid var(--wb-line);
  overflow-y: auto;
  background: var(--wb-surface-2);
  border-top-left-radius: var(--wb-radius);
  border-bottom-left-radius: var(--wb-radius);
}
.v-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  border: 0;
  background: transparent;
  padding: 7px 8px;
  border-radius: var(--wb-radius-sm);
  cursor: pointer;
  text-align: left;
  color: var(--wb-ink-2);
  position: relative;
  /* 不缩放：竖列满宽放大会把右侧模型列挤动；点击感靠底色 + 1px 描边 */
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
  animation: vendor-in var(--wb-dur) var(--wb-ease) backwards;
}
@keyframes vendor-in {
  from {
    opacity: 0;
    transform: translateX(-4px);
  }
  to {
    opacity: 1;
    transform: none;
  }
}
.v-row:hover:not(:disabled) {
  background: var(--wb-tint);
  color: var(--wb-ink);
}
.v-row:active:not(:disabled) {
  transform: scale(0.98);
  background: var(--wb-tint-lg);
}
.v-row.is-cur {
  background: var(--wb-surface);
  color: var(--wb-ink);
  box-shadow:
    inset 0 0 0 1px var(--wb-primary-line),
    var(--wb-shadow-sm);
}
.v-row.is-cur .v-chev {
  color: var(--wb-primary);
  opacity: 1;
  transform: translateX(2px);
}
.v-row.is-disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
.v-row.is-sel .v-initial {
  background: var(--wb-primary);
  color: var(--wb-primary-ink);
}
.v-glyph {
  width: 22px;
  height: 22px;
  border-radius: 6px;
  background: var(--wb-raise);
  color: var(--wb-muted);
  display: grid;
  place-items: center;
  font-size: var(--wb-fs-xs);
  font-weight: 700;
  flex: none;
  transition: background var(--wb-dur-fast) var(--wb-ease), color var(--wb-dur-fast) var(--wb-ease);
}
.v-glyph.is-sel {
  background: var(--wb-success-soft);
  color: var(--wb-success);
}
.v-tick {
  display: grid;
  place-items: center;
}
.v-initial {
  font-family: var(--font-mono);
}
.v-meta {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.v-name {
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.v-sub {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
}
.v-tag {
  font-size: var(--wb-fs-2xs);
  padding: 1px 6px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
  font-weight: 600;
  flex: none;
}
.v-chev {
  flex: none;
  opacity: 0.4;
  transition: transform var(--wb-dur-fast) var(--wb-ease), opacity var(--wb-dur-fast) var(--wb-ease);
}

/* === 右列：模型 === */
.col-model {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  padding: 0 6px 6px;
}
.m-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 6px 6px;
  flex: none;
}
.m-vendor {
  flex: 1;
  min-width: 0;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: var(--wb-fs-sm);
  font-weight: 600;
  color: var(--wb-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.m-vendor-dot {
  width: 6px;
  height: 6px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-primary);
  flex: none;
}
.m-filter {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 0 6px 0 8px;
  height: var(--wb-ctl-h-sm);
  border-radius: var(--wb-radius-sm);
  background: var(--wb-raise);
  border: 1px solid transparent;
  width: 140px;
  transition: border-color var(--wb-dur-fast) var(--wb-ease), width var(--wb-dur) var(--wb-ease);
}
.m-filter:focus-within {
  border-color: var(--wb-primary-line);
  width: 180px;
  background: var(--wb-surface);
}
.m-filter-ic {
  color: var(--wb-muted);
  flex: none;
}
.m-filter-in {
  flex: 1;
  min-width: 0;
  background: transparent;
  border: 0;
  outline: none;
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink);
  padding: 0;
}
.m-filter-in::placeholder {
  color: var(--wb-muted);
}
.m-filter-clr {
  border: 0;
  background: transparent;
  width: 16px;
  height: 16px;
  display: grid;
  place-items: center;
  border-radius: var(--wb-radius-full);
  color: var(--wb-muted);
  cursor: pointer;
  transition: background var(--wb-dur-fast) var(--wb-ease), color var(--wb-dur-fast) var(--wb-ease);
}
.m-filter-clr:hover {
  background: var(--wb-tint);
  color: var(--wb-ink);
}
.m-filter-clr:active {
  transform: scale(0.88);
  background: var(--wb-tint-lg);
}
.m-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 2px 0 0;
}
.m-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  border: 0;
  background: transparent;
  padding: 7px 8px;
  border-radius: var(--wb-radius-sm);
  cursor: pointer;
  text-align: left;
  color: var(--wb-ink-2);
  transition:
    background var(--wb-dur-fast) var(--wb-ease),
    color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.m-row:hover:not(:disabled) {
  background: var(--wb-tint);
  color: var(--wb-ink);
}
.m-row:active:not(:disabled) {
  transform: scale(0.985);
  background: var(--wb-tint-lg);
}
.m-row.is-hi {
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
}
.m-row:disabled {
  cursor: not-allowed;
}
/* 应用中的那一行：不吃 hover 底色、不吃按下缩放——已经在提交了，
   再给「邀请」反馈会让人以为还能再点一次 */
.m-row.is-loading {
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
  pointer-events: none;
  cursor: progress;
  opacity: 1;
}
.m-glyph {
  width: 16px;
  height: 16px;
  display: grid;
  place-items: center;
  flex: none;
  color: var(--wb-primary);
}
.m-name {
  flex: 1;
  min-width: 0;
  font-size: var(--wb-fs-sm);
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.m-row.is-hi .m-name {
  font-weight: 600;
}
.m-caps {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex: none;
}
.m-win {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
}
.m-tag {
  font-size: 9.5px;
  padding: 1px 5px;
  border-radius: var(--wb-radius-full);
  font-weight: 600;
  letter-spacing: 0.02em;
}
.tag-vision {
  background: var(--wb-primary-soft);
  color: var(--wb-primary);
}
.tag-off {
  background: var(--wb-tint);
  color: var(--wb-muted);
}
.m-loading {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  flex: none;
}

/* 规规矩矩面板：与 .pop-row 一致 */
.chips-pop-perm .pop-row {
  border-radius: 0;
  padding: 10px 12px;
}
.chips-pop-perm .pop-row:last-child {
  border-bottom: 0;
  border-bottom-left-radius: var(--wb-radius);
  border-bottom-right-radius: var(--wb-radius);
}
.grow {
  display: flex;
  flex-direction: column;
  min-width: 0;
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
.pr-check {
  color: var(--wb-success);
  flex: none;
}

/* 加载 / 警告 */
.pop-row.is-warn {
  display: flex;
  align-items: center;
  gap: 8px;
  text-align: left;
  font-size: var(--wb-fs-hint);
  color: var(--wb-warning);
  cursor: default;
  background: var(--wb-warning-soft);
  border-radius: 0;
  padding: 8px 12px;
}
.spinner {
  width: 14px;
  height: 14px;
  border-radius: var(--wb-radius-full);
  border: 1.6px solid var(--wb-line-2);
  border-top-color: var(--wb-primary);
  animation: spin 0.8s linear infinite;
  flex: none;
}
.spinner-sm {
  width: 12px;
  height: 12px;
  border-width: 1.4px;
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .pop-enter-active,
  .pop-leave-active,
  .v-row,
  .chip-chev,
  .v-chev,
  .m-filter,
  .m-row {
    transition: none;
    animation: none;
  }
}
</style>
