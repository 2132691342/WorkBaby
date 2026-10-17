<script setup lang="ts">
// 提交面板：勾选要入栈的文件 → 填提交信息（可让助手起草）→ 确认后提交。
// spec09 的不变量在这里落地：提交前一定列出将入的文件与增删行数，
// 助手只起草、不代填也不代提——提交这一下始终由用户点下去。
import { computed, ref, watch } from 'vue'
import * as api from '../../api'
import { useToastStore } from '../../stores/toast'
import type { GitFileVO, GitStatusVO } from '../../types/api'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{
  status: GitStatusVO | null
  sessionId: string
}>()
const emit = defineEmits<{ (e: 'done', status: GitStatusVO): void }>()

const toast = useToastStore()

// 勾选态按 path 存；已入栈的文件默认勾上——用户多半是想把准备好的都提交掉。
const picked = ref<Set<string>>(new Set())
const message = ref('')
const confirming = ref(false)
const busy = ref(false)
const drafting = ref(false)

const files = computed<GitFileVO[]>(() => props.status?.files || [])
const pickedFiles = computed(() => files.value.filter((f) => picked.value.has(f.path)))
const stagedFiles = computed(() => files.value.filter((f) => f.staged))

// 将入这次提交的统计：已入栈的 + 本次新勾的（还没入栈）。
const sum = computed(() =>
  pickedFiles.value.reduce(
    (n, f) => {
      n.add += f.additions
      n.del += f.deletions
      return n
    },
    { add: 0, del: 0 },
  ),
)

const STATUS_TEXT: Record<string, string> = {
  M: '改过',
  A: '新增',
  D: '删除',
  R: '改名',
  '?': '未跟踪',
}

function shortPath(p: string) {
  const parts = p.split('/').filter(Boolean)
  return parts.length > 3 ? '…/' + parts.slice(-3).join('/') : p
}

function toggle(path: string) {
  const next = new Set(picked.value)
  next.has(path) ? next.delete(path) : next.add(path)
  picked.value = next
}

// 换了一批文件（提交完、刷新后）就按新状态重算勾选：留着上一次的勾会让用户提交错东西。
function syncPicked() {
  const next = new Set<string>()
  for (const f of files.value) if (f.staged) next.add(f.path)
  picked.value = next
}

// 写操作后父级会回传新的状态：那时重算勾选，别让上一次的选择跟着过来。
watch(
  () => props.status,
  () => syncPicked(),
)

async function stage() {
  const paths = pickedFiles.value.filter((f) => !f.staged).map((f) => f.path)
  if (!paths.length) return
  busy.value = true
  try {
    const st = await api.git.stage(paths, props.sessionId)
    emit('done', st)
  } catch (e) {
    toast.bad(`入栈失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = false
  }
}

async function unstageAll() {
  if (!stagedFiles.value.length) return
  busy.value = true
  try {
    const st = await api.git.unstage(
      stagedFiles.value.map((f) => f.path),
      props.sessionId,
    )
    emit('done', st)
  } catch (e) {
    toast.bad(`取出失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = false
  }
}

// 助手起草：结果只填进输入框，用户还能改，也还能直接提交。
async function draft() {
  if (drafting.value) return
  drafting.value = true
  try {
    const res = await api.git.draftMessage(
      pickedFiles.value.map((f) => f.path),
      props.sessionId,
    )
    if (res.err) {
      toast.bad(`起草失败：${res.err}`)
      return
    }
    message.value = res.message
    toast.ok('已按改动起草，你可以改完再提交')
  } catch (e) {
    toast.bad(`起草失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    drafting.value = false
  }
}

async function commit() {
  if (!message.value.trim()) {
    toast.bad('先写一句这次改了什么')
    return
  }
  busy.value = true
  try {
    const res = await api.git.commit_(message.value.trim(), props.sessionId)
    message.value = ''
    confirming.value = false
    emit('done', res.status)
    toast.ok(`已提交 ${res.hash.slice(0, 7)}`)
  } catch (e) {
    toast.bad(`提交失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    busy.value = false
  }
}

// 没有勾选时按钮不给点：空提交只会换来一句「没什么可提交」，
// 而那句话在点之前就能凭勾选状态判断出来。
const canCommit = computed(() => pickedFiles.value.length > 0 && !!message.value.trim())
const needStage = computed(() => pickedFiles.value.some((f) => !f.staged))
</script>

<template>
  <div class="gp">
    <div v-if="!files.length" class="gp-empty">
      <AppIcon name="check" size="ic-lg" />
      <div>
        <b>没有可提交的改动</b>
        <p>助手改过的文件会出现在这里；干干净净说明这一轮还没动任何东西。</p>
      </div>
    </div>

    <template v-else>
      <div class="gp-pick">
        <div class="gp-pick-hd">
          <span class="ttl">将入这次提交</span>
          <span class="cnt">{{ pickedFiles.length }} / {{ files.length }}</span>
          <span class="sp" />
          <span class="sum mono">+{{ sum.add }} −{{ sum.del }}</span>
        </div>
        <ul class="gp-list">
          <li v-for="f in files" :key="f.path">
            <label class="gp-row" :class="{ 'is-on': picked.has(f.path) }">
              <input v-model="picked" type="checkbox" :value="f.path" @change="toggle(f.path)" />
              <span class="nm mono" :title="f.path">{{ shortPath(f.path) }}</span>
              <span class="tag" :class="f.staged ? 'is-staged' : ''">
                {{ f.staged ? '已入栈' : STATUS_TEXT[f.status] || f.status }}
              </span>
              <span class="pct mono">+{{ f.additions }} −{{ f.deletions }}</span>
            </label>
          </li>
        </ul>
      </div>

      <div class="gp-msg">
        <label class="gp-label" for="gp-msg">提交信息</label>
        <textarea
          id="gp-msg"
          v-model="message"
          class="gp-textarea"
          rows="3"
          maxlength="5000"
          placeholder="一句话说清这次改了什么，例如：修复导出表格时列宽丢失"
        />
        <div class="gp-msg-foot">
          <button
            class="btn btn-sm"
            type="button"
            :class="{ 'is-loading': drafting }"
            :disabled="drafting || busy || !pickedFiles.length"
            @click="draft()"
          >
            <AppIcon v-if="!drafting" name="sparkles" size="ic-xs" />
            {{ drafting ? '正在读改动…' : '让助手起草' }}
          </button>
          <span class="sp" />
          <span class="cnt mono">{{ message.length }} / 5000</span>
        </div>
      </div>

      <div class="gp-act">
        <button
          class="btn btn-sm"
          type="button"
          :class="{ 'is-loading': busy }"
          :disabled="busy || !needStage"
          @click="stage()"
        >
          {{ needStage ? '入栈勾选项' : '勾选的都已入栈' }}
        </button>
        <button
          class="btn btn-sm"
          type="button"
          :disabled="busy || !stagedFiles.length"
          @click="unstageAll()"
        >
          全部取出
        </button>
        <span class="sp" />
        <button
          class="btn btn-sm btn-primary"
          type="button"
          :class="{ 'is-loading': busy }"
          :disabled="busy || !canCommit"
          @click="commit()"
        >
          {{ busy ? '处理中…' : '提交' }}
        </button>
      </div>

      <!-- 确认态：把「提交这两个字背后是哪些文件」摊开再说一次 -->
      <div v-if="confirming" class="gp-confirm">
        <p>将提交 {{ pickedFiles.length }} 个文件，+{{ sum.add }} −{{ sum.del }} 行。</p>
        <div class="gp-confirm-act">
          <button class="btn btn-sm" type="button" @click="confirming = false">再想想</button>
          <button class="btn btn-sm btn-primary" type="button" @click="commit()">确认提交</button>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.gp {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
}
.gp-empty {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-4);
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-radius);
  background: var(--wb-surface-2);
  color: var(--wb-muted);
}
.gp-empty b {
  display: block;
  font-size: var(--wb-fs-sm);
  color: var(--wb-ink);
}
.gp-empty p {
  margin: 2px 0 0;
  font-size: var(--wb-fs-xs);
}
.gp-pick {
  border: 1px solid var(--wb-line);
  border-radius: var(--wb-radius);
  overflow: hidden;
}
.gp-pick-hd {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border-bottom: 1px solid var(--wb-line);
  background: var(--wb-surface-2);
  font-size: var(--wb-fs-xs);
}
.gp-pick-hd .ttl {
  font-weight: 600;
  color: var(--wb-ink-2);
}
.gp-pick-hd .sp {
  flex: 1;
}
.gp-list {
  max-height: 260px
;
  overflow-y: auto;
}
.gp-row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  cursor: pointer;
  transition: background var(--wb-dur) var(--wb-ease);
}
.gp-row:hover {
  background: var(--wb-tint);
}
.gp-row.is-on {
  background: var(--wb-primary-soft);
}
.gp-row .nm {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-2);
}
.tag {
  flex: none;
  padding: 1px 6px;
  border-radius: var(--wb-radius-full);
  border: 1px solid var(--wb-line-2);
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.tag.is-staged {
  border-color: var(--wb-primary-line);
  color: var(--wb-primary);
}
.pct {
  flex: none;
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
.gp-msg {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-2);
}
.gp-label {
  font-size: var(--wb-fs-xs);
  font-weight: 600;
  color: var(--wb-ink-2);
}
.gp-textarea {
  width: 100%;
  padding: var(--wb-sp-2) var(--wb-sp-3);
  border: 1px solid var(--wb-line-2);
  border-radius: var(--wb-radius);
  background: var(--wb-surface);
  color: var(--wb-ink);
  font-family: var(--font-sans);
  font-size: var(--wb-fs-sm);
  line-height: var(--wb-lh-base);
  resize: vertical;
  transition: border-color var(--wb-dur) var(--wb-ease);
}
.gp-textarea:focus {
  outline: none;
  border-color: var(--wb-primary);
}
.gp-msg-foot {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
}
.gp-msg-foot .sp {
  flex: 1;
}
.gp-act {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-2);
  padding-top: var(--wb-sp-2);
  border-top: 1px solid var(--wb-line);
}
.gp-act .sp {
  flex: 1;
}
.gp-confirm {
  padding: var(--wb-sp-3);
  border: 1px solid var(--wb-primary-line);
  border-radius: var(--wb-radius);
  background: var(--wb-primary-soft);
}
.gp-confirm p {
  margin: 0 0 var(--wb-sp-2);
  font-size: var(--wb-fs-xs);
  color: var(--wb-ink-2);
}
.gp-confirm-act {
  display: flex;
  justify-content: flex-end;
  gap: var(--wb-sp-2);
}
.mono {
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
}
.cnt {
  font-size: var(--wb-fs-2xs);
  color: var(--wb-muted);
}
</style>
