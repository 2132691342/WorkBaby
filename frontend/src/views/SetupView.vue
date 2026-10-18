<!-- 环境向导：装 Node / 装 PI / 配模型三段式。
     bootstrap.pi_env.ready=false 时强制进这里——没有 PI 就没有对话，
     留一个可以待着的空壳只会让用户以为程序坏了。 -->
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import * as api from '../api'
import { useSetupLog } from '../composables/useSetupLog'
import { useSettingsStore } from '../stores/settings'
import { useToastStore } from '../stores/toast'
import type { PiEnvVO } from '../types/api'
import AppIcon from '../components/common/AppIcon.vue'
import ProviderPanel from '../components/settings/ProviderPanel.vue'

const router = useRouter()
const settings = useSettingsStore()
const toast = useToastStore()

const env = ref<PiEnvVO | null>(null)
const loading = ref(true)
const loadErr = ref('')
const installing = ref(false)
// 安装日志：npm 可能跑几十秒，没有实时输出用户会以为卡死。
// 输出经 SSE 的全局通道回传（安装发生在任何会话之前）。
const { lines: installLines, begin: beginInstallLog, end: endInstallLog } = useSetupLog()
const installLog = computed(() => installLines.value)

const steps = computed(() => {
  const e = env.value
  return [
    {
      key: 'node',
      title: '运行环境',
      desc: '助手引擎跑在 Node 上，已经随程序自带',
      done: !!e?.node_ready,
      detail: e?.node_ready ? `Node ${e.node_version}` : e?.node_error || '正在准备',
    },
    {
      key: 'pi',
      title: '助手引擎',
      desc: 'PI 是真正干活的引擎，装一次就好',
      done: !!e?.pi_ready,
      detail: e?.pi_ready ? `版本 ${e.pi_version || '未知'}` : e?.pi_error || '还没安装',
    },
    {
      key: 'model',
      title: '模型服务',
      desc: '至少配一个能用的模型服务，助手才能思考',
      done: !!e?.model_ready,
      detail: e?.model_ready ? '已配置' : '还没配置',
    },
  ]
})

const blocking = computed(() => env.value?.blocking || '')
const allDone = computed(() => !!env.value?.ready)

async function load() {
  loading.value = true
  loadErr.value = ''
  try {
    env.value = await api.env.status()
  } catch (e) {
    loadErr.value = (e as Error)?.message || '读取环境失败'
  } finally {
    loading.value = false
  }
}

async function install() {
  if (installing.value) return
  installing.value = true
  beginInstallLog()
  try {
    await api.env.install()
    // 安装是后台动作：接口立刻返回，进度靠轮询 env + 日志轮询拉回来。
    // 没有 SSE 通道时也要能走完，所以这里用轮询而不是等事件。
    const deadline = Date.now() + 5 * 60 * 1000
    while (Date.now() < deadline) {
      await new Promise((r) => setTimeout(r, 1500))
      env.value = await api.env.recheck()
      if (env.value?.pi_ready) break
    }
    if (env.value?.pi_ready) {
      toast.ok('助手引擎已就绪')
      void settings.loadBoot()
    } else {
      toast.bad('安装好像没成功，看一下下面的日志')
    }
  } catch (e) {
    toast.bad(`安装失败：${(e as Error)?.message || '请重试'}`)
  } finally {
    installing.value = false
    endInstallLog()
  }
}

// 配好模型后重新判定一次闸门，三项全绿才放行进对话页。
async function afterProviderSaved() {
  try {
    env.value = await api.env.recheck()
    void settings.loadBoot()
  } catch {
    /* 重检失败不阻断：用户可以直接点「重新检查」 */
  }
}

function enter() {
  if (!allDone.value) return
  void router.replace('/')
}

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="setup">
    <div class="wrap">
      <header class="head">
        <div class="tile"><AppIcon name="sparkles" size="ic-lg" /></div>
        <div>
          <h1>先把环境准备好</h1>
          <p class="sub">三步走完就能开始干活，之后不再需要这些设置。</p>
        </div>
      </header>

      <div v-if="loading" class="wb-skel" />
      <div v-else-if="loadErr" class="wb-hint err">
        <AppIcon name="alert" />
        <span>{{ loadErr }}</span>
        <button class="btn btn-sm btn-ghost" @click="load">重试</button>
      </div>

      <template v-else>
        <ol class="steps">
          <li v-for="(s, i) in steps" :key="s.key" class="step" :class="{ on: s.done, active: blocking === s.key }">
            <div class="num">
              <AppIcon v-if="s.done" name="check" size="ic-sm" />
              <span v-else>{{ i + 1 }}</span>
            </div>
            <div class="body">
              <div class="row">
                <h3>{{ s.title }}</h3>
                <span class="state">{{ s.done ? '就绪' : blocking === s.key ? '需要处理' : '等待' }}</span>
              </div>
              <p class="desc">{{ s.desc }}</p>
              <p class="det">{{ s.detail }}</p>
              <button
                v-if="s.key === 'pi' && !s.done"
                class="btn btn-sm btn-primary"
                :disabled="installing"
                :class="{ 'is-loading': installing }"
                @click="install"
              >
                {{ installing ? '正在安装…' : '安装助手引擎' }}
              </button>
            </div>
          </li>
        </ol>

        <div v-if="installLog.length" class="log">
          <div v-for="(line, i) in installLog" :key="i" class="line">{{ line }}</div>
        </div>

        <!-- 模型服务复用设置页的同一个组件，不复制一份表单 -->
        <section v-if="blocking === 'model' || !allDone" class="card p-md">
          <h3>配一个模型服务</h3>
          <p class="hint">填服务商地址与密钥，再挑一个模型。填完会自动重新检查。</p>
          <ProviderPanel @saved="afterProviderSaved" />
        </section>

        <footer class="foot">
          <button class="btn btn-ghost" @click="load">重新检查</button>
          <button class="btn btn-primary" :disabled="!allDone" @click="enter">开始使用</button>
        </footer>
      </template>
    </div>
  </div>
</template>

<style scoped>
.setup {
  height: 100%;
  overflow: auto;
  background: var(--wb-bg);
}
.wrap {
  max-width: 720px;
  margin: 0 auto;
  padding: 40px 20px 60px;
  display: flex;
  flex-direction: column;
  gap: 20px;
}
.head {
  display: flex;
  align-items: center;
  gap: 14px;
}
.tile {
  width: var(--wb-tile-lg);
  height: var(--wb-tile-lg);
  border-radius: 14px;
  display: grid;
  place-items: center;
  background: var(--wb-brand-3);
  color: var(--wb-primary);
  flex: none;
}
h1 {
  margin: 0;
  font-size: var(--wb-fs-xl);
  color: var(--wb-text);
}
.sub {
  margin: 4px 0 0;
  color: var(--wb-text-muted);
  font-size: var(--wb-fs-sm);
}
.steps {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.step {
  display: flex;
  gap: 14px;
  padding: 14px 16px;
  border-radius: 12px;
  border: 1px solid var(--wb-border);
  background: var(--wb-surface);
  transition: border-color 0.16s ease, background 0.16s ease;
}
.step.on {
  border-color: var(--wb-primary);
  background: var(--wb-brand-4);
}
.step.active {
  border-color: var(--wb-brand-2);
}
.num {
  width: var(--wb-ctl-icon);
  height: var(--wb-ctl-icon);
  border-radius: 50%;
  display: grid;
  place-items: center;
  flex: none;
  font-size: var(--wb-fs-sm);
  font-weight: 700;
  background: var(--wb-brand-3);
  color: var(--wb-primary);
}
.step.on .num {
  background: var(--wb-primary);
  color: #fff;
}
.body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
h3 {
  margin: 0;
  font-size: var(--wb-fs-md);
  color: var(--wb-text);
}
.state {
  font-size: var(--wb-fs-xs);
  color: var(--wb-text-muted);
}
.step.on .state {
  color: var(--wb-primary);
}
.desc {
  margin: 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-text-muted);
}
.det {
  margin: 0;
  font-size: var(--wb-fs-xs);
  color: var(--wb-text-faint);
  word-break: break-all;
}
.log {
  max-height: 180px;
  overflow: auto;
  padding: 10px 12px;
  border-radius: 10px;
  background: var(--wb-brand-5);
  color: #e2e8f0;
  font-family: var(--font-mono);
  font-size: var(--wb-fs-xs);
  line-height: 1.6;
}
.foot {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}
</style>
