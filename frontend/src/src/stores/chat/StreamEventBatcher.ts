import { decodeStreamEvent, type StreamEventUpdate } from './ChatStreamDecoder'
import type { ChatStreamEvent } from '@/types/api'

/**
 * 流式事件批处理器。
 *
 * <p>把流式期间高频到达的 ChatStreamEvent 攒到一帧结束统一写 ref，避免每条增量触发 Vue 重渲染。
 * 终态与交互类事件（{@code tool_approval_request / error / stopped / stats}）立即 flush，避免「点了停止还在动」。
 * 调度用 rAF + 定时器双兜底：rAF 在窗口不可见时被浏览器暂停，定时器保证后台也能推进。
 */
export class StreamEventBatcher {

  /** 定时器兜底延迟（≈2 帧；rAF 未触发时保证更新不会停滞）。 */
  private static readonly FALLBACK_DELAY_MS = 32;

  /** 必须立即生效、不走批处理延迟的事件类型。 */
  private static readonly IMMEDIATE_TYPES = new Set([
    'tool_approval_request',
    'error',
    'stopped',
    'stats'
  ]);

  /** 一帧内累积、尚未写入 ref 的 update。 */
  private queue: StreamEventUpdate[] = [];

  private rafHandle: number | null = null;
  private timerHandle: ReturnType<typeof setTimeout> | null = null;

  /**
   * @param apply 批量写入回调（由 store 传入，内部调 {@code applyStreamUpdate}）
   */
  constructor(private readonly apply: (updates: StreamEventUpdate[]) => void) {}

  /**
   * 收一帧事件：解码后按需入队或立即落定。
   *
   * <p>解码是纯函数，这里同步执行，不影响批处理效果（它不碰 ref）。
   */
  push(event: ChatStreamEvent): void {
    const update = decodeStreamEvent(event)
    if (!update) {
      return
    }
    if (StreamEventBatcher.IMMEDIATE_TYPES.has(event.type)) {
      // 先把积压的增量落定，保证「先增量后终态」的顺序不被打乱
      this.flush()
      this.apply([update])
      return
    }
    this.queue.push(update)
    this.schedule()
  }

  /** 把积压的 update 一次性写入；无积压时是 no-op。 */
  flush(): void {
    this.cancelSchedule()
    if (this.queue.length === 0) {
      return
    }
    const batch = this.queue
    this.queue = []
    this.apply(batch)
  }

  /** 丢弃未落定的 update 并取消调度（组件卸载 / 会话切换时用）。 */
  dispose(): void {
    this.cancelSchedule()
    this.queue = []
  }

  /** 调度一次 flush：rAF 优先 + 定时器兜底。 */
  private schedule(): void {
    if (this.rafHandle !== null || this.timerHandle !== null) {
      return // 已有 pending 调度
    }
    if (typeof requestAnimationFrame === 'function') {
      this.rafHandle = requestAnimationFrame(() => this.flush())
    }
    this.timerHandle = setTimeout(() => this.flush(), StreamEventBatcher.FALLBACK_DELAY_MS)
  }

  private cancelSchedule(): void {
    if (this.rafHandle !== null) {
      cancelAnimationFrame(this.rafHandle)
      this.rafHandle = null
    }
    if (this.timerHandle !== null) {
      clearTimeout(this.timerHandle)
      this.timerHandle = null
    }
  }
}
