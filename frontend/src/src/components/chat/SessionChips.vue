<script setup lang="ts">
// 输入区里的两个会话开关：默认模型 + 动手前规矩（原先挂在会话头右侧）。
import { computed, ref } from 'vue'
import * as api from '../../api'
import { useSessionStore } from '../../stores/session'
import { useSettingsStore } from '../../stores/settings'
import AppIcon from '../common/AppIcon.vue'

// 后端 /sessions/:id/permission 只认这三个值，别再引入第四种叫法。
const PERMISSIONS = [
  { key: 'ask', name: '每次问我', desc: '改文件、跑命令前都先问一句，最稳妥' },
  { key: 'auto_edit', name: '自动改文件', desc: '改文件不再打断，跑命令与跑脚本仍会问' },
  { key: 'yolo', name: '全自动', desc: '什么都不问，直接做（只建议在自己电脑上用）' },
] as const

const session = useSessionStore()
const settings = useSettingsStore()

const open = ref<'model' | 'perm' | null>(null)
const models = ref<string[]>([])
const providerId = ref('')
const modelLoading = ref(false)

const modelName = computed(() => session.current?.model || settings.boot?.default_model || '默认模型')
const permName = computed(() => PERMISSIONS.find((p) => p.key === session.current?.permission)?.name || PERMISSIONS[0].name)

// 模型名从已配置的服务商里选，不让用户背模型 ID
async function loadModels() {
  modelLoading.value = true
  try {
    const list = await api.providers.list()
    const p = list.find((x) => x.id === session.current?.provider_id) || list.find((x) => x.is_default) || list[0]
    providerId.value = p?.id || ''
    models.value = p ? [...p.models] : []
  } finally {
    modelLoading.value = false
  }
}

function show(kind: 'model' | 'perm') {
  open.value = kind
  if (kind === 'model' && !models.value.length) void loadModels()
}

function toggle(kind: 'model' | 'perm') {
  if (open.value === kind) open.value = null
  else show(kind)
}

async function chooseModel(model: string) {
  if (!providerId.value) return
  await session.setModel(providerId.value, model)
  open.value = null
}

async function choosePermission(key: string) {
  await session.setPermission(key)
  open.value = null
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
        <div class="menu-label">用哪个模型</div>
        <div v-if="modelLoading" class="pop-row muted">正在读取模型列表…</div>
        <div v-else-if="!models.length" class="pop-row muted">还没有配置模型服务</div>
        <template v-else>
          <button
            v-for="m in models"
            :key="m"
            class="pop-row"
            :class="{ 'is-hi': m === session.current?.model }"
            type="button"
            @click="chooseModel(m)"
          >
            <AppIcon v-if="m === session.current?.model" name="check" size="ic-xs" />
            <span v-else class="pr-gap" />
            {{ m }}
          </button>
        </template>
      </template>

      <template v-else>
        <div class="menu-label">助手动手前的规矩</div>
        <button
          v-for="p in PERMISSIONS"
          :key="p.key"
          class="pop-row"
          :class="{ 'is-hi': p.key === session.current?.permission }"
          type="button"
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
</style>
