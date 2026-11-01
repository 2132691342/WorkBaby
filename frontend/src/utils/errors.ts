// 错误的界面文案：后端只给业务码与细节，人话在这里落。
// 优先级：本语言包按码查到的通用说明 + 后端原文（原文带的是「为什么」），
// 两者都有时拼起来，只有原文时用原文——用户永远不至于只看到一句「出错了」。
import { i18n } from '../i18n'
import { ApiError } from '../api/http'

/** 业务码 → 语言包前缀。未收录的码回落到通用的「出错了」。 */
const CODE_GROUPS: Record<string, string> = {
  '1001': 'errCode.paramInvalid',
  '1002': 'errCode.ioFailed',
  '1003': 'errCode.pathEscape',
  '1004': 'errCode.pathMissing',
  '1017': 'errCode.gitFailed',
  '1020': 'errCode.gitCommitFailed',
  '1021': 'errCode.gitConflict',
  '1024': 'errCode.gitDiverged',
  '1030': 'errCode.archiveInvalid',
  '2001': 'errCode.configInvalid',
  '2002': 'errCode.dbFailed',
  '2004': 'errCode.dataDirReadonly',
  '3001': 'errCode.providerNotFound',
  '3002': 'errCode.modelNotFound',
  '3003': 'errCode.keyMissing',
  '3005': 'errCode.baseUrlInvalid',
  '3109': 'errCode.validateFailed',
  '3111': 'errCode.nameDup',
  '3200': 'errCode.testFailed',
  '5001': 'errCode.busy',
  '5002': 'errCode.sessionNotFound',
  '5003': 'errCode.fileBusy',
  '5004': 'errCode.queueFull',
  '5005': 'errCode.askExpired',
  '5006': 'errCode.compactFailed',
  '5100': 'errCode.notReady',
  '7001': 'errCode.execTimeout',
  '7002': 'errCode.execCancelled',
  '7003': 'errCode.notAvailable',
  '7004': 'errCode.argInvalid',
  '7100': 'errCode.lspStartFailed',
  '7101': 'errCode.lspServerError',
  '7102': 'errCode.lspNotInstalled',
  '7103': 'errCode.lspNoMatch',
  '7301': 'errCode.memoryNotFound',
  '8001': 'errCode.skillParseFailed',
  '8002': 'errCode.skillDescInvalid',
  '8003': 'errCode.skillNameInvalid',
  '8004': 'errCode.skillNameDup',
  '8005': 'errCode.skillBuiltinLocked',
  '9101': 'errCode.mcpConfigInvalid',
  '9102': 'errCode.mcpStartFailed',
  '9103': 'errCode.mcpCallFailed',
  '9701': 'errCode.sessionFileCorrupt',
  '9703': 'errCode.streamBroken',
  '9704': 'errCode.subagentFailed',
}

/**
 * 把任意异常翻成一句可读的界面文案。
 *
 * 后端 `code` 决定「这是什么问题」，后端 `message` 决定「具体卡在哪」，
 * 两者都不是界面语言的权威——语言由前端当前 locale 决定。
 */
export function errorText(e: unknown): string {
  const t = i18n.global.t
  // 无错误对象时返回空串：调用方自己的兵底句还等着被用上
  if (e === null || e === undefined) return ''
  if (e instanceof ApiError) {
    const key = CODE_GROUPS[e.code]
    if (!key) return e.message
    const headline = t(key)
    const detail = (e.message || '').trim()
    if (!detail || detail === headline) return headline
    // 后端原文通常是中文的细节（路径 / stderr 摘要）：英文界面下留原文比丢掉更有用
    return `${headline}：${detail}`
  }
  if (e instanceof Error) return e.message
  return String(e)
}

/** 同上，但允许调用方给一句自己的兜底（失败上下文不同，兜底句也不同）。 */
export function errorTextOr(e: unknown, fallbackKey: string): string {
  const msg = errorText(e)
  return msg || i18n.global.t(fallbackKey)
}
