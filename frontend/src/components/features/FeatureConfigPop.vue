<script setup lang="ts">
// 功能配置抽屉：后端给出字段表（类型 / 候选 / 不可用原因），这里只负责渲染与收集。
// 不写死任何功能的具体表单——新增可配项只改后端 schema。
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import type { FeatureConfigFieldVO } from '../../types/api'
import { errorText } from '../../utils/errors'
import { useToastStore } from '../../stores/toast'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{
  /** 要配的功能码；空串 = 关闭 */
  code: string
  /** 功能显示名 */
  title: string
}>()

const emit = defineEmits<{ close: []; saved: [] }>()

const { t } = useI18n()
const toast = useToastStore()

const fields = ref<FeatureConfigFieldVO[]>([])
const configurable = ref(false)
const loading = ref(false)
const saving = ref(false)
const failed = ref('')
/** 编辑中的值：字段名 → 新值（只放被改过的字段，未改的由后端保留原值） */
const draft = ref<Record<string, unknown>>({})

const open = computed(() => !!props.code)

/** 某字段当前在草稿里的值（没改过就用后端给的值） */
function valueOf(f: FeatureConfigFieldVO): unknown {
  return f.field_name in draft.value ? draft.value[f.field_name] : f.field_value
}

function setValue(f: FeatureConfigFieldVO, v: unknown) {
  draft.value = { ...draft.value, [f.field_name]: v }
}

/** multi_select 的勾选切换 */
function toggleMulti(f: FeatureConfigFieldVO, optionValue: string, checked: boolean) {
  const cur = Array.isArray(valueOf(f)) ? ([...(valueOf(f) as string[])] as string[]) : []
  const next = checked ? [...cur, optionValue] : cur.filter((v) => v !== optionValue)
  setValue(f, next)
}

const isChecked = (f: FeatureConfigFieldVO, optionValue: string): boolean =>
  Array.isArray(valueOf(f)) && (valueOf(f) as string[]).includes(optionValue)

const dirty = computed(() => Object.keys(draft.value).length > 0)

async function load() {
  if (!props.code) return
  loading.value = true
  failed.value = ''
  draft.value = {}
  try {
    const resp = await api.featureConfig.get(props.code)
    fields.value = resp?.resp_fields ?? []
    configurable.value = resp?.resp_configurable ?? false
  } catch (e) {
    failed.value = errorText(e)
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!dirty.value || saving.value) return
  saving.value = true
  try {
    await api.featureConfig.save(props.code, draft.value)
    toast.ok(t('featurePanel.configSaved'))
    emit('saved')
    emit('close')
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: errorText(e) }))
  } finally {
    saving.value = false
  }
}

watch(() => props.code, load, { immediate: true })
</script>

<template>
  <Transition name="cfg-pop">
    <div v-if="open" class="cfg-mask" @click.self="emit('close')">
      <section class="cfg" role="dialog" aria-modal="true">
        <header class="cfg-head">
          <div class="cfg-tx">
            <h3 class="cfg-title">{{ t('featurePanel.configTitle', { name: title }) }}</h3>
            <p class="cfg-sub">{{ t('featurePanel.configSub') }}</p>
          </div>
          <button class="wb-btn cfg-x" type="button" :title="t('common.close')" @click="emit('close')">
            <AppIcon name="close" size="ic-sm" />
          </button>
        </header>

        <div class="cfg-body">
          <div v-if="loading" class="cfg-skel">
            <el-skeleton animated :rows="3" />
          </div>

          <div v-else-if="failed" class="cfg-err">
            <span>{{ t('common.loadFailed', { msg: failed }) }}</span>
            <el-button size="small" @click="load">{{ t('common.retry') }}</el-button>
          </div>

          <p v-else-if="!configurable" class="cfg-empty">
            {{ t('featurePanel.configNone') }}
          </p>

          <template v-else>
            <div v-for="f in fields" :key="f.field_name" class="field">
              <label class="field-label">{{ f.field_label }}</label>
              <p class="field-desc">{{ f.field_description }}</p>

              <!-- select：候选带不可用原因（缺密钥 / 未安装）时置灰但保留可见 -->
              <el-select
                v-if="f.field_kind === 'select'"
                :model-value="valueOf(f) as string"
                class="ctrl"
                @update:model-value="(v: string) => setValue(f, v)"
              >
                <el-option
                  v-for="o in f.field_options"
                  :key="o.option_value"
                  :label="o.option_reason ? `${o.option_label} — ${o.option_reason}` : o.option_label"
                  :value="o.option_value"
                  :disabled="!!o.option_reason && o.option_value !== (valueOf(f) as string)"
                />
              </el-select>

              <!-- multi_select：空选 = 不限制 -->
              <div v-else-if="f.field_kind === 'multi_select'" class="multis">
                <label v-for="o in f.field_options" :key="o.option_value" class="multi">
                  <el-checkbox
                    :model-value="isChecked(f, o.option_value)"
                    :disabled="!!o.option_reason"
                    @update:model-value="(v: boolean) => toggleMulti(f, o.option_value, v)"
                  />
                  <span class="multi-tx">{{ o.option_label }}</span>
                  <span v-if="o.option_reason" class="multi-why">{{ o.option_reason }}</span>
                </label>
              </div>

              <el-switch
                v-else-if="f.field_kind === 'boolean'"
                :model-value="!!valueOf(f)"
                @update:model-value="(v: boolean) => setValue(f, v)"
              />

              <el-input-number
                v-else-if="f.field_kind === 'number'"
                :model-value="Number(valueOf(f) ?? 0)"
                class="ctrl"
                controls-position="right"
                @update:model-value="(v: number | undefined) => setValue(f, v ?? 0)"
              />

              <el-input
                v-else
                :model-value="String(valueOf(f) ?? '')"
                class="ctrl"
                @update:model-value="(v: string) => setValue(f, v)"
              />
            </div>
          </template>
        </div>

        <footer class="cfg-foot">
          <el-button size="small" :disabled="saving" @click="emit('close')">
            {{ t('common.cancel') }}
          </el-button>
          <el-button
            size="small"
            type="primary"
            :loading="saving"
            :disabled="!dirty || saving || !configurable"
            @click="save"
          >
            {{ t('common.save') }}
          </el-button>
        </footer>
      </section>
    </div>
  </Transition>
</template>

<style scoped>
.cfg-mask {
  position: fixed;
  inset: 0;
  z-index: 40;
  display: grid;
  place-items: center;
  padding: var(--wb-sp-6);
  background: var(--wb-scrim, rgba(15, 23, 42, 0.32));
  backdrop-filter: blur(2px);
}
.cfg {
  width: min(calc(520px * var(--wb-vw-scale)), 100%);
  max-height: min(78vh, calc(620px * var(--wb-vw-scale)));
  display: flex;
  flex-direction: column;
  border-radius: var(--wb-radius-lg);
  border: 1px solid var(--wb-border-strong);
  background: var(--wb-surface-solid);
  box-shadow: var(--wb-shadow-pop);
  overflow: hidden;
}
.cfg-head {
  display: flex;
  align-items: flex-start;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-4);
  border-bottom: 1px solid var(--wb-border);
}
.cfg-tx {
  min-width: 0;
  flex: 1;
}
.cfg-title {
  margin: 0;
  font-size: var(--wb-fs-md);
  font-weight: 700;
  color: var(--wb-ink);
}
.cfg-sub {
  margin: 2px 0 0;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.cfg-x {
  flex: none;
  width: var(--wb-tile-sm);
  height: var(--wb-tile-sm);
  border-radius: var(--wb-radius-sm);
  color: var(--wb-muted);
  background: transparent;
  border: 1px solid transparent;
}
.cfg-x:hover {
  background: var(--wb-elev-hover);
  border-color: var(--wb-border);
  color: var(--wb-ink);
}
.cfg-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--wb-sp-4);
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
}
.cfg-skel {
  padding: var(--wb-sp-2) 0;
}
.cfg-err {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--wb-sp-3);
  font-size: var(--wb-fs-sm);
  color: var(--wb-danger);
}
.cfg-empty {
  margin: 0;
  padding: var(--wb-sp-5) 0;
  text-align: center;
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.field {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
}
.field-label {
  font-size: var(--wb-fs-sm);
  font-weight: 700;
  color: var(--wb-ink);
}
.field-desc {
  margin: 0 0 var(--wb-sp-1);
  font-size: var(--wb-fs-2xs);
  line-height: 1.55;
  color: var(--wb-muted);
}
.ctrl {
  width: 100%;
}
.multis {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-1);
}
.multi {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-1) var(--wb-sp-2);
  border-radius: var(--wb-radius-sm);
  transition: background-color var(--wb-dur) var(--wb-ease);
}
.multi:hover {
  background: var(--wb-elev-hover);
}
.multi-tx {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-ink-2);
  font-family: var(--font-mono);
}
.multi-why {
  margin-left: auto;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-warning);
}
.cfg-foot {
  display: flex;
  justify-content: flex-end;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-3) var(--wb-sp-4);
  border-top: 1px solid var(--wb-border);
  background: var(--wb-elev-hover);
}
.cfg-pop-enter-active,
.cfg-pop-leave-active {
  transition: opacity var(--wb-dur) var(--wb-ease);
}
.cfg-pop-enter-active .cfg,
.cfg-pop-leave-active .cfg {
  transition:
    transform var(--wb-dur) var(--wb-ease),
    opacity var(--wb-dur) var(--wb-ease);
}
.cfg-pop-enter-from,
.cfg-pop-leave-to {
  opacity: 0;
}
.cfg-pop-enter-from .cfg,
.cfg-pop-leave-to .cfg {
  opacity: 0;
  transform: translateY(8px) scale(0.98);
}
</style>
