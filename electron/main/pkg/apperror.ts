//! 业务错误：code 是数字字符串，边界上统一转 HTTP 信封。

/** 业务错误。code 必须是错误码表里的数字字符串，message 给用户看，details 留原文。 */
export class AppError extends Error {
  /** 业务码（数字字符串） */
  code: string
  /** 人话描述 */
  message: string
  /** 补充上下文（stderr / 路径 / 上游响应原话） */
  details?: string

  constructor(code: string, message: string, details?: string) {
    super(message)
    this.name = 'AppError'
    this.code = code
    this.message = message
    this.details = details
  }
}

/** 造一个业务错误。 */
export function appError(code: string, message: string, details?: string): AppError {
  return new AppError(code, message, details)
}

/** 包一层底层错误：message 自己写，原话进 details。 */
export function wrapError(code: string, message: string, err: unknown): AppError {
  const raw = err instanceof Error ? err.message : String(err)
  return new AppError(code, message, raw)
}
