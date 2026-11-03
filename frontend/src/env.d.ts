/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<Record<string, unknown>, Record<string, unknown>, unknown>
  export default component
}

/** preload 注入的壳桥；字段契约见 electron/preload/index.ts。 */
interface WbBridgeGlobal {
  getEndpoint(): Promise<{
    wb_port: number
    wb_token: string
    wb_version: string
    wb_contract: number
    wb_base_url: string
  }>
  pickFiles(title: string): Promise<string[]>
  pickFolder(defaultDir: string): Promise<string>
  notifyReady(): Promise<void>
  openExternal(url: string): Promise<void>
  windowCtl: {
    minimize(): Promise<void>
    toggleMaximize(): Promise<void>
    close(): Promise<void>
    quit(): Promise<void>
  }
  applyCloseDecision(action: 'tray' | 'quit', remember: boolean): Promise<void>
}

declare global {
  interface Window {
    wb: WbBridgeGlobal
  }
}

export {}
