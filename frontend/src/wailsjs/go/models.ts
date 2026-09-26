export namespace agent {
	
	export class ExecutionRegistry {
	
	
	    static createFrom(source: any = {}) {
	        return new ExecutionRegistry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}

}

export namespace domain {
	
	export class AdminOverviewRESP {
	    version: string;
	    phase: string;
	    home: string;
	    db_enabled: boolean;
	    tool_count: number;
	    providers: number;
	    sessions: number;
	    messages: number;
	    memory_episodes: number;
	    knowledge_docs: number;
	
	    static createFrom(source: any = {}) {
	        return new AdminOverviewRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.phase = source["phase"];
	        this.home = source["home"];
	        this.db_enabled = source["db_enabled"];
	        this.tool_count = source["tool_count"];
	        this.providers = source["providers"];
	        this.sessions = source["sessions"];
	        this.messages = source["messages"];
	        this.memory_episodes = source["memory_episodes"];
	        this.knowledge_docs = source["knowledge_docs"];
	    }
	}
	export class AgentProfileREQ {
	    name: string;
	    description: string;
	    system_prompt: string;
	    tools_allow: string[];
	    tools_deny: string[];
	    memory_enable: boolean;
	    max_turns: number;
	    model: string;
	    thinking: string;
	    enabled?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AgentProfileREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.system_prompt = source["system_prompt"];
	        this.tools_allow = source["tools_allow"];
	        this.tools_deny = source["tools_deny"];
	        this.memory_enable = source["memory_enable"];
	        this.max_turns = source["max_turns"];
	        this.model = source["model"];
	        this.thinking = source["thinking"];
	        this.enabled = source["enabled"];
	    }
	}
	export class AgentProfileRESP {
	    id: string;
	    name: string;
	    description: string;
	    system_prompt?: string;
	    tools_allow?: string[];
	    tools_deny?: string[];
	    memory_enable: boolean;
	    max_turns: number;
	    model?: string;
	    thinking?: string;
	    enabled: boolean;
	    created_at: number;
	    updated_at: number;
	
	    static createFrom(source: any = {}) {
	        return new AgentProfileRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.system_prompt = source["system_prompt"];
	        this.tools_allow = source["tools_allow"];
	        this.tools_deny = source["tools_deny"];
	        this.memory_enable = source["memory_enable"];
	        this.max_turns = source["max_turns"];
	        this.model = source["model"];
	        this.thinking = source["thinking"];
	        this.enabled = source["enabled"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class AiProviderREQ {
	    name: string;
	    kind: string;
	    api_key?: string;
	    base_url: string;
	    model: string;
	    alias: string;
	    tier: string;
	    enabled?: boolean;
	    context_window: number;
	    max_output_tokens: number;
	    compress_ratio: number;
	    temperature: number;
	    top_p: number;
	    thinking_effort: string;
	    thinking_style: string;
	    supports_tool_call?: boolean;
	    supports_vision?: boolean;
	    supports_reasoning?: boolean;
	    capabilities_json?: string;
	    pricing_json?: string;
	
	    static createFrom(source: any = {}) {
	        return new AiProviderREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.api_key = source["api_key"];
	        this.base_url = source["base_url"];
	        this.model = source["model"];
	        this.alias = source["alias"];
	        this.tier = source["tier"];
	        this.enabled = source["enabled"];
	        this.context_window = source["context_window"];
	        this.max_output_tokens = source["max_output_tokens"];
	        this.compress_ratio = source["compress_ratio"];
	        this.temperature = source["temperature"];
	        this.top_p = source["top_p"];
	        this.thinking_effort = source["thinking_effort"];
	        this.thinking_style = source["thinking_style"];
	        this.supports_tool_call = source["supports_tool_call"];
	        this.supports_vision = source["supports_vision"];
	        this.supports_reasoning = source["supports_reasoning"];
	        this.capabilities_json = source["capabilities_json"];
	        this.pricing_json = source["pricing_json"];
	    }
	}
	export class AiProviderRESP {
	    id: string;
	    name: string;
	    kind: string;
	    api_key_masked: string;
	    base_url: string;
	    model: string;
	    alias: string;
	    tier: string;
	    enabled: boolean;
	    context_window: number;
	    max_output_tokens: number;
	    compress_ratio: number;
	    temperature: number;
	    top_p: number;
	    thinking_effort: string;
	    thinking_style: string;
	    thinking_style_resolved: string;
	    supports_tool_call?: boolean;
	    supports_vision?: boolean;
	    supports_reasoning?: boolean;
	    tool_call_effective: boolean;
	    vision_effective: boolean;
	    reasoning_effective: boolean;
	    capabilities_json: string;
	    pricing_json: string;
	    created_at: number;
	    updated_at: number;
	
	    static createFrom(source: any = {}) {
	        return new AiProviderRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.api_key_masked = source["api_key_masked"];
	        this.base_url = source["base_url"];
	        this.model = source["model"];
	        this.alias = source["alias"];
	        this.tier = source["tier"];
	        this.enabled = source["enabled"];
	        this.context_window = source["context_window"];
	        this.max_output_tokens = source["max_output_tokens"];
	        this.compress_ratio = source["compress_ratio"];
	        this.temperature = source["temperature"];
	        this.top_p = source["top_p"];
	        this.thinking_effort = source["thinking_effort"];
	        this.thinking_style = source["thinking_style"];
	        this.thinking_style_resolved = source["thinking_style_resolved"];
	        this.supports_tool_call = source["supports_tool_call"];
	        this.supports_vision = source["supports_vision"];
	        this.supports_reasoning = source["supports_reasoning"];
	        this.tool_call_effective = source["tool_call_effective"];
	        this.vision_effective = source["vision_effective"];
	        this.reasoning_effective = source["reasoning_effective"];
	        this.capabilities_json = source["capabilities_json"];
	        this.pricing_json = source["pricing_json"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class AnswerInputREQ {
	    answer: string;
	
	    static createFrom(source: any = {}) {
	        return new AnswerInputREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.answer = source["answer"];
	    }
	}
	export class ApprovalGrantRESP {
	    id: string;
	    command: string;
	    risk: string;
	    created_at: number;
	
	    static createFrom(source: any = {}) {
	        return new ApprovalGrantRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.command = source["command"];
	        this.risk = source["risk"];
	        this.created_at = source["created_at"];
	    }
	}
	export class ApprovalPendingRESP {
	    id: string;
	    run_id: string;
	    session_id: string;
	    command: string;
	    reason: string;
	    risk: string;
	    expires_at: number;
	    can_remember: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ApprovalPendingRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.run_id = source["run_id"];
	        this.session_id = source["session_id"];
	        this.command = source["command"];
	        this.reason = source["reason"];
	        this.risk = source["risk"];
	        this.expires_at = source["expires_at"];
	        this.can_remember = source["can_remember"];
	    }
	}
	export class ArtifactRESP {
	    id: string;
	    session_id: string;
	    run_id: string;
	    kind: string;
	    name: string;
	    rel_path: string;
	    mime_type: string;
	    size: number;
	    created_at: number;
	    updated_at: number;
	
	    static createFrom(source: any = {}) {
	        return new ArtifactRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.session_id = source["session_id"];
	        this.run_id = source["run_id"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.rel_path = source["rel_path"];
	        this.mime_type = source["mime_type"];
	        this.size = source["size"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class ArtifactListRESP {
	    items: ArtifactRESP[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new ArtifactListRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], ArtifactRESP);
	        this.total = source["total"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class AvailableModelRESP {
	    id: string;
	    name: string;
	    kind: string;
	    model: string;
	    alias: string;
	    tier: string;
	    enabled: boolean;
	    context_window: number;
	    max_output_tokens: number;
	    compress_ratio: number;
	    temperature: number;
	    thinking_effort: string;
	    thinking_style: string;
	    tool_call: boolean;
	    vision: boolean;
	    reasoning: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AvailableModelRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.model = source["model"];
	        this.alias = source["alias"];
	        this.tier = source["tier"];
	        this.enabled = source["enabled"];
	        this.context_window = source["context_window"];
	        this.max_output_tokens = source["max_output_tokens"];
	        this.compress_ratio = source["compress_ratio"];
	        this.temperature = source["temperature"];
	        this.thinking_effort = source["thinking_effort"];
	        this.thinking_style = source["thinking_style"];
	        this.tool_call = source["tool_call"];
	        this.vision = source["vision"];
	        this.reasoning = source["reasoning"];
	    }
	}
	export class BatchDeleteResult {
	    ok: string[];
	    failed: string[];
	
	    static createFrom(source: any = {}) {
	        return new BatchDeleteResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.failed = source["failed"];
	    }
	}
	export class ChatSessionModelREQ {
	    provider_id: string;
	    model: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatSessionModelREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider_id = source["provider_id"];
	        this.model = source["model"];
	    }
	}
	export class ChatSessionPermissionREQ {
	    mode: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatSessionPermissionREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	    }
	}
	export class ChatSessionREQ {
	    name: string;
	    provider_id: string;
	    model: string;
	    workspace_id: string;
	    workspace_path: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatSessionREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.provider_id = source["provider_id"];
	        this.model = source["model"];
	        this.workspace_id = source["workspace_id"];
	        this.workspace_path = source["workspace_path"];
	    }
	}
	export class ChatSessionRESP {
	    id: string;
	    name: string;
	    user_id: string;
	    provider_id: string;
	    model: string;
	    workspace_id: string;
	    workspace_path: string;
	    active: boolean;
	    message_count: number;
	    last_message_at: number;
	    metadata_json: string;
	    status: string;
	    kind: string;
	    pinned: boolean;
	    permission_mode: string;
	    parent_id: string;
	    branch_point: number;
	    created_at: number;
	    updated_at: number;
	
	    static createFrom(source: any = {}) {
	        return new ChatSessionRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.user_id = source["user_id"];
	        this.provider_id = source["provider_id"];
	        this.model = source["model"];
	        this.workspace_id = source["workspace_id"];
	        this.workspace_path = source["workspace_path"];
	        this.active = source["active"];
	        this.message_count = source["message_count"];
	        this.last_message_at = source["last_message_at"];
	        this.metadata_json = source["metadata_json"];
	        this.status = source["status"];
	        this.kind = source["kind"];
	        this.pinned = source["pinned"];
	        this.permission_mode = source["permission_mode"];
	        this.parent_id = source["parent_id"];
	        this.branch_point = source["branch_point"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class ChatTaskDO {
	    id: string;
	    session_id: string;
	    agent: string;
	    prompt: string;
	    state: string;
	    run_id: string;
	    result: string;
	    error: string;
	    created_at: number;
	    started_at: number;
	    finished_at: number;
	
	    static createFrom(source: any = {}) {
	        return new ChatTaskDO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.session_id = source["session_id"];
	        this.agent = source["agent"];
	        this.prompt = source["prompt"];
	        this.state = source["state"];
	        this.run_id = source["run_id"];
	        this.result = source["result"];
	        this.error = source["error"];
	        this.created_at = source["created_at"];
	        this.started_at = source["started_at"];
	        this.finished_at = source["finished_at"];
	    }
	}
	export class ChatTaskListRESP {
	    items: ChatTaskDO[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new ChatTaskListRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], ChatTaskDO);
	        this.total = source["total"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ChatTaskREQ {
	    session_id: string;
	    agent: string;
	    prompt: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatTaskREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session_id = source["session_id"];
	        this.agent = source["agent"];
	        this.prompt = source["prompt"];
	    }
	}
	export class SlashCommand {
	    name: string;
	    args: string;
	    desc: string;
	    group: string;
	    client_only: boolean;
	    prompt?: string;
	    source?: string;
	
	    static createFrom(source: any = {}) {
	        return new SlashCommand(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.args = source["args"];
	        this.desc = source["desc"];
	        this.group = source["group"];
	        this.client_only = source["client_only"];
	        this.prompt = source["prompt"];
	        this.source = source["source"];
	    }
	}
	export class CommandListRESP {
	    items: SlashCommand[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new CommandListRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], SlashCommand);
	        this.total = source["total"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CompactREQ {
	    instructions?: string;
	    keep_recent?: number;
	
	    static createFrom(source: any = {}) {
	        return new CompactREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.instructions = source["instructions"];
	        this.keep_recent = source["keep_recent"];
	    }
	}
	export class CompactResultRESP {
	    session_id: string;
	    compacted: number;
	    freed_chars: number;
	    kept_recent: number;
	    total_before: number;
	    pinned: boolean;
	    freed_tokens: number;
	    failed: number;
	
	    static createFrom(source: any = {}) {
	        return new CompactResultRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session_id = source["session_id"];
	        this.compacted = source["compacted"];
	        this.freed_chars = source["freed_chars"];
	        this.kept_recent = source["kept_recent"];
	        this.total_before = source["total_before"];
	        this.pinned = source["pinned"];
	        this.freed_tokens = source["freed_tokens"];
	        this.failed = source["failed"];
	    }
	}
	export class ContextSegment {
	    key: string;
	    title: string;
	    tokens: number;
	    ratio: number;
	
	    static createFrom(source: any = {}) {
	        return new ContextSegment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.title = source["title"];
	        this.tokens = source["tokens"];
	        this.ratio = source["ratio"];
	    }
	}
	export class ContextUsageRESP {
	    session_id: string;
	    model: string;
	    context_window: number;
	    context_budget: number;
	    used_tokens: number;
	    free_tokens: number;
	    used_ratio: number;
	    segments: ContextSegment[];
	    message_count: number;
	    tool_count: number;
	    estimated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ContextUsageRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session_id = source["session_id"];
	        this.model = source["model"];
	        this.context_window = source["context_window"];
	        this.context_budget = source["context_budget"];
	        this.used_tokens = source["used_tokens"];
	        this.free_tokens = source["free_tokens"];
	        this.used_ratio = source["used_ratio"];
	        this.segments = this.convertValues(source["segments"], ContextSegment);
	        this.message_count = source["message_count"];
	        this.tool_count = source["tool_count"];
	        this.estimated = source["estimated"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SystemInfoRESP {
	    os_name: string;
	    os_arch: string;
	    go_version: string;
	    user_home: string;
	    num_cpu: number;
	    goroutines: number;
	    heap_alloc_mb: number;
	    heap_sys_mb: number;
	
	    static createFrom(source: any = {}) {
	        return new SystemInfoRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.os_name = source["os_name"];
	        this.os_arch = source["os_arch"];
	        this.go_version = source["go_version"];
	        this.user_home = source["user_home"];
	        this.num_cpu = source["num_cpu"];
	        this.goroutines = source["goroutines"];
	        this.heap_alloc_mb = source["heap_alloc_mb"];
	        this.heap_sys_mb = source["heap_sys_mb"];
	    }
	}
	export class RecentMessageRESP {
	    id: string;
	    session_id: string;
	    role: string;
	    content: string;
	    created_at: number;
	
	    static createFrom(source: any = {}) {
	        return new RecentMessageRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.session_id = source["session_id"];
	        this.role = source["role"];
	        this.content = source["content"];
	        this.created_at = source["created_at"];
	    }
	}
	export class DashboardStatsRESP {
	    ai_tools_total: number;
	    today_sessions: number;
	    today_messages: number;
	    today_tokens: number;
	    recent_messages: RecentMessageRESP[];
	    system?: SystemInfoRESP;
	
	    static createFrom(source: any = {}) {
	        return new DashboardStatsRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ai_tools_total = source["ai_tools_total"];
	        this.today_sessions = source["today_sessions"];
	        this.today_messages = source["today_messages"];
	        this.today_tokens = source["today_tokens"];
	        this.recent_messages = this.convertValues(source["recent_messages"], RecentMessageRESP);
	        this.system = this.convertValues(source["system"], SystemInfoRESP);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DashboardTrendRESP {
	    days: string[];
	    sessions: number[];
	    messages: number[];
	    tokens: number[];
	
	    static createFrom(source: any = {}) {
	        return new DashboardTrendRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.days = source["days"];
	        this.sessions = source["sessions"];
	        this.messages = source["messages"];
	        this.tokens = source["tokens"];
	    }
	}
	export class DecideApprovalREQ {
	    approved: boolean;
	    scope?: string;
	
	    static createFrom(source: any = {}) {
	        return new DecideApprovalREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.approved = source["approved"];
	        this.scope = source["scope"];
	    }
	}
	export class DocDetailRESP {
	    name: string;
	    title: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new DocDetailRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.title = source["title"];
	        this.content = source["content"];
	    }
	}
	export class DocItemRESP {
	    name: string;
	    title: string;
	
	    static createFrom(source: any = {}) {
	        return new DocItemRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.title = source["title"];
	    }
	}
	export class EffectiveParamsRESP {
	    session_id: string;
	    provider_id: string;
	    model: string;
	    temperature: number;
	    temperature_from: string;
	    thinking_effort: string;
	    thinking_from: string;
	    context_window: number;
	    context_window_from: string;
	    compression_ratio: number;
	    compression_from: string;
	    context_budget: number;
	    max_input_chars: number;
	
	    static createFrom(source: any = {}) {
	        return new EffectiveParamsRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session_id = source["session_id"];
	        this.provider_id = source["provider_id"];
	        this.model = source["model"];
	        this.temperature = source["temperature"];
	        this.temperature_from = source["temperature_from"];
	        this.thinking_effort = source["thinking_effort"];
	        this.thinking_from = source["thinking_from"];
	        this.context_window = source["context_window"];
	        this.context_window_from = source["context_window_from"];
	        this.compression_ratio = source["compression_ratio"];
	        this.compression_from = source["compression_from"];
	        this.context_budget = source["context_budget"];
	        this.max_input_chars = source["max_input_chars"];
	    }
	}
	export class FileChangeDetailRESP {
	    id: string;
	    session_id: string;
	    run_id: string;
	    tool_name: string;
	    rel_path: string;
	    action: string;
	    bytes_before: number;
	    bytes_after: number;
	    added_lines: number;
	    removed_lines: number;
	    rolled_back: boolean;
	    created_at: number;
	    diff: string;
	    before_text: string;
	    after_text: string;
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FileChangeDetailRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.session_id = source["session_id"];
	        this.run_id = source["run_id"];
	        this.tool_name = source["tool_name"];
	        this.rel_path = source["rel_path"];
	        this.action = source["action"];
	        this.bytes_before = source["bytes_before"];
	        this.bytes_after = source["bytes_after"];
	        this.added_lines = source["added_lines"];
	        this.removed_lines = source["removed_lines"];
	        this.rolled_back = source["rolled_back"];
	        this.created_at = source["created_at"];
	        this.diff = source["diff"];
	        this.before_text = source["before_text"];
	        this.after_text = source["after_text"];
	        this.truncated = source["truncated"];
	    }
	}
	export class FileChangeRESP {
	    id: string;
	    session_id: string;
	    run_id: string;
	    tool_name: string;
	    rel_path: string;
	    action: string;
	    bytes_before: number;
	    bytes_after: number;
	    added_lines: number;
	    removed_lines: number;
	    rolled_back: boolean;
	    created_at: number;
	
	    static createFrom(source: any = {}) {
	        return new FileChangeRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.session_id = source["session_id"];
	        this.run_id = source["run_id"];
	        this.tool_name = source["tool_name"];
	        this.rel_path = source["rel_path"];
	        this.action = source["action"];
	        this.bytes_before = source["bytes_before"];
	        this.bytes_after = source["bytes_after"];
	        this.added_lines = source["added_lines"];
	        this.removed_lines = source["removed_lines"];
	        this.rolled_back = source["rolled_back"];
	        this.created_at = source["created_at"];
	    }
	}
	export class FileChangeListRESP {
	    items: FileChangeRESP[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new FileChangeListRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], FileChangeRESP);
	        this.total = source["total"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class FileRESP {
	    id: string;
	    name: string;
	    original_name: string;
	    file_type: string;
	    mime_type: string;
	    size: number;
	    status: string;
	    session_id: string;
	    folder_id: string;
	    created_at: number;
	
	    static createFrom(source: any = {}) {
	        return new FileRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.original_name = source["original_name"];
	        this.file_type = source["file_type"];
	        this.mime_type = source["mime_type"];
	        this.size = source["size"];
	        this.status = source["status"];
	        this.session_id = source["session_id"];
	        this.folder_id = source["folder_id"];
	        this.created_at = source["created_at"];
	    }
	}
	export class FolderREQ {
	    name: string;
	    parent_id?: string;
	    description?: string;
	    workspace_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new FolderREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.parent_id = source["parent_id"];
	        this.description = source["description"];
	        this.workspace_id = source["workspace_id"];
	    }
	}
	export class FolderRESP {
	    id: string;
	    name: string;
	    parent_id?: string;
	    path: string;
	    description?: string;
	    workspace_id?: string;
	    child_count: number;
	    created_at: number;
	    updated_at: number;
	
	    static createFrom(source: any = {}) {
	        return new FolderRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.parent_id = source["parent_id"];
	        this.path = source["path"];
	        this.description = source["description"];
	        this.workspace_id = source["workspace_id"];
	        this.child_count = source["child_count"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class FolderTreeNode {
	    folder: FolderRESP;
	    children: FolderTreeNode[];
	
	    static createFrom(source: any = {}) {
	        return new FolderTreeNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.folder = this.convertValues(source["folder"], FolderRESP);
	        this.children = this.convertValues(source["children"], FolderTreeNode);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ForkSessionREQ {
	    message_id: string;
	    name?: string;
	
	    static createFrom(source: any = {}) {
	        return new ForkSessionREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message_id = source["message_id"];
	        this.name = source["name"];
	    }
	}
	export class GoalREQ {
	    action: string;
	    text?: string;
	
	    static createFrom(source: any = {}) {
	        return new GoalREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.action = source["action"];
	        this.text = source["text"];
	    }
	}
	export class SessionGoal {
	    text: string;
	    status: string;
	    round: number;
	    max_rounds: number;
	    next_step?: string;
	    done_because?: string;
	    updated_at: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionGoal(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.status = source["status"];
	        this.round = source["round"];
	        this.max_rounds = source["max_rounds"];
	        this.next_step = source["next_step"];
	        this.done_because = source["done_because"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class GoalRESP {
	    session_id: string;
	    goal?: SessionGoal;
	
	    static createFrom(source: any = {}) {
	        return new GoalRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session_id = source["session_id"];
	        this.goal = this.convertValues(source["goal"], SessionGoal);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class HealthInfo {
	    status: string;
	    phase: string;
	    uptime_ms: number;
	    db_enabled: boolean;
	    providers: number;
	
	    static createFrom(source: any = {}) {
	        return new HealthInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.phase = source["phase"];
	        this.uptime_ms = source["uptime_ms"];
	        this.db_enabled = source["db_enabled"];
	        this.providers = source["providers"];
	    }
	}
	export class HookTestRESP {
	    decision: string;
	    reason: string;
	    context: string;
	    err: string;
	    duration_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new HookTestRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.decision = source["decision"];
	        this.reason = source["reason"];
	        this.context = source["context"];
	        this.err = source["err"];
	        this.duration_ms = source["duration_ms"];
	    }
	}
	export class KnowledgeDocREQ {
	    folder_id?: string;
	    name: string;
	    source: string;
	    source_type: string;
	
	    static createFrom(source: any = {}) {
	        return new KnowledgeDocREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.folder_id = source["folder_id"];
	        this.name = source["name"];
	        this.source = source["source"];
	        this.source_type = source["source_type"];
	    }
	}
	export class KnowledgeDocRESP {
	    id: string;
	    folder_id?: string;
	    name: string;
	    source: string;
	    source_type: string;
	    mime: string;
	    size_bytes: number;
	    chunk_count: number;
	    status: string;
	    error_msg?: string;
	    created_at: number;
	    updated_at: number;
	
	    static createFrom(source: any = {}) {
	        return new KnowledgeDocRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.folder_id = source["folder_id"];
	        this.name = source["name"];
	        this.source = source["source"];
	        this.source_type = source["source_type"];
	        this.mime = source["mime"];
	        this.size_bytes = source["size_bytes"];
	        this.chunk_count = source["chunk_count"];
	        this.status = source["status"];
	        this.error_msg = source["error_msg"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class KnowledgeHitRESP {
	    doc_id: string;
	    doc_name: string;
	    chunk_id: string;
	    content: string;
	    score: number;
	    source: string;
	    meta?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new KnowledgeHitRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.doc_id = source["doc_id"];
	        this.doc_name = source["doc_name"];
	        this.chunk_id = source["chunk_id"];
	        this.content = source["content"];
	        this.score = source["score"];
	        this.source = source["source"];
	        this.meta = source["meta"];
	    }
	}
	export class McpRawREQ {
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new McpRawREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.content = source["content"];
	    }
	}
	export class McpRawRESP {
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new McpRawRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.content = source["content"];
	    }
	}
	export class McpReloadRESP {
	    active: number;
	    saved: boolean;
	
	    static createFrom(source: any = {}) {
	        return new McpReloadRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.active = source["active"];
	        this.saved = source["saved"];
	    }
	}
	export class McpServerREQ {
	    name: string;
	    transport: string;
	    command: string;
	    args: string[];
	    env: Record<string, string>;
	    enabled?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new McpServerREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.transport = source["transport"];
	        this.command = source["command"];
	        this.args = source["args"];
	        this.env = source["env"];
	        this.enabled = source["enabled"];
	    }
	}
	export class McpServerRESP {
	    id: string;
	    name: string;
	    transport: string;
	    command: string;
	    args: string[];
	    env: Record<string, string>;
	    enabled: boolean;
	    tool_count: number;
	    ready: boolean;
	    error?: string;
	    created_at: number;
	    updated_at: number;
	
	    static createFrom(source: any = {}) {
	        return new McpServerRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.transport = source["transport"];
	        this.command = source["command"];
	        this.args = source["args"];
	        this.env = source["env"];
	        this.enabled = source["enabled"];
	        this.tool_count = source["tool_count"];
	        this.ready = source["ready"];
	        this.error = source["error"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class MemoryDeleteREQ {
	    section: string;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new MemoryDeleteREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.section = source["section"];
	        this.text = source["text"];
	    }
	}
	export class MemoryEntryRESP {
	    section: string;
	    text: string;
	    score?: number;
	
	    static createFrom(source: any = {}) {
	        return new MemoryEntryRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.section = source["section"];
	        this.text = source["text"];
	        this.score = source["score"];
	    }
	}
	export class MemoryListRESP {
	    file: string;
	    items: MemoryEntryRESP[];
	
	    static createFrom(source: any = {}) {
	        return new MemoryListRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file = source["file"];
	        this.items = this.convertValues(source["items"], MemoryEntryRESP);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class MemorySectionRESP {
	    section: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new MemorySectionRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.section = source["section"];
	        this.count = source["count"];
	    }
	}
	export class MemoryOverviewRESP {
	    file: string;
	    entries: number;
	    sections: MemorySectionRESP[];
	
	    static createFrom(source: any = {}) {
	        return new MemoryOverviewRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file = source["file"];
	        this.entries = source["entries"];
	        this.sections = this.convertValues(source["sections"], MemorySectionRESP);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class MemoryReplaceREQ {
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new MemoryReplaceREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	    }
	}
	
	export class MemoryWriteREQ {
	    section: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new MemoryWriteREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.section = source["section"];
	        this.content = source["content"];
	    }
	}
	export class MessageAttachment {
	    id: string;
	    name: string;
	    mime: string;
	    size: number;
	    kind: string;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new MessageAttachment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.mime = source["mime"];
	        this.size = source["size"];
	        this.kind = source["kind"];
	        this.url = source["url"];
	    }
	}
	export class MessageBlockRESP {
	    id: string;
	    message_id: string;
	    seq: number;
	    kind: string;
	    payload: string;
	    created_at: number;
	
	    static createFrom(source: any = {}) {
	        return new MessageBlockRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.message_id = source["message_id"];
	        this.seq = source["seq"];
	        this.kind = source["kind"];
	        this.payload = source["payload"];
	        this.created_at = source["created_at"];
	    }
	}
	export class MessageRESP {
	    id: string;
	    session_id: string;
	    run_id: string;
	    role: string;
	    content: string;
	    thinking: string;
	    tool_call_id: string;
	    tool_calls_json: string;
	    status: string;
	    context_scope: string;
	    stop_reason: string;
	    model: string;
	    input_tokens: number;
	    output_tokens: number;
	    cache_read_tokens: number;
	    total_tokens: number;
	    latency_ms: number;
	    cost: string;
	    created_at: number;
	    updated_at: number;
	    blocks?: MessageBlockRESP[];
	    attachments?: MessageAttachment[];
	
	    static createFrom(source: any = {}) {
	        return new MessageRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.session_id = source["session_id"];
	        this.run_id = source["run_id"];
	        this.role = source["role"];
	        this.content = source["content"];
	        this.thinking = source["thinking"];
	        this.tool_call_id = source["tool_call_id"];
	        this.tool_calls_json = source["tool_calls_json"];
	        this.status = source["status"];
	        this.context_scope = source["context_scope"];
	        this.stop_reason = source["stop_reason"];
	        this.model = source["model"];
	        this.input_tokens = source["input_tokens"];
	        this.output_tokens = source["output_tokens"];
	        this.cache_read_tokens = source["cache_read_tokens"];
	        this.total_tokens = source["total_tokens"];
	        this.latency_ms = source["latency_ms"];
	        this.cost = source["cost"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	        this.blocks = this.convertValues(source["blocks"], MessageBlockRESP);
	        this.attachments = this.convertValues(source["attachments"], MessageAttachment);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class MessageListRESP {
	    items: MessageRESP[];
	    total: number;
	    next_seq: number;
	
	    static createFrom(source: any = {}) {
	        return new MessageListRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], MessageRESP);
	        this.total = source["total"];
	        this.next_seq = source["next_seq"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ProviderCircuitRESP {
	    id: string;
	    name: string;
	    enabled: boolean;
	    state: string;
	    consecutive_failures: number;
	    retry_in_ms: number;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderCircuitRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	        this.state = source["state"];
	        this.consecutive_failures = source["consecutive_failures"];
	        this.retry_in_ms = source["retry_in_ms"];
	        this.reason = source["reason"];
	    }
	}
	export class ProviderKindMeta {
	    kind: string;
	    label: string;
	    base_url_default: string;
	    model_placeholder: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderKindMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.label = source["label"];
	        this.base_url_default = source["base_url_default"];
	        this.model_placeholder = source["model_placeholder"];
	    }
	}
	
	export class RunRecordListREQ {
	    session_id: string;
	    limit: number;
	    offset: number;
	
	    static createFrom(source: any = {}) {
	        return new RunRecordListREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session_id = source["session_id"];
	        this.limit = source["limit"];
	        this.offset = source["offset"];
	    }
	}
	export class RunRecordRESP {
	    run_id: string;
	    session_id: string;
	    model: string;
	    status: string;
	    reason: string;
	    turns: number;
	    input_tokens: number;
	    output_tokens: number;
	    cache_read_tokens: number;
	    total_tokens: number;
	    llm_ms: number;
	    tools_ms: number;
	    compress_ms: number;
	    started_at: number;
	    ended_at: number;
	
	    static createFrom(source: any = {}) {
	        return new RunRecordRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.run_id = source["run_id"];
	        this.session_id = source["session_id"];
	        this.model = source["model"];
	        this.status = source["status"];
	        this.reason = source["reason"];
	        this.turns = source["turns"];
	        this.input_tokens = source["input_tokens"];
	        this.output_tokens = source["output_tokens"];
	        this.cache_read_tokens = source["cache_read_tokens"];
	        this.total_tokens = source["total_tokens"];
	        this.llm_ms = source["llm_ms"];
	        this.tools_ms = source["tools_ms"];
	        this.compress_ms = source["compress_ms"];
	        this.started_at = source["started_at"];
	        this.ended_at = source["ended_at"];
	    }
	}
	export class RunRecordListRESP {
	    items: RunRecordRESP[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new RunRecordListRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], RunRecordRESP);
	        this.total = source["total"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class RuntimeAssetStatus {
	    id: string;
	    version: string;
	    ready: boolean;
	    path: string;
	    executable: string;
	    archive: string;
	    archive_found: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new RuntimeAssetStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.version = source["version"];
	        this.ready = source["ready"];
	        this.path = source["path"];
	        this.executable = source["executable"];
	        this.archive = source["archive"];
	        this.archive_found = source["archive_found"];
	        this.error = source["error"];
	    }
	}
	export class RuntimeStatusRESP {
	    home: string;
	    bundled_dir: string;
	    ready: boolean;
	    error?: string;
	    assets: RuntimeAssetStatus[];
	
	    static createFrom(source: any = {}) {
	        return new RuntimeStatusRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.home = source["home"];
	        this.bundled_dir = source["bundled_dir"];
	        this.ready = source["ready"];
	        this.error = source["error"];
	        this.assets = this.convertValues(source["assets"], RuntimeAssetStatus);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SendStreamREQ {
	    session_id: string;
	    content: string;
	    file_ids?: string[];
	    temperature?: number;
	    thinking_effort?: string;
	
	    static createFrom(source: any = {}) {
	        return new SendStreamREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session_id = source["session_id"];
	        this.content = source["content"];
	        this.file_ids = source["file_ids"];
	        this.temperature = source["temperature"];
	        this.thinking_effort = source["thinking_effort"];
	    }
	}
	export class SendStreamResult {
	    run_id: string;
	    session_id: string;
	    user_message_id: string;
	    assistant_message_id: string;
	
	    static createFrom(source: any = {}) {
	        return new SendStreamResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.run_id = source["run_id"];
	        this.session_id = source["session_id"];
	        this.user_message_id = source["user_message_id"];
	        this.assistant_message_id = source["assistant_message_id"];
	    }
	}
	export class SessionArchiveREQ {
	    archived: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SessionArchiveREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.archived = source["archived"];
	    }
	}
	
	export class SessionListRESP {
	    items: ChatSessionRESP[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionListRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], ChatSessionRESP);
	        this.total = source["total"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SessionPinREQ {
	    pinned: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SessionPinREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pinned = source["pinned"];
	    }
	}
	export class SessionSearchREQ {
	    query: string;
	    scope?: string;
	    limit?: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionSearchREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.query = source["query"];
	        this.scope = source["scope"];
	        this.limit = source["limit"];
	    }
	}
	export class SetSessionAgentREQ {
	    agent: string;
	
	    static createFrom(source: any = {}) {
	        return new SetSessionAgentREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agent = source["agent"];
	    }
	}
	export class SkillScript {
	    name: string;
	    language: string;
	    code: string;
	
	    static createFrom(source: any = {}) {
	        return new SkillScript(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.language = source["language"];
	        this.code = source["code"];
	    }
	}
	export class SkillREQ {
	    name: string;
	    description: string;
	    when_to_use: string;
	    body: string;
	    allowed_tools: string[];
	    scripts?: SkillScript[];
	    enabled?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SkillREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.when_to_use = source["when_to_use"];
	        this.body = source["body"];
	        this.allowed_tools = source["allowed_tools"];
	        this.scripts = this.convertValues(source["scripts"], SkillScript);
	        this.enabled = source["enabled"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SkillRESP {
	    id: string;
	    name: string;
	    description: string;
	    when_to_use: string;
	    body?: string;
	    allowed_tools?: string[];
	    scripts?: SkillScript[];
	    source_kind: string;
	    source_ref?: string;
	    version?: string;
	    enabled: boolean;
	    created_at: number;
	    updated_at: number;
	
	    static createFrom(source: any = {}) {
	        return new SkillRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.when_to_use = source["when_to_use"];
	        this.body = source["body"];
	        this.allowed_tools = source["allowed_tools"];
	        this.scripts = this.convertValues(source["scripts"], SkillScript);
	        this.source_kind = source["source_kind"];
	        this.source_ref = source["source_ref"];
	        this.version = source["version"];
	        this.enabled = source["enabled"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class SteerREQ {
	    session_id: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new SteerREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session_id = source["session_id"];
	        this.content = source["content"];
	    }
	}
	export class SteerResultRESP {
	    run_id: string;
	    session_id: string;
	    message_id: string;
	    queued: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SteerResultRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.run_id = source["run_id"];
	        this.session_id = source["session_id"];
	        this.message_id = source["message_id"];
	        this.queued = source["queued"];
	    }
	}
	
	export class SystemSettingRESP {
	    k: string;
	    v: string;
	    updated_at: number;
	
	    static createFrom(source: any = {}) {
	        return new SystemSettingRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.k = source["k"];
	        this.v = source["v"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class TodoItem {
	    id: string;
	    title: string;
	    done: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TodoItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.done = source["done"];
	    }
	}
	export class TodoStateRESP {
	    session_id: string;
	    items: TodoItem[];
	    done_count: number;
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new TodoStateRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session_id = source["session_id"];
	        this.items = this.convertValues(source["items"], TodoItem);
	        this.done_count = source["done_count"];
	        this.total = source["total"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TokenTrendREQ {
	    scope: string;
	    start_at: number;
	    end_at: number;
	
	    static createFrom(source: any = {}) {
	        return new TokenTrendREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scope = source["scope"];
	        this.start_at = source["start_at"];
	        this.end_at = source["end_at"];
	    }
	}
	export class TokenTrendRESP {
	    scope: string;
	    granularity: string;
	    start_at: number;
	    end_at: number;
	    labels: string[];
	    input: number[];
	    output: number[];
	    cache_read: number[];
	    total: number;
	    cost_usd: number;
	
	    static createFrom(source: any = {}) {
	        return new TokenTrendRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scope = source["scope"];
	        this.granularity = source["granularity"];
	        this.start_at = source["start_at"];
	        this.end_at = source["end_at"];
	        this.labels = source["labels"];
	        this.input = source["input"];
	        this.output = source["output"];
	        this.cache_read = source["cache_read"];
	        this.total = source["total"];
	        this.cost_usd = source["cost_usd"];
	    }
	}
	export class ToolParamVO {
	    name: string;
	    type: string;
	    required: boolean;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new ToolParamVO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.required = source["required"];
	        this.description = source["description"];
	    }
	}
	export class ToolMeta {
	    name: string;
	    description: string;
	    risk_level: string;
	    group: string;
	    category: string;
	    activity_desc: string;
	    read_only: boolean;
	    destructive: boolean;
	    enabled: boolean;
	    params: ToolParamVO[];
	    schema_json: string;
	
	    static createFrom(source: any = {}) {
	        return new ToolMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.risk_level = source["risk_level"];
	        this.group = source["group"];
	        this.category = source["category"];
	        this.activity_desc = source["activity_desc"];
	        this.read_only = source["read_only"];
	        this.destructive = source["destructive"];
	        this.enabled = source["enabled"];
	        this.params = this.convertValues(source["params"], ToolParamVO);
	        this.schema_json = source["schema_json"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class TruncateMessagesREQ {
	    message_id: string;
	
	    static createFrom(source: any = {}) {
	        return new TruncateMessagesREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message_id = source["message_id"];
	    }
	}
	export class TrustResolveRESP {
	    path: string;
	    state: string;
	    source: string;
	    matched: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TrustResolveRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.state = source["state"];
	        this.source = source["source"];
	        this.matched = source["matched"];
	    }
	}
	export class UploadDataREQ {
	    name: string;
	    data_base64: string;
	    session_id?: string;
	    folder_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new UploadDataREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.data_base64 = source["data_base64"];
	        this.session_id = source["session_id"];
	        this.folder_id = source["folder_id"];
	    }
	}
	export class UserCommandREQ {
	    name: string;
	    prompt: string;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new UserCommandREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.prompt = source["prompt"];
	        this.description = source["description"];
	    }
	}
	export class UserCommandRESP {
	    id: string;
	    name: string;
	    prompt: string;
	    description: string;
	    created_at: number;
	    updated_at: number;
	
	    static createFrom(source: any = {}) {
	        return new UserCommandRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.prompt = source["prompt"];
	        this.description = source["description"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class UserHookREQ {
	    id: string;
	    name: string;
	    event: string;
	    matcher: string;
	    command: string;
	    timeout_ms: number;
	    enabled?: boolean;
	    sort?: number;
	
	    static createFrom(source: any = {}) {
	        return new UserHookREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.event = source["event"];
	        this.matcher = source["matcher"];
	        this.command = source["command"];
	        this.timeout_ms = source["timeout_ms"];
	        this.enabled = source["enabled"];
	        this.sort = source["sort"];
	    }
	}
	export class UserHookRESP {
	    id: string;
	    name: string;
	    event: string;
	    matcher: string;
	    command: string;
	    timeout_ms: number;
	    enabled: boolean;
	    sort: number;
	
	    static createFrom(source: any = {}) {
	        return new UserHookRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.event = source["event"];
	        this.matcher = source["matcher"];
	        this.command = source["command"];
	        this.timeout_ms = source["timeout_ms"];
	        this.enabled = source["enabled"];
	        this.sort = source["sort"];
	    }
	}
	export class VersionInfo {
	    app_name: string;
	    version: string;
	    phase: string;
	    env: string;
	    default_tenant: string;
	    local_user_id: string;
	
	    static createFrom(source: any = {}) {
	        return new VersionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.app_name = source["app_name"];
	        this.version = source["version"];
	        this.phase = source["phase"];
	        this.env = source["env"];
	        this.default_tenant = source["default_tenant"];
	        this.local_user_id = source["local_user_id"];
	    }
	}
	export class WebSearchConfigRESP {
	    enabled: string;
	    engine: string;
	    api_key: string;
	
	    static createFrom(source: any = {}) {
	        return new WebSearchConfigRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.engine = source["engine"];
	        this.api_key = source["api_key"];
	    }
	}
	export class WorkspaceEntry {
	    path: string;
	    name: string;
	    is_dir: boolean;
	    size: number;
	    ext: string;
	    kind: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.is_dir = source["is_dir"];
	        this.size = source["size"];
	        this.ext = source["ext"];
	        this.kind = source["kind"];
	    }
	}
	export class WorkspaceFileItem {
	    path: string;
	    name: string;
	    ext: string;
	    kind: string;
	    size: number;
	    url: string;
	    modified_at: number;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceFileItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.ext = source["ext"];
	        this.kind = source["kind"];
	        this.size = source["size"];
	        this.url = source["url"];
	        this.modified_at = source["modified_at"];
	    }
	}
	export class WorkspaceListRESP {
	    items: WorkspaceEntry[];
	    truncated: boolean;
	    root: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceListRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], WorkspaceEntry);
	        this.truncated = source["truncated"];
	        this.root = source["root"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WorkspaceTrustREQ {
	    path: string;
	    state: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceTrustREQ(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.state = source["state"];
	    }
	}
	export class WorkspaceTrustRESP {
	    path: string;
	    state: string;
	    updated_at: number;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceTrustRESP(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.state = source["state"];
	        this.updated_at = source["updated_at"];
	    }
	}

}

export namespace event {
	
	export class Bus {
	
	
	    static createFrom(source: any = {}) {
	        return new Bus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}
	export class RunEventLog {
	
	
	    static createFrom(source: any = {}) {
	        return new RunEventLog(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}

}

export namespace llm {
	
	export class ProviderPreset {
	    name: string;
	    kind: string;
	    base_url: string;
	    models: string[];
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderPreset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.base_url = source["base_url"];
	        this.models = source["models"];
	        this.note = source["note"];
	    }
	}

}

