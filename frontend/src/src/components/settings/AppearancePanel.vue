<script setup lang="ts">
// 外观面板：主题切换 + 常规设置项。
import { onMounted, ref } from 'vue'
import { useTheme } from '../../composables/useTheme'
import { useSettingsStore } from '../../stores/settings'
import { AutoStartEnabled, SetAutoStart } from '../../../wailsjs/go/main/App'

const { theme, setTheme, THEMES } = useTheme()
const store = useSettingsStore()
const autoStart = ref(false)
const autoStartErr = ref('')

// 权限档取值必须与后端 domain.PermissionXxx 一致，否则设置写入直接被拒。
const permissionOptions = [
  { key: 'ask', name: '先问我', desc: '读写文件、跑命令都先问我一句，推荐新手使用' },
  { key: 'auto_edit', name: '少打扰', desc: '改文件不问，跑命令和跑脚本仍然要审批' },
  { key: 'yolo', name: '全自动', desc: '不再询问，直接执行（只建议在自己电脑上用）' },
]

async function setPermission(key: string) {
  await store.setValue('permission', key)
  await store.loadBoot()
}

async function loadAutoStart() {
  try {
    autoStart.value = await AutoStartEnabled()
  } catch {
    autoStart.value = false
  }
}

async function toggleAutoStart() {
  autoStartErr.value = ''
  const next = !autoStart.value
  try {
    await SetAutoStart(next)
    autoStart.value = await AutoStartEnabled()
  } catch (e) {
    autoStartErr.value = (e as Error)?.message || '设置失败，请重启后再试一次'
    autoStart.value = await AutoStartEnabled().catch(() => autoStart.value)
  }
}

onMounted(loadAutoStart)
</script>

<template>
  <div class="ap">
    <div class="pair">
      <div class="card p-sm">
        <h3>主题</h3>
        <p class="hint">现在用的是哪一套配色。</p>
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
            {{ t.name }}
          </button>
        </div>
      </div>

      <div class="card p-sm">
        <h3>开机自启</h3>
        <p class="hint">开机登录后自动在后台待命，要用时点右下角托盘图标。</p>
        <div class="perm-list">
          <button
            class="perm"
            type="button"
            :class="{ 'is-on': autoStart }"
            @click="toggleAutoStart"
          >
            <b>{{ autoStart ? '已开启' : '未开启' }}</b>
            <span>{{ autoStart ? '开机后会自动在后台运行' : '需要你自己双击打开' }}</span>
          </button>
        </div>
        <p v-if="autoStartErr" class="hint err">{{ autoStartErr }}</p>
      </div>
    </div>

    <div class="card p-sm">
      <h3>执行方式</h3>
      <p class="hint">助手做多「危险」的事前要不要先问你。改了立刻生效。</p>
      <div class="perm-row">
        <button
          v-for="o in permissionOptions"
          :key="o.key"
          class="perm"
          :class="{ 'is-on': store.boot?.permission === o.key || store.values['permission'] === o.key }"
          type="button"
          @click="setPermission(o.key)"
        >
          <b>{{ o.name }}</b>
          <span>{{ o.desc }}</span>
        </button>
      </div>
    </div>

    <div class="card p-sm">
      <h3>关于</h3>
      <div class="kv">
        <dt>版本</dt>
        <dd class="mono">{{ store.boot?.version || '—' }}</dd>
        <dt>内置 Python</dt>
        <dd>
          <span class="led" :class="store.boot?.python_ready ? 'g' : 'r'" />
          {{ store.boot?.python_ready ? '可用' : '未就绪' }}
        </dd>
        <dt>工作目录</dt>
        <dd class="mono">{{ store.boot?.workspace || '—' }}</dd>
      </div>
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
/* 主题与开机自启等高并排；执行方式三档横排一行，杜绝空格与参差 */
.pair {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--wb-sp-4);
}
@media (max-width: 720px) {
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
  font-size: var(--wb-fs-md);
  color: var(--wb-ink);
}
.theme-card.is-on {
  border-color: var(--wb-primary);
  box-shadow: 0 0 0 3px var(--wb-primary-soft);
}
.sw {
  width: 40px;
  height: 40px;
  border-radius: var(--wb-radius-full);
  box-shadow: inset 0 0 0 1px var(--wb-tint-lg);
}
.hint {
  color: var(--wb-muted);
  font-size: var(--wb-fs-sm);
  margin: var(--wb-sp-2) 0 var(--wb-sp-3);
}
.hint.err {
  color: var(--wb-danger);
  margin-bottom: 0;
}
/* 三档并排：窄窗口再换行为竖排 */
.perm-row {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--wb-sp-2);
}
@media (max-width: 720px) {
  .perm-row {
    grid-template-columns: minmax(0, 1fr);
  }
}
.perm-list {
  display: grid;
  gap: var(--wb-sp-2);
}
.perm {
  text-align: left;
  display: grid;
  gap: 2px;
  padding: var(--wb-sp-3);
  border-radius: var(--wb-radius-sm);
  border: 1px solid var(--wb-border);
  background: var(--wb-surface);
  cursor: pointer;
  min-width: 0;
}
.perm.is-on {
  border-color: var(--wb-primary);
  background: var(--wb-primary-soft);
}
.perm b { font-size: var(--wb-fs-md); color: var(--wb-ink); }
.perm span { font-size: var(--wb-fs-sm); color: var(--wb-muted); }
</style>
