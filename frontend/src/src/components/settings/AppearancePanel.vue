<script setup lang="ts">
// 外观面板：只管「看起来」的事——主题、字体、字号、消息显示。
// 凡是影响「怎么做事」的（执行方式、驻留、自启、工作目录）都在「行为」里，
// 混在一起时用户找不到自己要改的那一项。
import { computed, onMounted, ref } from 'vue'
import * as api from '../../api'
import {
  FONTS,
  FONT_SIZES,
  bgBlur,
  bgImage,
  bgStrength,
  currentFont,
  currentSize,
  syncFromSettings,
} from '../../composables/useAppearance'
import { useTheme } from '../../composables/useTheme'
import { useChatStore } from '../../stores/chat'
import { useSettingsStore } from '../../stores/settings'
import { useToastStore } from '../../stores/toast'
import AppIcon from '../common/AppIcon.vue'
import { OpenFileDialog } from '../../../wailsjs/go/main/App'

const { theme, setTheme, THEMES } = useTheme()
const store = useSettingsStore()
const toast = useToastStore()

async function setFont(key: string) {
  currentFont.value = key
  try {
    await store.setValue('font_family', key)
  } catch {
    /* setValue 已提示 */
  }
}

async function setSize(key: string) {
  currentSize.value = key
  try {
    await store.setValue('font_size', key)
  } catch {
    /* setValue 已提示 */
  }
}

// 思考显示是即时生效的界面开关：setValue 乐观落值，成功后同步进聊天 store——
// 只落库的话，下一次流式之前界面仍按旧值渲染。
const thinkingBusy = ref(false)
async function toggleThinking() {
  if (thinkingBusy.value) return
  const next = store.values['show_thinking'] === 'false'
  thinkingBusy.value = true
  try {
    await store.setValue('show_thinking', next ? 'true' : 'false')
    useChatStore().setShowThinking(next)
  } catch {
    /* setValue 已提示并回滚 */
  } finally {
    thinkingBusy.value = false
  }
}

// ---- 自定义背景 ----
// 浓度与模糊给离散档位而不是滑杆：滑杆要另造一个控件（全站没有 range 样式），
// 而这两项本来也不需要连续取值——三档足够，选完立刻看得见差别。
const BG_STRENGTHS = [
  { key: 'soft', name: '淡', value: 0.25 },
  { key: 'mid', name: '适中', value: 0.5 },
  { key: 'bold', name: '明显', value: 0.75 },
]
const BG_BLURS = [
  { key: 'none', name: '不模糊', value: 0 },
  { key: 'light', name: '轻', value: 6 },
  { key: 'mid', name: '中', value: 14 },
]

const bgBusy = ref(false)
const hasBg = computed(() => bgImage.value !== '')

// 写回本地与 store 两份：界面靠本地 ref 立刻生效（不等往返），
// store 那份是给「下次进设置页」对齐用的，漏了就会出现「明明换了图，进来又是旧的」。
function applyBgVo(image: string, strength: number, blur: number) {
  bgImage.value = image
  bgStrength.value = strength
  bgBlur.value = blur
  store.values = {
    ...store.values,
    bg_image: image,
    bg_opacity: String(strength),
    bg_blur: String(blur),
  }
}

async function saveBg(path: string, strength: number, blur: number) {
  if (bgBusy.value) return
  bgBusy.value = true
  try {
    const vo = await api.settings.setBackground(path, strength, blur)
    applyBgVo(vo.image, vo.opacity, vo.blur)
  } catch (e) {
    toast.bad(`${path ? '换背景' : '清除背景'}失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    bgBusy.value = false
  }
}

async function pickBackground() {
  if (bgBusy.value) return
  // 只对图片开一道过滤：把任意文件当背景塞进来，失败会发生在后端，
  // 那时用户已经选完文件了，提示来得太晚。
  const p = await OpenFileDialog('选一张背景图', '*.png;*.jpg;*.jpeg;*.webp;*.bmp')
  if (!p) return
  await saveBg(p, bgStrength.value, bgBlur.value)
}

function setStrength(v: number) {
  if (!hasBg.value) return
  void saveBg(bgImage.value, v, bgBlur.value)
}

function setBlur(v: number) {
  if (!hasBg.value) return
  void saveBg(bgImage.value, bgStrength.value, v)
}

// 背景三项以服务端为准：它会对越界值做归一化（浓度封顶 0.9、模糊封顶 24px），
// 设置表里的原值不一定等于实际生效值。
async function loadBg() {
  try {
    const vo = await api.settings.background()
    applyBgVo(vo.image, vo.opacity, vo.blur)
  } catch {
    /* 读不到就保持上一次的显示：背景不是关键设置，不值得为它弹错 */
  }
}

onMounted(async () => {
  syncFromSettings()
  await loadBg()
})
</script>

<template>
  <div class="ap">
    <div class="card p-sm">
      <h3>主题</h3>
      <p class="hint">浅白适合白天办公，暗黑适合夜里或长时间盯屏幕。</p>
      <div class="theme-row">
        <button
          v-for="t in THEMES"
          :key="t.key"
          class="theme-card"
          :class="{ 'is-on': theme === t.key }"
          type="button"
          @click="setTheme(t.key)"
        >
          <span class="sw" :style="{ background: t.swatch }" />
          <b>{{ t.name }}</b>
          <span class="theme-sub">{{ t.key === 'light' ? '石板 · 纯白 · 蓝' : '夜石板 · 亮蓝 · 亮紫' }}</span>
        </button>
      </div>
    </div>

    <div class="pair">
      <div class="card p-sm">
        <h3>字体</h3>
        <p class="hint">只影响正文与界面文字，路径和数字仍用等宽字体。</p>
        <div class="font-list">
          <button
            v-for="f in FONTS"
            :key="f.key"
            class="perm"
            :class="{ 'is-on': currentFont === f.key }"
            type="button"
            @click="setFont(f.key)"
          >
            <b :style="{ fontFamily: f.stack }">{{ f.name }}</b>
            <span>{{ f.desc }}</span>
          </button>
        </div>
      </div>

      <div class="card p-sm">
        <h3>字号</h3>
        <p class="hint">整站等比放大，布局不会错位。</p>
        <div class="perm-row">
          <button
            v-for="s in FONT_SIZES"
            :key="s.key"
            class="perm size-cell"
            :class="{ 'is-on': currentSize === s.key }"
            type="button"
            @click="setSize(s.key)"
          >
            <b :style="{ fontSize: `calc(var(--wb-fs-md) * ${s.scale})` }">A</b>
            <span>{{ s.name }}</span>
          </button>
        </div>
      </div>
    </div>

    <div class="card p-sm">
      <h3>背景</h3>
      <p class="hint">
        换一张自己的图当界面背景。图片会复制一份存起来，原图挪走也不影响。
      </p>
      <div class="bg-row">
        <div class="bg-preview" :class="{ 'is-empty': !hasBg }" aria-hidden="true">
          <span v-if="!hasBg" class="bg-none">未设置</span>
        </div>
        <div class="bg-acts">
          <div class="bg-btns">
            <button
              class="btn btn-sm"
              type="button"
              :class="{ 'is-loading': bgBusy }"
              :disabled="bgBusy"
              @click="pickBackground"
            >
              <AppIcon v-if="!bgBusy" name="image" size="ic-xs" />
              {{ hasBg ? '换一张' : '选一张' }}
            </button>
            <button
              v-if="hasBg"
              class="btn btn-sm btn-ghost"
              type="button"
              :disabled="bgBusy"
              @click="saveBg('', bgStrength, bgBlur)"
            >
              清除
            </button>
          </div>
          <div class="bg-opts">
            <div class="bg-opt">
              <span class="bg-lab">浓度</span>
              <div class="seg" role="group" aria-label="背景浓度">
                <button
                  v-for="s in BG_STRENGTHS"
                  :key="s.key"
                  type="button"
                  :class="{ on: Math.abs(bgStrength - s.value) < 0.01 }"
                  :disabled="!hasBg || bgBusy"
                  @click="setStrength(s.value)"
                >
                  {{ s.name }}
                </button>
              </div>
            </div>
            <div class="bg-opt">
              <span class="bg-lab">模糊</span>
              <div class="seg" role="group" aria-label="背景模糊">
                <button
                  v-for="b in BG_BLURS"
                  :key="b.key"
                  type="button"
                  :class="{ on: Math.abs(bgBlur - b.value) < 0.01 }"
                  :disabled="!hasBg || bgBusy"
                  @click="setBlur(b.value)"
                >
                  {{ b.name }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div class="card p-sm">
      <h3>消息显示</h3>
      <p class="hint">控制思考过程与工具细节的展开方式。</p>
      <label class="row">
        <span class="grow">
          <b>显示思考过程</b>
          <span>模型推理时展示思考内容，关闭后只显示最终答复</span>
        </span>
        <button
          class="switch"
          type="button"
          role="switch"
          :aria-checked="store.values['show_thinking'] !== 'false'"
          :class="{
            'is-on': store.values['show_thinking'] !== 'false',
            'is-loading': thinkingBusy,
          }"
          :disabled="thinkingBusy"
          @click="toggleThinking"
        />
      </label>
    </div>
  </div>
</template>

<style scoped>
.ap {
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-4);
  min-width: 0;
}
/* 背景卡：左边一块预览，右边按钮 + 两组档位 */
.bg-row {
  display: flex;
  align-items: stretch;
  gap: var(--wb-sp-4);
  flex-wrap: wrap;
}
.bg-preview {
  flex: none;
  width: 148px;
  height: 92px;
  border-radius: var(--wb-radius);
  border: 1px solid var(--wb-border);
  background-color: var(--wb-raise);
  background-image: var(--wb-user-bg-url);
  background-size: cover;
  background-position: center;
  display: grid;
  place-items: center;
  overflow: hidden;
}
.bg-preview.is-empty {
  background-image: none;
}
.bg-none {
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.bg-acts {
  flex: 1 1 260px;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--wb-sp-3);
  justify-content: center;
}
.bg-btns {
  display: flex;
  gap: var(--wb-sp-2);
}
.bg-opts {
  display: grid;
  gap: var(--wb-sp-2);
}
.bg-opt {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  min-width: 0;
}
.bg-lab {
  flex: none;
  width: 44px;
  font-size: var(--wb-fs-xs);
  font-weight: 600;
  color: var(--wb-ink-2);
}
.pair {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--wb-sp-4);
}
@media (max-width: 860px) {
  .pair {
    grid-template-columns: minmax(0, 1fr);
  }
}
.theme-row {
  display: flex;
  gap: var(--wb-sp-3);
  margin-top: var(--wb-sp-2);
}
.theme-card {
  flex: 1;
  display: grid;
  gap: var(--wb-sp-2);
  justify-items: center;
  padding: var(--wb-sp-3) var(--wb-sp-4);
  border-radius: var(--wb-radius);
  background: var(--wb-surface);
  border: 1px solid var(--wb-border);
  cursor: pointer;
  color: var(--wb-ink);
  transition:
    border-color var(--wb-dur) var(--wb-ease),
    background var(--wb-dur) var(--wb-ease),
    box-shadow var(--wb-dur) var(--wb-ease),
    transform var(--wb-dur-fast) var(--wb-ease);
}
.theme-card:hover:not(:disabled) {
  border-color: var(--wb-border-strong);
  background: var(--wb-surface-hover);
}
.theme-card:active:not(:disabled) {
  border-color: var(--wb-border-strong);
  transform: scale(0.98);
}
.theme-card:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.theme-card.is-loading {
  pointer-events: none;
  cursor: progress;
}
.theme-card.is-on {
  border-color: var(--wb-primary);
  box-shadow: 0 0 0 3px var(--wb-primary-soft);
}
.theme-card b {
  font-size: var(--wb-fs-md);
}
.theme-sub {
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
.sw {
  width: var(--wb-ctl-h-xl);
  height: var(--wb-ctl-h-xl);
  border-radius: var(--wb-radius-full);
  box-shadow: inset 0 0 0 1px var(--wb-tint-lg);
}
.hint {
  color: var(--wb-muted);
  font-size: var(--wb-fs-sm);
  margin: var(--wb-sp-2) 0 var(--wb-sp-3);
}
.font-list {
  display: grid;
  gap: var(--wb-sp-2);
}
/* .perm-row / .perm 是共享选择卡，已收回到 wb-ui.css；这里只留本地修饰符 */
.size-cell {
  justify-items: center;
  text-align: center;
}
.row {
  display: flex;
  align-items: center;
  gap: var(--wb-sp-3);
  padding: var(--wb-sp-2) 0;
  cursor: pointer;
}
.row .grow {
  display: grid;
  gap: 2px;
  min-width: 0;
}
.row b {
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}
.row span {
  font-size: var(--wb-fs-xs);
  color: var(--wb-muted);
}
</style>
