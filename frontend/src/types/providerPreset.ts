// 内置服务商预设类型。从 api.ts 拆出：它随服务商预设清单演进，
// 与消息 / 会话模型是两套生命周期；api.ts 末尾重新导出，调用方无感。

/** 内置服务商预设：首启一键添加的候选。 */
export interface ProviderPresetVO {
  /** 预设码（同时作为 provider 键） */
  preset_id: string
  preset_name: string
  /** 去哪儿申请密钥 / 适用场景 */
  preset_hint: string
  preset_base_url: string
  preset_model_count: number
  /** 配置里是否已存在 */
  preset_installed: boolean
}
