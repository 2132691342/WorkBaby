//! HTTP 信封：恒回 200，业务失败看 body.code；express 出口的统一形状。
import type { Request, Response, NextFunction, RequestHandler } from 'express'
import { AppError } from './apperror'
import { CODE_INTERNAL } from '../domain/errors'

/** 成功信封。数组类 data 永远给数组不给 null。 */
export function ok<T>(res: Response, data: T): void {
  res.json({ code: '0', message: '', data })
}

/** 失败信封。data 恒为 null，detail 带原文。 */
export function fail(res: Response, code: string, message: string, detail?: string): void {
  res.json({ code, message, detail: detail || undefined, data: null })
}

/**
 * 包一个路由处理器：同步或 async 抛出的错误统一进信封。
 * 业务错误用它的 code；未知错误归 1000 并把原话写日志。
 */
export function route(
  handler: (req: Request, res: Response) => Promise<void> | void,
): RequestHandler {
  return (req: Request, res: Response, next: NextFunction) => {
    void Promise.resolve(handler(req, res)).catch((err: unknown) => {
      if (err instanceof AppError) {
        fail(res, err.code, err.message, err.details)
        return
      }
      const raw = err instanceof Error ? err.stack ?? err.message : String(err)
      console.error('[unhandled]', raw)
      fail(res, CODE_INTERNAL, '内部错误', raw.slice(0, 2000))
      next()
    })
  }
}
