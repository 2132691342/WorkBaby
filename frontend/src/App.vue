<script setup lang="ts">
// 应用外壳：自绘标题栏 + 左侧导航 rail + 主区。
// 启动失败与首屏加载都在这里处理，视图层不需要各自兜底。
import { computed, nextTick, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import zhCN from 'element-plus/es/locale/lang/zh-cn'
import enUS from 'element-plus/es/locale/lang/en'
import AppRail from '@/components/layout/AppRail.vue'
import TitleBar from '@/components/layout/TitleBar.vue'
import ToastHost from '@/components/common/ToastHost.vue'
import StartupError from '@/components/common/StartupError.vue'
import { applyCloseDecision, notifyReady } from '@/api/wails'
import { useAppStore } from '@/stores/app'
import { useViewportScale } from '@/composables/useViewportScale'

const app = useAppStore()
const { t } = useI18n()

// Element Plus 自带文案（空数据、ARIA 标签等）跟着界面语言走：
// 不接桥的话英文界面里混着组件库的英文默认值之外的另一套中文。
const epLocale = computed(() => (app.settings['ui.locale'] === 'en' ? enUS : zhCN))

// 首次按关闭时问一句：收进托盘还是退出。
// 在界面里问而不是窗口层：窗口层没有可用通知手段（窗口已要藏），
// 托盘气泡这个版本的 Wails 不支持。问一次并记住选择。
async function askClose(): Promise<'tray' | 'quit' | ''> {
  try {
    await ElMessageBox.confirm(t('app.closePromptBody'), t('app.closePromptTitle'), {
      confirmButtonText: t('app.closeToTray'),
      cancelButtonText: t('app.closeToQuit'),
      distinguishCancelAndClose: true,
      type: 'info',
    })
    return 'tray'
  } catch (action) {
    // cancel = 点了“退出”；close = 按了 Esc / 点了右上角叉
    return action === 'cancel' ? 'quit' : ''
  }
}

/** 关闭对话框：用户要求关窗时问一次，然后把选择交给后端执行。 */
async function settleClose(): Promise<void> {
  const action = await askClose()
  app.closePrompt = false
  // 按 Esc 或点叉也是“我要关窗”的意思：按最保守的处理——收进托盘。
  // 留着窗口不动是最差的答案：按钮看起来就是失灵了。
  // 这一种不记住：用户没真正选择，下次还该问。
  await applyCloseDecision(action || 'tray', Boolean(action))
}

// 整套刻度随窗口宽度等比缩放：写进 --wb-vw-scale 一个值就够，
// 组件全走令牌，不必为每个断点写媒体查询。
useViewportScale()

// 后端发出「用户按了关闭」时弹选择框。
watch(
  () => app.closePrompt,
  (open) => {
    if (open) void settleClose()
  },
)

onMounted(() => {
  void app.load()
  // 外壳一画完就把窗口叫出来：用户先看到骨架与转圈，而不是迟迟没有窗口。
  // 不能更早——后端装配阶段窗口还是隐藏的，那时显形会白屏。
  void nextTick().then(notifyReady)
})
</script>

<template>
  <el-config-provider :locale="epLocale">
    <div class="shell">
      <TitleBar />
      <div class="shell-main">
        <AppRail v-if="app.ready" />
        <main class="shell-view">
          <div v-if="app.fatal" class="shell-fallback">
            <StartupError :message="app.fatal" />
          </div>
          <div v-else-if="!app.ready" class="shell-fallback">
            <div class="boot">
              <div class="boot-spinner" />
              <p class="boot-text">{{ t('app.starting') }}</p>
            </div>
          </div>
          <router-view v-else v-slot="{ Component }">
            <transition name="wb-fade" mode="out-in">
              <component :is="Component" />
            </transition>
          </router-view>
        </main>
      </div>
      <ToastHost />
    </div>
  </el-config-provider>
</template>

<style scoped>
.shell {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--wb-window);
}

.shell-main {
  display: flex;
  flex: 1;
  min-height: 0;
  /* 给子视图一个容器查询的锚点：窄窗口下视图按容器宽度收窄列宽，
     比媒体查询准确（窗口是最外层容器，两者等效，但换布局时不会失配）。 */
  container-type: inline-size;
  container-name: app;
}

.shell-view {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: var(--wb-work);
}

.shell-fallback {
  flex: 1;
  display: grid;
  place-items: center;
  padding: var(--wb-sp-8);
}

.boot {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wb-sp-4);
}

.boot-spinner {
  width: 22px;
  height: 22px;
  border: 2px solid var(--wb-line-strong);
  border-top-color: var(--wb-brand);
  border-radius: 50%;
  animation: boot-spin 720ms linear infinite;
}

@keyframes boot-spin {
  to {
    transform: rotate(360deg);
  }
}

.boot-text {
  margin: 0;
  font-size: var(--wb-fs-sm);
  color: var(--wb-mute);
}
</style>
