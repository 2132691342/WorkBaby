<script setup lang="ts">
// 服务商编辑弹窗：名 / 地址 / 协议 / 密钥，加一张模型表。
// 草稿与三处就地编辑只在弹窗打开期间存在；拉回的上游清单渲染成可点选 chip。
// 勾选语义：推理默认不写（非推理模型会报错）；模态逐选，写错一个词配置静默失效。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import AppIcon from '@/components/common/AppIcon.vue'
import { DEFAULT_CONTEXT_WINDOW, DEFAULT_MAX_TOKENS } from '@/composables/useProviderForm'
import type { ModelEntry } from '@/types/api'

/** 草稿形态：与 ProviderView 对齐，但密钥是可以改的空串而不是掩码。 */
export interface ProviderForm {
  name: string
  base_url: string
  api: string
  api_key: string
  models: ModelEntry[]
}

const props = defineProps<{
  modelValue: boolean
  form: ProviderForm
  /** 正在编辑的服务商名（空表示新建） */
  editing: string
  saving: boolean
  /** 正在拉上游清单（true 表示按钮转圈） */
  fetching: boolean
  /** 最近一次拉回来的上游清单（弹窗渲染成 chip） */
  fetched: ModelEntry[]
  apiOptions: string[]
}>()

const emit = defineEmits<{
  'update:modelValue': [open: boolean]
  save: []
  fetchModels: []
}>()

const { t } = useI18n()

const open = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit('update:modelValue', v),
})

// 跟随全局视口缩放：小窗口里弹窗也跟着收，不会顶出屏幕
const dialogWidth = 'calc(760px * var(--wb-vw-scale))'

/** 名字在编辑时不可改：它是配置里的键，改了等于新建一条。 */
const nameLocked = computed(() => Boolean(props.editing))

function addModel(): void {
  // 新建默认 text 输入与一组常见窗口：空着提交会让引擎侧静默失败。
  props.form.models.push({
    model_id: '',
    model_name: '',
    context_window: DEFAULT_CONTEXT_WINDOW,
    max_tokens: DEFAULT_MAX_TOKENS,
    reasoning: false,
    input: ['text'],
  })
}

function removeModel(index: number): void {
  props.form.models.splice(index, 1)
}

/** 这个拉回来的模型在不在草稿里（chip 的高亮态就靠它）。 */
function isFetchedOn(entry: ModelEntry): boolean {
  return props.form.models.some((m) => m.model_id === entry.model_id)
}

/**
 * 点一枚拉取 chip：在草稿里就移出，不在就按默认值加进来。
 * 默认值与手动「加一个模型」一致——空着提交会让引擎侧静默失败。
 */
function toggleFetched(entry: ModelEntry): void {
  const index = props.form.models.findIndex((m) => m.model_id === entry.model_id)
  if (index >= 0) {
    props.form.models.splice(index, 1)
    return
  }
  props.form.models.push({
    model_id: entry.model_id,
    model_name: entry.model_name || '',
    context_window: entry.context_window || DEFAULT_CONTEXT_WINDOW,
    max_tokens: entry.max_tokens || DEFAULT_MAX_TOKENS,
    reasoning: Boolean(entry.reasoning),
    input: entry.input?.length ? [...entry.input] : ['text'],
  })
}

function setReasoning(index: number, on: boolean): void {
  props.form.models[index].reasoning = on
}

function hasInput(model: ModelEntry, kind: string): boolean {
  return Boolean(model.input?.includes(kind))
}

/**
 * 勾 / 取消一种输入模态。
 *
 * 至少保留 text：空数组在引擎那边等价于「什么都收不了」，
 * 而那种配置不会报错、只会静默失败——用户只会看到「模型不回话」。
 */
function setInput(index: number, kind: string, on: boolean): void {
  const model = props.form.models[index]
  let next = (model.input ?? []).filter((k) => k !== kind)
  if (on) next = [...next, kind]
  if (!next.length) next = ['text']
  model.input = next
}
</script>

<template>
  <el-dialog
    v-model="open"
    :title="props.editing ? t('models.editProvider') : t('models.addProvider')"
    :width="dialogWidth"
  >
    <el-form label-position="top" size="small">
      <el-form-item :label="t('common.name')">
        <el-input v-model="props.form.name" :disabled="nameLocked" placeholder="router" />
      </el-form-item>
      <el-form-item :label="t('models.baseUrl')">
        <el-input v-model="props.form.base_url" placeholder="https://api.example.com/v1" />
      </el-form-item>
      <div class="field-row">
        <el-form-item :label="t('models.api')" class="grow">
          <el-select v-model="props.form.api" style="width: 100%">
            <el-option v-for="option in props.apiOptions" :key="option" :label="option" :value="option" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('models.apiKey')" class="grow">
          <el-input v-model="props.form.api_key" :placeholder="t('models.apiKeyHint')" />
        </el-form-item>
      </div>
    </el-form>

    <div class="models-head">
      <span class="field-label">{{ t('models.modelsLabel') }}</span>
      <span class="sp" />
      <!-- 拉上游清单：填了地址与密钥就能问出有哪些模型，
           不用一个个手打 id。拉回来的清单在下方以 chip 呈现，点选要哪些。 -->
      <button
        class="wb-btn sm ghost"
        :class="{ 'is-loading': props.fetching }"
        :disabled="props.fetching || props.saving"
        :title="t('models.fetchModelsHint')"
        @click="emit('fetchModels')"
      >
        <AppIcon name="download" :size="12" />
        {{ t('models.fetchModels') }}
      </button>
      <button class="wb-btn text sm" :disabled="props.fetching" @click="addModel">
        {{ t('models.addModel') }}
      </button>
    </div>

    <!-- 拉取结果：点选要保留的模型。选中的会出现在下方的模型行里，
         再点一次移出——「问出来的」和「要保存的」在这里分家。 -->
    <div v-if="props.fetched.length" class="fetched">
      <span class="fetched-hint">{{ t('models.fetchedHint') }}</span>
      <div class="fetched-chips">
        <button
          v-for="entry in props.fetched"
          :key="entry.model_id"
          type="button"
          class="fetched-chip"
          :class="{ 'is-on': isFetchedOn(entry) }"
          :title="entry.model_name || entry.model_id"
          @click="toggleFetched(entry)"
        >
          <AppIcon v-if="isFetchedOn(entry)" name="check" :size="11" />
          {{ entry.model_id }}
        </button>
      </div>
    </div>

    <!-- 表头：四个字段各自的含义写在列头上，不靠占位符（输入后就消失了） -->
    <div v-if="props.form.models.length" class="model-row model-head">
      <span>{{ t('models.modelId') }}</span>
      <span>{{ t('common.name') }}</span>
      <span>{{ t('models.contextWindow') }}</span>
      <span>{{ t('models.maxTokens') }}</span>
      <span>{{ t('models.reasoning') }}</span>
      <span>{{ t('models.input') }}</span>
      <span />
    </div>

    <div v-for="(model, index) in props.form.models" :key="index" class="model-row">
      <input v-model="model.model_id" class="wb-input" :placeholder="t('models.modelId')" />
      <input v-model="model.model_name" class="wb-input" :placeholder="t('common.name')" />
      <input
        v-model.number="model.context_window"
        class="wb-input num-input"
        type="number"
        :placeholder="t('models.contextWindow')"
      />
      <input
        v-model.number="model.max_tokens"
        class="wb-input num-input"
        type="number"
        :placeholder="t('models.maxTokens')"
      />
      <label class="cap-check" :title="t('models.reasoningHint')">
        <input
          type="checkbox"
          :checked="Boolean(model.reasoning)"
          @change="(e: Event) => setReasoning(index, (e.target as HTMLInputElement).checked)"
        />
        <span>R</span>
      </label>
      <div class="cap-inputs">
        <label class="cap-check" :title="t('models.inputText')">
          <input
            type="checkbox"
            :checked="hasInput(model, 'text')"
            @change="(e: Event) => setInput(index, 'text', (e.target as HTMLInputElement).checked)"
          />
          <span>TXT</span>
        </label>
        <label class="cap-check" :title="t('models.inputImage')">
          <input
            type="checkbox"
            :checked="hasInput(model, 'image')"
            @change="(e: Event) => setInput(index, 'image', (e.target as HTMLInputElement).checked)"
          />
          <span>IMG</span>
        </label>
      </div>
      <button
        class="wb-btn icon ghost danger"
        :title="t('common.delete')"
        @click="removeModel(index)"
      >
        <AppIcon name="close" :size="13" />
      </button>
    </div>

    <template #footer>
      <button class="wb-btn" @click="open = false">{{ t('common.cancel') }}</button>
      <button
        class="wb-btn primary"
        :class="{ 'is-loading': props.saving }"
        :disabled="props.saving || !props.form.name.trim()"
        @click="emit('save')"
      >
        {{ t('common.save') }}
      </button>
    </template>
  </el-dialog>
</template>

<style scoped>
.field-row {
  display: flex;
  gap: var(--wb-sp-3);
}

.grow {
  flex: 1;
  min-width: 0;
}

.models-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--wb-sp-2);
  margin: var(--wb-sp-2) 0;
}

.sp {
  flex: 1;
}

.field-label {
  font-size: var(--wb-fs-3xs);
  font-weight: 700;
  letter-spacing: var(--wb-ls-plate);
  text-transform: uppercase;
  color: var(--wb-faint);
}

.model-row {
  display: grid;
  grid-template-columns: 1.6fr 1.2fr 104px 96px 44px 96px 32px;
  gap: var(--wb-sp-2);
  align-items: center;
  margin-bottom: var(--wb-sp-2);
}

/* 表头那一行没有输入框，靠文字说明每一列是什么 */
.model-head {
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
  margin-bottom: var(--wb-sp-1);
}

/* 拉取结果：一排可点选的 chip。宽 760px 的弹窗装得下一排十几个 id，
   超出的部分换行——这里允许换行，chip 流不动才需要滚动。 */
.fetched {
  margin-bottom: var(--wb-sp-2);
}

.fetched-hint {
  display: block;
  margin-bottom: var(--wb-sp-1);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-faint);
}

.fetched-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.fetched-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  max-width: 100%;
  padding: 2px 8px;
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-r-full);
  background: var(--wb-surface);
  color: var(--wb-mute);
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-2xs);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition:
    color var(--wb-dur-fast) var(--wb-ease),
    border-color var(--wb-dur-fast) var(--wb-ease),
    background-color var(--wb-dur-fast) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}

.fetched-chip:hover:not(:disabled) {
  color: var(--wb-brand);
  border-color: var(--wb-brand-line);
  transform: scale(var(--wb-press-hover));
}

.fetched-chip:active:not(:disabled) {
  transform: scale(var(--wb-press-active));
}

.fetched-chip.is-on {
  color: var(--wb-brand);
  border-color: var(--wb-brand-line);
  background: var(--wb-brand-soft);
}

.model-row .wb-input {
  min-width: 0;
  height: var(--wb-ctl-sm);
  font-size: var(--wb-fs-2xs);
}

.num-input {
  text-align: right;
  font-family: var(--wb-font-mono);
}

.cap-check {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-family: var(--wb-font-mono);
  font-size: var(--wb-fs-3xs);
  color: var(--wb-mute);
  cursor: pointer;
  user-select: none;
}

.cap-inputs {
  display: flex;
  gap: var(--wb-sp-2);
}
</style>
