<script setup lang="ts">
// 外观面板：只管「看起来」的事——主题、字体、字号、消息显示。
// 凡是影响「怎么做事」的（执行方式、驻留、自启、工作目录）都在「行为」里，
// 混在一起时用户找不到自己要改的那一项。
import { onMounted } from 'vue'
import { FONTS, FONT_SIZES, currentFont, currentSize, syncFromSettings } from '../../composables/useAppearance'
import { useTheme } from '../../composables/useTheme'
import { useSettingsStore } from '../../stores/settings'

const { theme, setTheme, THEMES } = useTheme()
const store = useSettingsStore()

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

onMounted(syncFromSettings)
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
          <span class="theme-sub">{{ t.key === 'light' ? '紫雾 · 纯白 · 深紫' : '墨夜 · 暗紫 · 浅紫' }}</span>
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
      <h3>消息显示</h3>
      <p class="hint">控制思考过程与工具细节的展开方式。</p>
      <label class="row">
        <span class="grow">
          <b>显示思考过程</b>
          <span>模型推理时展示思考内容，关闭后只显示最终答复</span>
        </span>
        <input
          class="wb-switch"
          type="checkbox"
          :checked="store.values['show_thinking'] !== 'false'"
          @change="store.setValue('show_thinking', ($event.target as HTMLInputElement).checked ? 'true' : 'false')"
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
