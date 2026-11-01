// 本轮耗时秒表：run 开始时起跳、结束时停表。
// 单独成文件是因为它有一份自己的定时器与清理时机，混在对话状态机里容易被漏掉 stop。
import { onScopeDispose, ref } from 'vue'

export function useRunClock() {
  /** run 开始时刻（毫秒）；0 = 还没开始 */
  const startedAt = ref(0)
  /** 已跑秒数：界面直接读它 */
  const elapsedSec = ref(0)

  let tickTimer = 0

  function stopTicker() {
    if (tickTimer) clearInterval(tickTimer)
    tickTimer = 0
  }

  function startTicker() {
    stopTicker()
    tickTimer = window.setInterval(() => {
      if (startedAt.value) elapsedSec.value = Math.round((Date.now() - startedAt.value) / 1000)
    }, 1000)
  }

  /** 从此刻起跳，秒数归零。 */
  function restart() {
    startedAt.value = Date.now()
    elapsedSec.value = 0
    startTicker()
  }

  /** 停表并保留最后读数（用户还要看这轮跑了多久）。 */
  function stop() {
    stopTicker()
  }

  // store 被销毁时定时器必须一起走，否则整页卸载后仍在跑
  onScopeDispose(stopTicker)

  return { startedAt, elapsedSec, restart, stop, startTicker, stopTicker }
}
