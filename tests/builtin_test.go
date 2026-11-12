// 引擎内置能力：待办清单、结构化提问、工具输出折叠、计划模式闸门。
// 这四条都是引擎原生能力而非外挂扩展，每条都要在「落盘 → 回放」或
// 「拦截 → 放行」这一段闭合，否则界面还原不出状态或闸门形同虚设。
package tests

import (
	"context"
	"strings"
	"testing"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/service/parser"
)

// TestTodoRoundTrip 断言待办工具的往返：清单整份替换、状态与现在进行时保留、
// 全量表随结果落盘，回放层据此派生界面要的行。
func TestTodoRoundTrip(t *testing.T) {
	workspace := t.TempDir()
	registry := agent.NewRegistry(func() string { return workspace })

	result := registry.Run(context.Background(), "todo", `{"tasks":[
		{"subject":"先把接口写好","status":"in_progress","activeForm":"正在写接口"},
		{"subject":"再写测试","status":"pending"}
	]}`)
	if result.IsError {
		t.Fatalf("待办工具失败：%s", result.Text)
	}
	// 全量表必须跟着结果走：回放层只靠它复原清单。
	raw, ok := result.Details["tasks"].([]any)
	if !ok || len(raw) != 2 {
		t.Fatalf("待办清单没有随结果落盘：%v", result.Details)
	}
	first, ok := raw[0].(map[string]any)
	if !ok {
		t.Fatalf("待办项形状不对：%v", raw[0])
	}
	if first["subject"] != "先把接口写好" {
		t.Fatalf("待办项内容不对：%v", first)
	}
	if first["status"] != "in_progress" {
		t.Fatalf("待办状态不该丢：%v", first)
	}
	if first["activeForm"] != "正在写接口" {
		t.Fatalf("现在进行时不该丢：%v", first)
	}
	// 任务号按顺序编号：模型不认得旧编号，重排比保留更可预期。
	if id, ok := first["id"].(int); !ok || id != 1 {
		t.Fatalf("任务号该从 1 顺排，实际 %v", first["id"])
	}

	// 整份替换：再调一次，旧任务不该还在。
	result = registry.Run(context.Background(), "todo", `{"tasks":[{"subject":"只做这一件","status":"completed"}]}`)
	if result.IsError {
		t.Fatalf("待办工具失败：%s", result.Text)
	}
	raw, _ = result.Details["tasks"].([]any)
	if len(raw) != 1 {
		t.Fatalf("清单该被整份替换，实际 %d 项", len(raw))
	}

	// 清单能跨「落盘 → 回放」闭合：活动分支上最近一次调用就是当前待办。
	store := agent.NewStore(t.TempDir(), func() string { return workspace })
	session, err := store.Create(workspace)
	if err != nil {
		t.Fatalf("建会话失败：%v", err)
	}
	if _, err := session.AppendMessage(agent.Message{
		Role:      agent.RoleTool,
		ToolName:  "todo",
		Blocks:    []agent.Block{{Kind: agent.KindText, Text: result.Text}},
		Details:   result.Details,
		CreatedAt: 1,
	}); err != nil {
		t.Fatalf("写工具结果失败：%v", err)
	}
	todos := parser.TodosFromFile(session.File())
	if len(todos) != 1 {
		t.Fatalf("回放该派生出一条待办，实际 %d 条", len(todos))
	}
	if todos[0].TodoSubject != "只做这一件" {
		t.Fatalf("待办内容不对：%q", todos[0].TodoSubject)
	}
	if todos[0].TodoStatus != "completed" {
		t.Fatalf("待办状态不对：%q", todos[0].TodoStatus)
	}
}

// TestPlanModeGate 断言只读闸门的三条纪律：写工具被拦、读工具放行、
// 未登记的工具默认按「会改」处理，且系统提示写明被拦的是哪些。
func TestPlanModeGate(t *testing.T) {
	workspace := t.TempDir()
	registry := agent.NewRegistry(func() string { return workspace })

	// 默认不拦：自由态下写工具照常可用。
	write := registry.Run(context.Background(), "write", `{"path":"a.txt","content":"x"}`)
	if write.IsError {
		t.Fatalf("自由态不该拦写工具：%s", write.Text)
	}

	registry.Policy.SetReadOnly("SES_test", true)

	blocked := registry.Run(context.Background(), "write", `{"path":"b.txt","content":"y"}`)
	if !blocked.IsError {
		t.Fatal("只读态下写工具必须被拦下")
	}
	if !strings.Contains(blocked.Text, "计划模式") {
		t.Fatalf("拦截文案要说清是计划模式：%s", blocked.Text)
	}
	if blocked.Details["blocked_by"] != "plan_mode" {
		t.Fatalf("拦截详情要标出原因：%v", blocked.Details)
	}

	// 读工具不受影响：只读模式拦的是改，不是看。
	read := registry.Run(context.Background(), "read", `{"path":"a.txt"}`)
	if read.IsError {
		t.Fatalf("只读态下读工具不该被拦：%s", read.Text)
	}

	// 未登记的工具名默认按写工具处理：漏放一个写工具，计划模式就名存实亡。
	registry.Add(&stubTool{name: "deploy_prod"})
	if out := registry.Run(context.Background(), "deploy_prod", `{}`); !out.IsError {
		t.Fatal("未登记的工具在只读态下必须被拦下")
	}

	// 模型得知道为什么写工具会报错，否则它只会当成故障反复重试。
	restricted := agent.SystemPrompt(registry, t.TempDir())
	if !strings.Contains(restricted, "计划模式") {
		t.Fatalf("只读态的系统提示要说清计划模式：\n%s", restricted)
	}
	if !strings.Contains(restricted, "write") {
		t.Fatalf("系统提示该点名被拦的工具：\n%s", restricted)
	}

	// 退出计划模式后恢复自由。
	registry.Policy.SetReadOnly("SES_test", false)
	again := registry.Run(context.Background(), "write", `{"path":"c.txt","content":"z"}`)
	if again.IsError {
		t.Fatalf("退出只读态后写工具该能用：%s", again.Text)
	}
}

// TestTidyFold 断言工具输出折叠的策略：短输出原样保留、长输出折成头尾加标记、
// 策略关掉后原样返回。折叠直接决定模型的上下文预算，所以三条都要定死。
func TestTidyFold(t *testing.T) {
	registry := agent.NewRegistry(func() string { return t.TempDir() })

	short := "这是一段很短的工具输出。"
	text, truncated, _ := registry.Tidy(short)
	if truncated {
		t.Fatal("短输出不该被折叠")
	}
	if text != short {
		t.Fatalf("短输出该原样保留：%q", text)
	}

	var builder strings.Builder
	builder.WriteString("开头标记")
	for range 5000 {
		builder.WriteString("中间内容")
	}
	builder.WriteString("结尾标记")
	long := builder.String()

	text, truncated, original := registry.Tidy(long)
	if !truncated {
		t.Fatal("长输出该被折叠")
	}
	if original != len([]rune(long)) {
		t.Fatalf("原始字数不对：%d", original)
	}
	if !strings.Contains(text, "开头标记") || !strings.Contains(text, "结尾标记") {
		t.Fatal("头尾都要保留")
	}
	if !strings.Contains(text, "已折叠") {
		t.Fatalf("折叠标记要给模型看：%q", text[:100])
	}
	// 折叠后的长度要真的变短。断言按「至少减到四分之一」：
	// 折叠后仍留着头尾，比例不会趋近于零，但至少要大幅缩短。
	if len([]rune(text)) >= len([]rune(long))/4 {
		t.Fatalf("折叠后还是太长：%d → %d", len([]rune(long)), len([]rune(text)))
	}

	registry.SetTidyEnabled(false)
	plain := strings.Repeat("x", 10000)
	text, truncated, _ = registry.Tidy(plain)
	if truncated {
		t.Fatal("策略关掉后不该折叠")
	}
	if text != plain {
		t.Fatal("策略关掉后该原样返回")
	}
}

// stubTool 是测试用的占位工具：只关心名字，不关心行为。
type stubTool struct {
	// name 工具名
	name string
}

// Spec 工具声明。
func (t *stubTool) Spec() agent.ToolSpec {
	return agent.ToolSpec{
		Name:        t.name,
		Description: "测试用占位工具",
		Schema:      map[string]any{"type": "object", "properties": map[string]any{}},
	}
}

// Run 执行：恒成功。
func (t *stubTool) Run(_ context.Context, _ *agent.Registry, _ map[string]any) agent.Result {
	return agent.Result{Text: "ok"}
}
