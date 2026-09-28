<script setup lang="ts">
// 审批决策卡：面向小白的「放行一次 / 总是放行 / 拒绝」三选。
import { ref } from 'vue'
import type { ApprovalVO } from '../../types/api'
import { summarizeArgs } from '../../utils/md'
import AppIcon from '../common/AppIcon.vue'

defineProps<{ approval: ApprovalVO; busy?: boolean }>()
const emit = defineEmits<{ decide: [id: string, approved: boolean, scope: string] }>()

const scope = ref<'once' | 'session'>('once')
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
      <button class="btn btn-sm btn-outline" :disabled="busy" @click="emit('decide', approval.id, false, scope)">拒绝</button>
      <button class="btn btn-sm btn-primary" :disabled="busy" @click="emit('decide', approval.id, true, scope)">
        放行
      </button>
      <span class="ap-note">拿不准就拒绝，可以让助手换个做法</span>
    </div>
  </div>
</template>
