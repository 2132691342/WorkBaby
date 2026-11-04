//! HTTP 服务：express 装配、令牌鉴权、/api/v1 路由挂载与统一兜底。
import express, { type Express, type Request, type Response, type Router } from 'express'
import { token } from '../context'
import { fail } from '../pkg/resp'
import { CODE_INTERNAL } from '../domain/errors'
import { logWarn } from '../pkg/log'
import { attach } from '../sse/hub'
import { registerAppRoutes } from './routes_app'
import { registerEngineRoutes } from './routes_engine'
import { registerChatRoutes } from './routes_chat'
import { registerDataRoutes } from './routes_data'
import { registerResRoutes } from './routes_res'

/** 令牌校验：HTTP 走头，SSE 走查询参数（EventSource 带不了自定义头）。 */
function auth(req: Request, res: Response, next: () => void): void {
  const given = String(req.headers['x-desktop-token'] ?? req.query.token ?? '')
  if (given !== token()) {
    logWarn(`未授权请求被拒：${req.method} ${req.path}`)
    res.status(401).json({ code: '1401', message: '令牌无效', data: null })
    return
  }
  next()
}

/** 组装 express 应用；全部业务路由挂在 /api/v1 下。 */
export function buildServer(): Express {
  const app = express()
  app.disable('x-powered-by')
  // 图片走 base64 进 body，给足限额。
  app.use(express.json({ limit: '64mb' }))

  const api = express.Router()
  api.use(auth)
  registerAppRoutes(api)
  registerEngineRoutes(api)
  registerChatRoutes(api)
  registerDataRoutes(api)
  registerResRoutes(api)
  api.get('/events', (req, res) => {
    attach(res, Number(req.query.since ?? 0))
  })
  app.use('/api/v1', api)

  app.use((_req: Request, res: Response) => {
    fail(res, '1404', '接口不存在')
  })

  // 兜底：路由层抛出的同步异常也不能裸崩。
  app.use((err: unknown, _req: Request, res: Response, _next: unknown) => {
    const raw = err instanceof Error ? err.stack ?? err.message : String(err)
    fail(res, CODE_INTERNAL, '内部错误', raw.slice(0, 2000))
  })

  return app
}

/** 起服务：随机端口，返回实际端口。 */
export function startServer(app: Express): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = app.listen(0, '127.0.0.1')
    server.once('listening', () => {
      const addr = server.address()
      if (addr && typeof addr === 'object') resolve(addr.port)
      else reject(new Error('端口分配失败'))
    })
    server.once('error', reject)
  })
}
