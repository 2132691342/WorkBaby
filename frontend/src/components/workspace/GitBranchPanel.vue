<script setup lang="ts">
// 分支面板：列出分支、切换、新建。
// 只允许普通 checkout 与 checkout -b：强删、强改指向这类动作留给用户自己的 Git 工具（specs/09）。
import { computed, onMounted, ref } from 'vue'
import * as api from '../../api'
import { useToastStore } from '../../stores/toast'
import type { GitBranchVO, GitStatusVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{ sessionId: string }>()
const emit = defineEmits<{ (e: 'done', status: GitStatusVO): void }>()

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
    toast.bad(`读不到分支列表：${(e as Error)?.message || '请重试'}`)
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
    toast.ok(`已切到 ${name}`)
    await load()
  } catch (e) {
    // 切不过去多半是工作区有未提交改动挡着：原文来自 git，直接说给用户听
    toast.bad(`切不过去：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}

async function create() {
  const name = newName.value.trim()
  if (!name) {
    toast.bad('先给新分支起个名字')
    return
  }
  if (busy.value) return
  busy.value = name
  try {
    const st = await api.git.createBranch(name, props.sessionId)
    newName.value = ''
    creating.value = false
    emit('done', st)
    toast.ok(`已新建并切到 ${name}`)
    await load()
  } catch (e) {
    toast.bad(`新建失败：${(e as Error)?.message || '请重试'}`)
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
    toast.ok('已推送')
    await load()
  } catch (e) {
    // 推送被拒最常见是远端有新提交：原文里带着该先 pull，别猜
    toast.bad(`推送失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = ''
  }
}
</script>

<template>
  <div class="gb">
    <div class="gb-hd">
      <span class="cur">
        <AppIcon name="git-branch" size="ic-sm" />
        当前 <b>{{ current?.name || '（读不到）' }}</b>
      </span>
      <span class="sp" />
      <button class="btn btn-sm" type="button" :class="{ 'is-loading': busy === 'push' }" :disabled="!!busy" @click="push()">
        推送
      </button>
      <button class="btn btn-sm" type="button" :class="{ 'is-loading': loading }" :disabled="loading || !!busy" @click="creating = !creating">
        新建分支
      </button>
    </div>

    <div v-if="creating" class="gb-new">
      <input v-model="newName" class="gb-input" type="text" maxlength="128" placeholder="例如 feature/表格导出" @keyup.enter="create()" />
      <button class="btn btn-sm btn-primary" type="button" :class="{ 'is-loading': !!busy }" :disabled="!!busy" @click="create()">
        新建并切过去
      </button>
      <button class="btn btn-sm btn-ghost" type="button" @click="creating = false">取消</button>
    </div>

    <p v-if="loading" class="none">正在读取…</p>
    <ul v-else-if="branches.length" class="gb-list">
      <li v-for="b in branches" :key="b.name">
        <span class="gb-name mono" :class="{ 'is-cur': b.current }">{{ b.name }}</span>
        <span v-if="b.ahead" class="tag">领先 {{ b.ahead }}</span>
        <span v-if="b.behind" class="tag">落后 {{ b.behind }}</span>
        <span class="sp" />
        <button
          v-if="!b.current"
          class="btn btn-sm btn-ghost"
          type="button"
          :class="{ 'is-loading': busy === b.name }"
          :disabled="!!busy"
          @click="switchTo(b.name)"
        >
          切换
        </button>
        <span v-else class="gb-cur-tag">当前</span>
      </li>
    </ul>
    <p v-else class="none">还没有其他分支。</p>
  </div>
</template>

<style scoped>
.gb {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}
.gb-hd {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.gb-hd .cur {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.gb-hd .cur b {
  color: var(--wb-ink);
}
.gb-hd .sp {
  flex: 1;
}
.gb-new {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.gb-input {
  flex: 1;
  min-width: 0;
  height: var(--wb-ctl-h-sm);
  padding: 0 var(--wb-sp-3);
  border: 1px solid var(--wb-line-2);
  border-radius: var(--wb-radius-full);
  background: var(--wb-surface);
  color: var(--wb-ink);
  font-size: var(--wb-fs-sm);
  transition: border-color var(--wb-dur) var(--wb-ease);
}
.gb-input:focus {
  outline: none;
  border-color: var(--wb-primary);
}
.gb-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.gb-list li {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-1);
  border-radius: var(--wb-radius-sm);
  font-size: var(--wb-fs-sm);
  transition: background var(--wb-dur) var(--wb-ease);
}
.gb-list li:hover {
  background: var(--wb-tint);
}
.gb-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--wb-ink-2);
}
.gb-name.is-cur {
  font-weight: 700;
  color: var(--wb-primary);
}
.gb-list .sp {
  flex: 1;
}
.tag {
  flex: none;
  padding: 1px 6px;
  border-radius: var(--wb-radius-full);
  border: 1px solid var(--wb-line-2);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.gb-cur-tag {
  flex: none;
  font-size: var(--wb-fs-2xs);
  font-weight: 700;
  color: var(--wb-primary);
}
.none {
  margin: 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-muted);
}
.mono {
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
}
</style>
