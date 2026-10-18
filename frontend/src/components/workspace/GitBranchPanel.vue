<script setup lang="ts">
// 分支面板：列出分支、切换、新建、推送。
// 只允许普通 checkout 与 checkout -b：强删、强改指向留给用户自己的 Git 工具。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '../../api'
import { useToastStore } from '../../stores/toast'
import type { GitBranchVO, GitStatusVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{ sessionId: string }>()
const emit = defineEmits<{ (e: 'done', status: GitStatusVO): void }>()

const { t } = useI18n()
const toast = useToastStore()

const branches = ref<GitBranchVO[]>([])
const loading = ref(true)
const busy = ref('')
const creating = ref(false)
const newName = ref('')

const current = computed(() => branches.value.find((b) => b.current))

async function load() {
  loading.value = true
  try {
    branches.value = await api.git.branches(props.sessionId)
  } catch (e) {
    branches.value = []
    toast.bad(t('common.loadFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    loading.value = false
  }
}
onMounted(load)

async function switchTo(name: string) {
  if (busy.value) return
  busy.value = name
  try {
    const st = await api.git.switchBranch(name, props.sessionId)
    emit('done', st)
    await load()
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

async function create() {
  const name = newName.value.trim()
  if (!name) {
    toast.bad(t('workspace.branchName'))
    return
  }
  if (busy.value) return
  busy.value = name
  try {
    const st = await api.git.createBranch(name, props.sessionId)
    newName.value = ''
    creating.value = false
    emit('done', st)
    await load()
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}

async function push() {
  if (busy.value) return
  busy.value = 'push'
  try {
    const st = await api.git.push(props.sessionId)
    emit('done', st)
    toast.ok(t('workspace.pushed'))
    await load()
  } catch (e) {
    toast.bad(t('common.saveFailed', { msg: (e as Error)?.message || '' }))
  } finally {
    busy.value = ''
  }
}
</script>

<template>
  <div class="br">
    <header class="br-head">
      <span class="br-cur">
        <AppIcon name="git-branch" size="ic-sm" />
        {{ current?.name || t('workspace.branchEmpty') }}
      </span>
      <span class="sp" />
      <el-button size="small" :loading="busy === 'push'" :disabled="!!busy" @click="push">
        {{ busy === 'push' ? t('workspace.pushing') : t('workspace.push') }}
      </el-button>
      <el-button size="small" type="primary" :disabled="!!busy" @click="creating = !creating">
        {{ t('workspace.branchNew') }}
      </el-button>
    </header>

    <div v-if="creating" class="br-new">
      <el-input
        v-model="newName"
        size="small"
        :placeholder="t('workspace.branchName')"
        @keydown.enter="create"
      />
      <el-button size="small" type="primary" :loading="!!busy" @click="create">
        {{ t('common.create') }}
      </el-button>
      <el-button size="small" @click="creating = false">{{ t('common.cancel') }}</el-button>
    </div>

    <div v-if="loading" class="br-state"><span class="spin" />{{ t('common.loading') }}</div>
    <ul v-else-if="branches.length" class="br-list">
      <li v-for="b in branches" :key="b.name">
        <div class="br-row" :class="{ 'is-on': b.current }">
          <span class="br-dot" :class="{ 'is-on': b.current }" />
          <span class="br-name" :title="b.name">{{ b.name }}</span>
          <el-tag v-if="b.current" size="small" type="primary">{{ t('workspace.branchCurrent') }}</el-tag>
          <span v-if="b.ahead" class="br-ahead">{{ t('workspace.ahead', { n: b.ahead }) }}</span>
          <span v-if="b.behind" class="br-behind">{{ t('workspace.behind', { n: b.behind }) }}</span>
          <span class="sp" />
          <el-button
            v-if="!b.current"
            size="small"
            text
            :loading="busy === b.name"
            :disabled="!!busy"
            @click="switchTo(b.name)"
          >
            {{ t('workspace.branchSwitch') }}
          </el-button>
        </div>
      </li>
    </ul>
    <div v-else class="br-state">{{ t('workspace.branchEmpty') }}</div>
  </div>
</template>

<style scoped>
.br {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}
.br-head {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.br-cur {
  display: inline-flex;
  align-items: center;
  gap: var(--wb-sp-2);
  font-size: var(--wb-fs-sm);
  font-weight: 700;
  color: var(--wb-ink);
}
.br-new {
  display: flex;
  gap: var(--wb-sp-2);
}
.br-list {
  display: flex;
  flex-direction: column;
  gap: 1px;
}
.br-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-radius: var(--wb-radius);
  transition: background var(--wb-dur-fast) var(--wb-ease);
}
.br-row:hover {
  background: var(--wb-surface-2);
}
.br-row.is-on {
  background: var(--wb-primary-soft);
}
.br-dot {
  width: 7px;
  height: 7px;
  border-radius: var(--wb-radius-full);
  background: var(--wb-border-strong);
  flex: none;
}
.br-dot.is-on {
  background: var(--wb-primary);
}
.br-name {
  font-family: var(--font-mono);
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.br-ahead,
.br-behind {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
  font-variant-numeric: tabular-nums;
}
.br-state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-8) 0;
  color: var(--wb-muted);
  font-size: var(--wb-fs-sm);
}
.spin {
  width: calc(13px * var(--wb-vw-scale));
  height: calc(13px * var(--wb-vw-scale));
  border-radius: var(--wb-radius-full);
  border: 1.8px solid var(--wb-border-strong);
  border-top-color: var(--wb-primary);
  animation: wb-spin 0.8s linear infinite;
}
.sp {
  flex: 1;
}
</style>
