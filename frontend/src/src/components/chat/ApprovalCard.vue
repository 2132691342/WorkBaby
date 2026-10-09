<script setup lang="ts">
// 审批决策卡：面向小白的「放行一次 / 总是放行 / 拒绝」三选。
import { ref } from 'vue'
import type { ApprovalVO } from '../../types/api'
import { summarizeArgs } from '../../utils/md'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{
  approval: ApprovalVO
  busy?: boolean
  /** 决策回调（返回 Promise）：成功后卡片由父级移除，失败由卡片复位重试。 */
  onDecide?: (id: string, approved: boolean, scope: string) => Promise<void> | void
}>()

const scope = ref<'once' | 'session'>('once')
// 点了哪个决定就只有那个按钮转圈，await 期间另一个按钮禁用，防双击重复决策
const pending = ref<'' | 'allow' | 'reject'>('')
async function decide(id: string, approved: boolean) {
  if (props.busy || pending.value) return
  pending.value = approved ? 'allow' : 'reject'
  try {
    await props.onDecide?.(id, approved, scope.value)
  } catch {
    // 失败已由 store 弹提示；这里吸收异常，让 finally 能复位按钮供重试
  } finally {
    // 无论成败都要复位：失败时卡片不会被移除，不复位就是一个永久转圈的按钮，
    // 而这是「助手卡住」时用户唯一的自救入口。
    pending.value = ''
  }
}
const riskText: Record<string, string> = {
  low: '低风险',
  medium: '中风险',
  high: '高风险',
}
</script>

<template>
  <div class="approve" :class="{ 'is-danger': approval.risk === 'high' }">
    <div class="hd">
      <AppIcon :name="approval.risk === 'high' ? 'alert' : 'help'" />
      <span class="ap-title">{{ approval.label || approval.tool }} 想要执行一个操作</span>
    </div>
    <div class="cmd">{{ summarizeArgs(approval.args) }}</div>
    <div v-if="approval.reason" class="meta">{{ approval.reason }}</div>
    <div class="ft">
      <label class="check">
        <input v-model="scope" type="radio" value="once" />
        <span class="box" />
        只这一次
      </label>
      <label v-if="approval.risk !== 'high'" class="check">
        <input v-model="scope" type="radio" value="session" />
        <span class="box" />
        本会话内都放行
      </label>
      <span class="tag">{{ riskText[approval.risk] || approval.risk }}</span>
    </div>
    <div class="approve-foot">
      <button
        class="btn btn-sm btn-outline"
        :class="{ 'is-loading': pending === 'reject' }"
        :disabled="busy || (!!pending && pending !== 'reject')"
        @click="decide(approval.id, false)"
      >
        拒绝
      </button>
      <button
        class="btn btn-sm btn-primary"
        :class="{ 'is-loading': pending === 'allow' }"
        :disabled="busy || (!!pending && pending !== 'allow')"
        @click="decide(approval.id, true)"
      >
        放行
      </button>
      <span class="ap-note">拿不准就拒绝，可以让助手换个做法</span>
    </div>
  </div>
</template>
