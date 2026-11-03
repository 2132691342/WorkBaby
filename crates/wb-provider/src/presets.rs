//! 内置服务商预设：常见服务商的地址 / 模型 / 能力，供首启「一键添加」。密钥永远由用户给。

use wb_domain::provider::ProviderCfg;

/// 一条预设：条目本身 + 给界面看的一句话说明。
pub struct Preset {
    /// models.json 里的键（同时是预设码）
    pub preset_id: &'static str,
    /// 显示名
    pub preset_name: &'static str,
    /// 一句话说明（申请密钥的地方 / 适用场景）
    pub preset_hint: &'static str,
    /// 填入 models.json 的条目
    pub preset_cfg: fn() -> ProviderCfg,
}

/// 全部预设。顺序即界面展示顺序：先放国内可直连的，再放需要网络的。
pub static PRESETS: &[Preset] = &[
    Preset {
        preset_id: "openai",
        preset_name: "OpenAI",
        preset_hint: "官方接口，需要能访问 api.openai.com",
        preset_cfg: openai,
    },
    Preset {
        preset_id: "deepseek",
        preset_name: "DeepSeek",
        preset_hint: "国内直连，性价比高；到 platform.deepseek.com 申请密钥",
        preset_cfg: deepseek,
    },
    Preset {
        preset_id: "moonshot",
        preset_name: "Moonshot / Kimi",
        preset_hint: "国内直连，长上下文；到 platform.moonshot.cn 申请密钥",
        preset_cfg: moonshot,
    },
    Preset {
        preset_id: "dashscope",
        preset_name: "阿里云百炼 / 通义",
        preset_hint: "国内直连，Qwen 系列；到 bailian.console.aliyun.com 申请密钥",
        preset_cfg: dashscope,
    },
    Preset {
        preset_id: "zhipu",
        preset_name: "智谱 GLM",
        preset_hint: "国内直连，GLM 系列；到 open.bigmodel.cn 申请密钥",
        preset_cfg: zhipu,
    },
    Preset {
        preset_id: "openrouter",
        preset_name: "OpenRouter",
        preset_hint: "一个密钥通吃多家模型，聚合网关",
        preset_cfg: openrouter,
    },
    Preset {
        preset_id: "anthropic",
        preset_name: "Anthropic Claude",
        preset_hint: "Claude 官方接口，需要能访问 api.anthropic.com",
        preset_cfg: anthropic,
    },
    Preset {
        preset_id: "gemini",
        preset_name: "Google Gemini",
        preset_hint: "Gemini 官方接口，需要能访问 generativelanguage.googleapis.com",
        preset_cfg: gemini,
    },
    Preset {
        preset_id: "ollama",
        preset_name: "Ollama 本地",
        preset_hint: "本机跑模型，无需密钥；先 `ollama serve` 再选模型",
        preset_cfg: ollama,
    },
];

/// 按预设码取构造器。
pub fn find(preset_id: &str) -> Option<&'static Preset> {
    PRESETS.iter().find(|p| p.preset_id == preset_id)
}

/// 空的附加头表（大多数服务商不需要）。
fn no_headers() -> std::collections::BTreeMap<String, String> {
    std::collections::BTreeMap::new()
}

fn openai() -> ProviderCfg {
    ProviderCfg {
        name: "OpenAI".into(),
        base_url: "https://api.openai.com/v1".into(),
        api: "openai-completions".into(),
        api_key: String::new(),
        headers: no_headers(),
        cache: None,
        models: vec![
            wb_domain::provider::ModelCfg {
                id: "gpt-5.1".into(),
                name: "GPT-5.1".into(),
                reasoning: true,
                vision: true,
                context_window: 400_000,
                max_tokens: 32_768,
            },
            wb_domain::provider::ModelCfg {
                id: "gpt-5.1-mini".into(),
                name: "GPT-5.1 mini".into(),
                reasoning: true,
                vision: true,
                context_window: 400_000,
                max_tokens: 32_768,
            },
            wb_domain::provider::ModelCfg {
                id: "gpt-4.1".into(),
                name: "GPT-4.1".into(),
                reasoning: false,
                vision: true,
                context_window: 1_047_576,
                max_tokens: 32_768,
            },
        ],
    }
}

fn deepseek() -> ProviderCfg {
    ProviderCfg {
        name: "DeepSeek".into(),
        base_url: "https://api.deepseek.com/v1".into(),
        api: "openai-completions".into(),
        api_key: String::new(),
        headers: no_headers(),
        cache: None,
        models: vec![
            wb_domain::provider::ModelCfg {
                id: "deepseek-chat".into(),
                name: "DeepSeek V3".into(),
                reasoning: false,
                vision: false,
                context_window: 131_072,
                max_tokens: 8_192,
            },
            wb_domain::provider::ModelCfg {
                id: "deepseek-reasoner".into(),
                name: "DeepSeek R1".into(),
                reasoning: true,
                vision: false,
                context_window: 131_072,
                max_tokens: 65_536,
            },
        ],
    }
}

fn moonshot() -> ProviderCfg {
    ProviderCfg {
        name: "Moonshot / Kimi".into(),
        base_url: "https://api.moonshot.cn/v1".into(),
        api: "openai-completions".into(),
        api_key: String::new(),
        headers: no_headers(),
        cache: None,
        models: vec![wb_domain::provider::ModelCfg {
            id: "kimi-k2-0905-preview".into(),
            name: "Kimi K2".into(),
            reasoning: false,
            vision: false,
            context_window: 262_144,
            max_tokens: 16_384,
        }],
    }
}

fn dashscope() -> ProviderCfg {
    ProviderCfg {
        name: "阿里云百炼 / 通义".into(),
        base_url: "https://dashscope.aliyuncs.com/compatible-mode/v1".into(),
        api: "openai-completions".into(),
        api_key: String::new(),
        headers: no_headers(),
        cache: None,
        models: vec![
            wb_domain::provider::ModelCfg {
                id: "qwen3-coder-plus".into(),
                name: "Qwen3 Coder Plus".into(),
                reasoning: false,
                vision: false,
                context_window: 1_000_000,
                max_tokens: 65_536,
            },
            wb_domain::provider::ModelCfg {
                id: "qwen3-max".into(),
                name: "Qwen3 Max".into(),
                reasoning: true,
                vision: false,
                context_window: 262_144,
                max_tokens: 32_768,
            },
        ],
    }
}

fn zhipu() -> ProviderCfg {
    ProviderCfg {
        name: "智谱 GLM".into(),
        base_url: "https://open.bigmodel.cn/api/paas/v4".into(),
        api: "openai-completions".into(),
        api_key: String::new(),
        headers: no_headers(),
        cache: None,
        models: vec![wb_domain::provider::ModelCfg {
            id: "glm-4.6".into(),
            name: "GLM-4.6".into(),
            reasoning: true,
            vision: false,
            context_window: 204_800,
            max_tokens: 16_384,
        }],
    }
}

fn openrouter() -> ProviderCfg {
    ProviderCfg {
        name: "OpenRouter".into(),
        base_url: "https://openrouter.ai/api/v1".into(),
        api: "openai-completions".into(),
        api_key: String::new(),
        headers: no_headers(),
        cache: None,
        models: vec![
            wb_domain::provider::ModelCfg {
                id: "anthropic/claude-sonnet-4.5".into(),
                name: "Claude Sonnet 4.5".into(),
                reasoning: true,
                vision: true,
                context_window: 200_000,
                max_tokens: 64_000,
            },
            wb_domain::provider::ModelCfg {
                id: "google/gemini-2.5-pro".into(),
                name: "Gemini 2.5 Pro".into(),
                reasoning: true,
                vision: true,
                context_window: 1_048_576,
                max_tokens: 65_536,
            },
        ],
    }
}

fn ollama() -> ProviderCfg {
    ProviderCfg {
        name: "Ollama 本地".into(),
        base_url: "http://127.0.0.1:11434/v1".into(),
        api: "openai-completions".into(),
        // 本地服务不校验密钥，但请求头必须有值：留一个占位符
        api_key: "ollama".into(),
        headers: no_headers(),
        cache: None,
        models: vec![wb_domain::provider::ModelCfg {
            id: "qwen3:8b".into(),
            name: "Qwen3 8B".into(),
            reasoning: false,
            vision: false,
            context_window: 32_768,
            max_tokens: 8_192,
        }],
    }
}

fn anthropic() -> ProviderCfg {
    ProviderCfg {
        name: "Anthropic Claude".into(),
        base_url: "https://api.anthropic.com".into(),
        api: "anthropic-messages".into(),
        api_key: String::new(),
        headers: no_headers(),
        cache: None,
        models: vec![
            wb_domain::provider::ModelCfg {
                id: "claude-opus-4-5".into(),
                name: "Claude Opus 4.5".into(),
                reasoning: true,
                vision: true,
                context_window: 200_000,
                max_tokens: 64_000,
            },
            wb_domain::provider::ModelCfg {
                id: "claude-sonnet-4-5".into(),
                name: "Claude Sonnet 4.5".into(),
                reasoning: true,
                vision: true,
                context_window: 200_000,
                max_tokens: 64_000,
            },
            wb_domain::provider::ModelCfg {
                id: "claude-haiku-4-5".into(),
                name: "Claude Haiku 4.5".into(),
                reasoning: true,
                vision: true,
                context_window: 200_000,
                max_tokens: 32_000,
            },
        ],
    }
}

fn gemini() -> ProviderCfg {
    ProviderCfg {
        name: "Google Gemini".into(),
        base_url: "https://generativelanguage.googleapis.com".into(),
        api: "google-generative-ai".into(),
        api_key: String::new(),
        headers: no_headers(),
        cache: None,
        models: vec![
            wb_domain::provider::ModelCfg {
                id: "gemini-2.5-pro".into(),
                name: "Gemini 2.5 Pro".into(),
                reasoning: true,
                vision: true,
                context_window: 1_048_576,
                max_tokens: 65_536,
            },
            wb_domain::provider::ModelCfg {
                id: "gemini-2.5-flash".into(),
                name: "Gemini 2.5 Flash".into(),
                reasoning: true,
                vision: true,
                context_window: 1_048_576,
                max_tokens: 65_536,
            },
        ],
    }
}
