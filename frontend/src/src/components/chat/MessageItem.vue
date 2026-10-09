<script setup lang="ts">
// 单条消息：用户气泡 / 助手回答（markdown + 思考折叠 + 工具卡）。
import { computed, ref } from 'vue'
import type { MessageVO } from '../../types/api'
import { useToastStore } from '../../stores/toast'
import { renderMarkdown } from '../../utils/md'
import { fmtCount } from '../../utils/num'
import AppIcon from '../common/AppIcon.vue'

const props = defineProps<{ msg: MessageVO; plain?: boolean }>()

// 正文里可能混着 <think> 标签（没有单独 thinking 字段的消息）：渲染时拆开，
// 正文干净、思考归位——用户看到裸标签才会以为程序坏了。
const split = computed(() => splitThink(props.msg.content || ''))
const html = computed(() => (split.value.text ? renderMarkdown(split.value.text) : ''))
// 正文拆掉思考后可能为空，这种回合只保留思考块，不画空气泡。
const hasText = computed(() => split.value.text.trim().length > 0)
// 优先用后端单独存的 thinking；没有再退回从正文里拆出来的。
const thinking = computed(() => props.msg.thinking || split.value.think)
const thinkOpen = ref(false)
const copied = ref(false)

// splitThink 把 <think>…</think> 从正文里拆出来。
function splitThink(s: string): { think: string; text: string } {
  if (!s.includes('<think>')) return { think: '', text: s }
  const think = [...s.matchAll(/<think>([\s\S]*?)<\/think>/g)]
    .map((m) => m[1].trim())
    .filter(Boolean)
    .join('\n\n')
  const text = s
    .replace(/<think>[\s\S]*?<\/think>/g, '')
    .replace(/<\/?think>/g, '')
    .trim()
  return { think, text }
}

const copying = ref(false)
async function copy() {
  if (!split.value.text || copying.value) return
  copying.value = true
  try {
    await navigator.clipboard.writeText(split.value.text)
    copied.value = true
    setTimeout(() => (copied.value = false), 1200)
  } catch {
    // 剪贴板被系统策略拒绝时不能静默：用户以为复制成功了
    useToastStore().bad('复制失败，请手动选中文本复制')
  } finally {
    copying.value = false
  }
}

function fmtTime(ms: number) {
  const d = new Date(ms)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

// 一个笼统的「N tokens」读不出任何信息：用户真正关心的是三件事——
// 这一轮读进来多少、写出去多少、其中多少走了缓存（决定长对话贵不贵）。
// 上下文占用单独给：它回答的是「还能聊多久」，和这一轮花了多少是两回事。
const metrics = computed(() => {
  const u = props.msg.usage
  if (!u) return []
  // 命中量大于输入量说明这是一条老口径的行（输入只记了未命中部分）：读侧按同一规则
  // 补回总量，口径与仪表盘汇总一致。不补的话，命中率只能压到 100% 这种不实读数。
  const input = u.cached > u.input ? u.input + u.cached : u.input
  const context = Math.max(u.context, input)
  const out: { k: string; v: string; tip: string; hi?: boolean }[] = [
    { k: '入', v: fmtCount(input), tip: '这一轮发出去的输入 token（含系统提示与历史消息）' },
    { k: '出', v: fmtCount(u.output), tip: '模型写出来的 token' },
  ]
  if (input > 0) {
    const rate = u.cached > 0 ? Math.min(100, Math.round((u.cached / input) * 100)) : 0
    out.push({
      k: '缓存',
      v: rate > 0 ? `${rate}%` : '无',
      tip: u.cached > 0
        ? `${fmtCount(u.cached)} 个输入 token 命中了服务端缓存，这部分单价更低`
        : '这一轮没有命中服务端缓存，输入部分按全量计费',
      hi: rate > 0,
    })
  }
  if (context > 0) {
    out.push({
      k: '上下文',
      v: fmtCount(context),
      tip: '这一轮发出时占用的上下文窗口，超了会自动整理较早的内容',
    })
  }
  return out
})
</script>

<template>
  <!-- plain：整块回答内部的一段内容，头像与缩进由外层统一给，自己不要再画一遍 -->
  <div class="turn">
    <!-- 用户消息 -->
    <div v-if="msg.role === 'user'" class="msg-u-wrap">
      <div class="msg-u">
        <div v-if="msg.images?.length" class="msg-imgs">
          <img
            v-for="(im, i) in msg.images"
            :key="i"
            class="msg-img"
            :src="`data:${im.mime};base64,${im.base64}`"
            :alt="`图片 ${i + 1}`"
          />
        </div>
        <div v-if="msg.content">{{ msg.content }}</div>
      </div>
      <time class="msg-t">{{ fmtTime(msg.created_at) }}</time>
    </div>

    <!-- 助手回答：纯思考回合（只有工具声明、正文还没出）不画头像，
         否则一次多轮回答会碎成一串互不相干的 WB 图标。
         工具结果由 MessageList 收进所属回合，不在这里单独渲染。 -->
    <div v-else class="msg-a" :class="{ 'is-think-only': !hasText, 'is-plain': plain }">
      <div v-if="hasText && !plain" class="avatar"><span>WB</span></div>
      <div class="msg-a-body">
        <div v-if="thinking" class="think">
          <button class="wb-think-toggle" type="button" @click="thinkOpen = !thinkOpen">
            <AppIcon :name="thinkOpen ? 'chevron-down' : 'chevron-right'" size="ic-xs" />
            思考过程
          </button>
          <div v-if="thinkOpen" class="wb-think-body">{{ thinking }}</div>
        </div>
        <!-- 只有工具声明、正文还没出来的回合不画气泡：
             每轮都画一个空头像，一次回答就会碎成一串互不相干的 WB 图标。 -->
        <div v-if="hasText" class="bubble" :class="{ 'is-error': msg.is_error }" v-html="html" />
        <div v-if="hasText" class="msg-acts">
          <button
            class="act"
            type="button"
            :class="{ 'is-loading': copying }"
            :disabled="copying"
            @click="copy"
          >
            <AppIcon name="copy" size="ic-xs" />
            {{ copied ? '已复制' : '复制' }}
          </button>
          <template v-for="m in metrics" :key="m.k">
            <span class="sep" />
            <span class="act is-dim" :class="{ 'is-hit': m.hi }" :title="m.tip">
              <b>{{ m.k }}</b>{{ m.v }}
            </span>
          </template>
          <span v-if="msg.latency_ms" class="sep" />
          <span v-if="msg.latency_ms" class="act is-dim">{{ (msg.latency_ms / 1000).toFixed(1) }}s</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* 用户随消息发的图片：小缩略图排在气泡内，点击消息不发新事件 */
.msg-imgs {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 4px;
}
.msg-img {
  max-width: 180px;
  max-height: 120px;
  border-radius: var(--wb-radius-sm);
  border: 1px solid var(--wb-line-2);
  object-fit: cover;
}
.act.is-dim {
  color: var(--wb-muted);
  cursor: default;
  font-variant-numeric: tabular-nums;
  gap: 4px;
}
/* 指标名用粗细区分而不是加色块：一行四个读数已经够密，再上底色就成了表 */
.act.is-dim b {
  font-weight: 600;
  opacity: 0.75;
}
.act.is-hit {
  color: var(--wb-success);
}
.bubble.is-error {
  border-color: var(--wb-danger);
  color: var(--wb-danger);
}
/* 纯思考回合：没有头像也没有正文，缩进对齐到助手列即可 */
.msg-a.is-think-only {
  padding-left: calc(var(--wb-avatar-size) + var(--wb-sp-3));
}
/* 整块回答内部：头像在外层，自身不缩进也不补左边距 */
.msg-a.is-plain {
  padding-left: 0;
}
</style>
