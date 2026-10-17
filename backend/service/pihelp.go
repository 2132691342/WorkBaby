// WorkBaby 与 PI 之间的辅助装配：小工具函数、审批扩展的生成、配置同步入口。
package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	pienv "WorkBaby/backend/pi/env"
	"WorkBaby/backend/pkg"
)

// pkgWarn 记一条警告（封装一层，避免各处重复写格式化）。
func pkgWarn(msg string, err error) {
	pkg.Warnf("%s: %v", msg, err)
}

// writeFileAtomic 原子替换写入。
func writeFileAtomic(path string, data []byte) error {
	return pienv.WriteFileAtomic(path, data, 0o600)
}

// readFileOrEmpty 读文件；不存在时返回空串。
func readFileOrEmpty(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", pkg.Wrap("9713", "读配置文件失败", err)
	}
	return string(raw), nil
}

// parseInt 解析一个整数设置值。
func parseInt(s string) (int, error) { return strconv.Atoi(strings.TrimSpace(s)) }

// selfExe 返回当前可执行文件路径（知识库 MCP server 要用它重新拉自己）。
func selfExe() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe
}

// MCPConfig 读 mcp.json 里的服务名列表。
func (e *Env) MCPConfig() map[string]json.RawMessage {
	raw, err := e.PiEnv.ReadMCPRaw()
	if err != nil {
		return map[string]json.RawMessage{}
	}
	var doc struct {
		Servers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return map[string]json.RawMessage{}
	}
	if doc.Servers == nil {
		return map[string]json.RawMessage{}
	}
	return doc.Servers
}

// ensureApprovalExtension 把审批闸门扩展写进 PI 的扩展目录并在 settings 里登记。
// 只写文件不登记，PI 根本不加载它，表现为「审批档设了没用」。
func ensureApprovalExtension(env *Env) error {
	dir := env.PiEnv.ExtensionsDir()
	if err := pkg.EnsureDir(dir); err != nil {
		return err
	}
	file := filepath.Join(dir, "workbaby.ts")
	// 每次启动重写整个文件：追加会让多次启动累积重复的处理器，
	// 同一个调用会弹两次确认。
	if err := writeFileAtomic(file, []byte(approvalExtensionSource)); err != nil {
		return err
	}
	settings, err := env.PiEnv.ReadSettings()
	if err != nil {
		return err
	}
	if settings == nil {
		settings = &pienv.SettingsFile{}
	}
	// 用绝对路径登记：settings 里的相对路径按 agent 目录解析，写绝对路径更不会错。
	found := false
	for _, e := range settings.Extensions {
		if e == file {
			found = true
			break
		}
	}
	if !found {
		settings.Extensions = append(settings.Extensions, file)
		if err := env.PiEnv.WriteSettings(settings); err != nil {
			return err
		}
	}
	return nil
}

// approvalExtensionSource 是审批闸门与记忆写入工具的实现。
// 它跑在 PI 进程里，通过 extension_ui_request 与 WorkBaby 对话。
const approvalExtensionSource = `import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { readFile } from "node:fs/promises";
import { join } from "node:path";

interface Policy {
  permission: "auto" | "ask_write" | "readonly";
  timeoutMs: number;
  allowTools?: string[];
}

const READONLY = new Set(["read", "grep", "find", "ls", "web_search", "web_fetch", "tool_search"]);

function policyPath(): string {
  const dir = process.env.PI_CODING_AGENT_DIR || "";
  return join(dir, "workbaby-policy.json");
}

async function loadPolicy(): Promise<Policy> {
  try {
    const raw = await readFile(policyPath(), "utf8");
    const parsed = JSON.parse(raw) as Partial<Policy>;
    return {
      permission: parsed.permission ?? "ask_write",
      timeoutMs: parsed.timeoutMs ?? 1800000,
      allowTools: parsed.allowTools ?? [],
    };
  } catch {
    return { permission: "ask_write", timeoutMs: 1800000, allowTools: [] };
  }
}

export default function (pi: ExtensionAPI) {
  let policy: Policy = { permission: "ask_write", timeoutMs: 1800000, allowTools: [] };

  pi.on("session_start", async () => {
    policy = await loadPolicy();
  });

  pi.on("tool_call", async (event, ctx) => {
    if (policy.permission === "auto") return;
    if (policy.allowTools?.includes(event.toolName)) return;
    if (READONLY.has(event.toolName)) return;
    if (policy.permission === "readonly") {
      return { block: true, reason: "当前是只读模式，这个操作不会执行" };
    }
    const detail = JSON.stringify(event.input ?? {}).slice(0, 500);
    const ok = await ctx.ui.confirm("允许 " + event.toolName + "?", detail, {
      timeout: policy.timeoutMs,
    });
    if (!ok) {
      return { block: true, reason: event.toolName + " 没有被批准" };
    }
  });
}
`
