<script setup lang="ts">
// 输入区：多行输入 + 待发送附件 + @ 引用 + / 命令菜单 + 工具行 + 发送/停止。
// 只做编排（持有状态、接事件），三块各自在组件里：
// ComposerStatus 状态条 / ComposerMenu 弹出菜单 / ComposerBar 工具行。
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import ComposerStatus from '@/components/chat/ComposerStatus.vue'
import ActivityBar from '@/components/chat/ActivityBar.vue'
import ComposerMenu from '@/components/chat/ComposerMenu.vue'
import ComposerBar from '@/components/chat/ComposerBar.vue'
import AttachStrip from '@/components/chat/AttachStrip.vue'
import { useChatStore } from '@/stores/chat'
import { useAppStore } from '@/stores/app'
import { useComposerDraft } from '@/composables/useComposerDraft'
import { useAttachments, baseName, toWorkspaceRelative } from '@/composables/useAttachments'
import { useComposerMenu } from '@/composables/useComposerMenu'
import { useComposerActions } from '@/composables/useComposerActions'
import { pickFiles } from '@/api/wails'
import type { MenuItem } from '@/composables/useComposerMenu'

const chat = useChatStore()
const app = useAppStore()
const { t } = useI18n()

const draft = ref('')
const textarea = ref<HTMLTextAreaElement | null>(null)
/** @ 菜单的浏览器：它的键盘导航（↑↓ / 回车）在这个组件里实现。 */
const mentionRef = ref<{ move: (d: number) => void; pickCurrent: () => boolean } | null>(null)

const sending = ref(false)
const queueing = ref(false)
const stopping = ref(false)
const attachError = ref('')

// 草稿按会话存：切走再切回来还在，关掉窗口也不丢。
const draftStore = useComposerDraft(computed(() => chat.sessionFile), draft)

// 引擎或「停止撤回排队」要求把一段话放回输入框：整段替换当前草稿。
// 消费即清空——它是一次性的，留着会让下一次输入被同样的内容顶掉。
watch(
  () => chat.draftOverride,
  (text) => {
    if (!text) return
    draft.value = text
    chat.draftOverride = ''
    void nextTick(() => {
      autosize()
      textarea.value?.focus()
    })
  },
)

// 附件：粘贴图片 / 回形针选文件 / @ 引用工作区文件，三种来源一个列表。
const attachments = useAttachments()

// 菜单：/ 命令 与 @ 引用。开关时机与选中后的动作都在 composable 里。
const composerMenu = useComposerMenu(draft, (name) => {
  draft.value = `/${name} `
  composerMenu.close()
  void nextTick(() => textarea.value?.focus())
})
const { menuIndex, busyCmd, mentionOpen, slashOpen, slashItems, applyItem } = composerMenu

// ── 菜单选中 ──
function onPickItem(item: MenuItem): void {
  applyItem(item)
  if (item.source === 'engine') void nextTick(autosize)
}

function onPickFile(relPath: string, name: string): void {
  const err = attachments.addWorkspaceFile(relPath, name)
  if (err) {
    attachError.value = err
    return
  }
  // 把触发菜单的那段 @xxx 从输入里去掉：引用已经变成附件条目了。
  draft.value = draft.value.replace(/@\S*$/, '').trimEnd()
  composerMenu.close()
  void nextTick(() => {
    autosize()
    textarea.value?.focus()
  })
}

// ── 附件 ──
async function onPaste(event: ClipboardEvent): Promise<void> {
  const errors = await attachments.addFromClipboard(event.clipboardData)
  if (errors.length) {
    attachError.value = errors.join('；')
    return
  }
  // 有图片时吃掉这次粘贴，避免同一张图既进附件又变成一段二进制文本；
  // 没有图片就放行——普通文本粘进输入框是正常操作。
  if (attachments.images.value.length) event.preventDefault()
}

/** 回形针：选文件。工作区内的变成相对路径引用，工作区外的给绝对路径。 */
async function browseFiles(): Promise<void> {
  const picked = await pickFiles(t('chat.attachPick'))
  if (!picked.length) return
  const errors: string[] = []
  for (const path of picked) {
    const rel = toWorkspaceRelative(path, app.workspace)
    const err = attachments.addWorkspaceFile(rel ?? path, baseName(path))
    if (err) errors.push(err)
  }
  if (errors.length) attachError.value = errors.join('；')
  void nextTick(() => textarea.value?.focus())
}

// ── 输入与键盘 ──
function autosize(): void {
  const el = textarea.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = `${Math.min(220, el.scrollHeight)}px`
}

/** 回车发送、Shift+回车换行。输入法组合中的回车不算发送。 */
function onKeydown(event: KeyboardEvent): void {
  if (event.isComposing) return

  if (slashOpen.value) {
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      composerMenu.move(1)
      return
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault()
      composerMenu.move(-1)
      return
    }
    if (event.key === 'Enter' || event.key === 'Tab') {
      event.preventDefault()
      const item = slashItems.value[menuIndex.value]
      if (item) onPickItem(item)
      return
    }
    if (event.key === 'Escape') {
      event.preventDefault()
      composerMenu.close()
      return
    }
  }

  if (mentionOpen.value) {
    // @ 菜单的键盘导航在 MentionBrowser 内部（它才知道有哪些条目）。
    // 父层只转发按键：列表在子组件里，指针在下标算在子组件里。
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      mentionRef.value?.move(1)
      return
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault()
      mentionRef.value?.move(-1)
      return
    }
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault()
      mentionRef.value?.pickCurrent()
      return
    }
    if (event.key === 'Escape') {
      event.preventDefault()
      composerMenu.close()
      return
    }
  }

  // 运行中按 Esc = 停止；只在输入框为空时响应，免得误按清掉上下文
  if (event.key === 'Escape' && chat.streaming && !draft.value.trim()) {
    event.preventDefault()
    void stopTurn()
    return
  }

  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault()
    void submit()
  }
}

const canSend = computed(
  () => (draft.value.trim().length > 0 || !attachments.isEmpty.value) && !sending.value,
)

async function submit(): Promise<void> {
  if (!canSend.value || !app.hasEngine) return
  const text = draft.value.trim()
  // 只有 `/xxx` 这种完整命令时，回车应当是「执行它」而不是把它当成一句话发出去。
  if (!attachments.isEmpty.value && text.startsWith('/') && !text.includes(' ')) {
    const exact = slashItems.value.find((item) => `/${item.name}` === text)
    if (exact) {
      onPickItem(exact)
      return
    }
  }
  sending.value = true
  attachError.value = ''
  try {
    const payload = attachments.payloads()
    if (chat.streaming) {
      await chat.steer(text, payload)
      chat.pushNotice('info', t('chat.queueSteer'))
    } else {
      await chat.send(text, payload)
    }
    draft.value = ''
    attachments.clear()
    draftStore.flush()
    await nextTick(autosize)
  } catch (err) {
    chat.promptError = err instanceof Error ? err.message : String(err)
  } finally {
    sending.value = false
  }
}

async function queue(): Promise<void> {
  if (!canSend.value || !chat.streaming) return
  queueing.value = true
  try {
    await chat.followUp(draft.value.trim(), attachments.payloads())
    draft.value = ''
    attachments.clear()
    draftStore.flush()
    await nextTick(autosize)
    chat.pushNotice('info', t('chat.queueAdded'))
  } catch (err) {
    chat.promptError = err instanceof Error ? err.message : String(err)
  } finally {
    queueing.value = false
  }
}

async function stopTurn(): Promise<void> {
  stopping.value = true
  try {
    await chat.stop()
  } catch (err) {
    chat.promptError = err instanceof Error ? err.message : String(err)
  } finally {
    stopping.value = false
  }
}

const actions = useComposerActions()

const hint = computed(() => {
  if (!app.hasEngine) return t('chat.noEngine')
  if (chat.streaming) return t('chat.placeholderBusy')
  return t('chat.placeholder')
})

/** 当前模型收不收图片：收才让用户粘图，否则那条提示会一直失败。 */
const imagesAllowed = computed(() => {
  const input = chat.model?.input_kinds
  if (!input || input.length === 0) return true
  return input.includes('image')
})

onMounted(() => void nextTick(autosize))

function focus(): void {
  textarea.value?.focus()
}

defineExpose({ focus })
</script>

<template>
  <div class="composer">
    <ComposerStatus :attach-error="attachError" @clear-attach-error="attachError = ''" />
    <ActivityBar :stopping="stopping" @stop="stopTurn" />

    <!-- 菜单的定位上下文：菜单贴着输入框上沿往上弹，必须相对「输入框那一块」定位。 -->
    <div class="composer-inner">
      <div class="composer-box">
        <!-- 附件占独立一行：超过两行就在区内滚动，不把输入框顶出屏 -->
        <AttachStrip :items="attachments.items.value" @remove="attachments.remove" />

        <!-- 输入块：撑满高度，点击空白区即可落光标 -->
        <div class="ta-shell" @click="focus()">
          <textarea
            ref="textarea"
            v-model="draft"
            class="ta"
            :placeholder="hint"
            :disabled="!app.hasEngine"
            rows="2"
            @input="autosize"
            @keydown="onKeydown"
            @paste="onPaste"
          />
        </div>

        <!-- 工具行：与输入块用一条分隔线切开，不再抢输入框的横向空间 -->
        <ComposerBar
          class="is-divided"
          :can-send="canSend"
          :sending="sending"
          :queueing="queueing"
          :stopping="stopping"
          :busy-cmd="busyCmd"
          :images-allowed="imagesAllowed"
          :count="draft.length"
          @browse-files="browseFiles"
          @queue="queue"
          @toggle-compact="actions.pickAutoCompaction"
          @stop="stopTurn"
          @submit="submit"
        />
      </div>

      <ComposerMenu
        v-if="slashOpen"
        kind="slash"
        :items="slashItems"
        :index="menuIndex"
        @hover="menuIndex = $event"
        @pick="onPickItem"
        @close="composerMenu.close()"
      />
      <ComposerMenu
        v-else-if="mentionOpen"
        kind="mention"
        :items="[]"
        :index="0"
        ref="mentionRef"
        @pick-file="onPickFile"
        @close="composerMenu.close()"
      />
    </div>
  </div>
</template>

<style scoped>
/* 输入区根：它自己在页面底部，菜单要从它内部往上弹。
   页脚的分隔线与底色在这里声明，不再依赖全局样式。 */
.composer {
  flex: none;
  border-top: 1px solid var(--wb-line);
  background: var(--wb-work);
  padding: var(--wb-sp-3) var(--wb-sp-5) var(--wb-sp-4);
}

.composer-inner {
  position: relative;
  width: 100%;
  max-width: var(--wb-col);
  margin: 0 auto;
}

/* 输入框容器：只管外壳，三块内容各自管自己的排布。 */
.composer-box {
  display: flex;
  flex-direction: column;
  padding: var(--wb-sp-1);
  border: 1px solid var(--wb-line-strong);
  border-radius: var(--wb-r-lg);
  background: var(--wb-surface);
  transition: border-color var(--wb-dur-fast) var(--wb-ease);
}

.composer-box:focus-within {
  border-color: var(--wb-brand-2);
  box-shadow: 0 0 0 3px var(--wb-brand-soft);
}

/* 输入块：给一个 68px 的最小高度——空态也得是一个看得见的大目标，
   高度不足时整块输入区看着就是一条缝，用户找不到可以点的地方。 */
.ta-shell {
  display: flex;
  min-height: calc(68px * var(--wb-vw-scale));
  padding: var(--wb-sp-1);
  cursor: text;
}

.ta {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: none;
  resize: none;
  background: transparent;
  color: var(--wb-ink);
  font-family: inherit;
  font-size: var(--wb-fs-md);
  line-height: var(--wb-lh-base);
  padding: var(--wb-sp-2) var(--wb-sp-3);
  max-height: calc(240px * var(--wb-vw-scale));
}

.ta:disabled {
  cursor: not-allowed;
}

.ta::placeholder {
  color: var(--wb-faint);
}

/* 工具行与输入块用一条分隔线切开 */
.is-divided {
  margin-top: var(--wb-sp-1);
  padding-top: var(--wb-sp-2);
  border-top: 1px solid var(--wb-line);
}
</style>
