// 联网与预索引的契约类型。字段名与 Go 侧的 json tag 一一对应。

/** 联网设置与状态。 */
export interface WebStatusView {
  web_kind: string
  web_has_key: boolean
  web_message: string
  web_kinds: string[]
}

/** 一条搜索结果。 */
export interface WebSearchItem {
  result_title: string
  result_url: string
  result_snippet: string
  result_source?: string
}

/** 搜索结果清单。 */
export interface WebSearchView {
  search_results: WebSearchItem[]
  search_provider: string
}

/** 抓取结果。 */
export interface WebFetchView {
  fetch_url: string
  fetch_text: string
  fetch_char_count: number
}

/** 索引状态。 */
export interface IndexStatusView {
  index_workspace: string
  index_doc_count: number
  index_built: boolean
  index_building: boolean
  index_built_at: number
}

/** 一条检索命中。 */
export interface IndexHitView {
  doc_path: string
  doc_kind: string
  doc_symbol?: string
  doc_symbol_kind?: string
  doc_line?: number
  hit_score: number
  hit_reason?: string
}

/** 检索结果与索引状态。 */
export interface IndexSearchView {
  search_hits: IndexHitView[]
  index_status: IndexStatusView
}
