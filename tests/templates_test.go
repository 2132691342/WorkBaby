// 提示词模板：落盘、清洗、斜杠命令契约。
package tests

import (
	"strings"
	"testing"

	"WorkBaby/backend/domain"
)

// TestTemplates 断言提示词模板的三条契约：写进去的正文能原样取回（头部不算正文）、
// 模板名清洗后能落成文件、斜杠命令清单带着正文与 prompt 来源——前端正是靠它填框。
func TestTemplates(t *testing.T) {
	h := newHarness(t)

	// ---- 落盘与回读：正文是「头部之后」的那段，头部不混进正文 ----
	body := "请 review 这段代码，关注并发安全。"
	if err := h.app.Templates.Save(domain.PromptTemplateREQ{
		TemplateName: "Review",
		TemplateDesc: "代码评审",
		TemplateArgs: "<文件>",
		TemplateBody: body,
	}); err != nil {
		t.Fatalf("保存模板失败：%v", err)
	}

	rows := h.app.Templates.List().Rows
	if len(rows) != 1 {
		t.Fatalf("应有 1 条模板，实际 %d 条", len(rows))
	}
	got := rows[0]
	if got.TemplateName != "review" {
		t.Errorf("模板名应清洗成小写 review，实际 %q", got.TemplateName)
	}
	if got.TemplateBody != body {
		t.Errorf("正文应原样取回且不含头部：\n期望 %q\n实际 %q", body, got.TemplateBody)
	}
	if strings.Contains(got.TemplateBody, "---") || strings.Contains(got.TemplateBody, "description:") {
		t.Errorf("正文不该混进 YAML 头：%q", got.TemplateBody)
	}
	if got.TemplateDesc != "代码评审" || got.TemplateArgs != "<文件>" {
		t.Errorf("头部字段应保留：desc=%q args=%q", got.TemplateDesc, got.TemplateArgs)
	}

	// ---- 命令契约：斜杠命令清单要带 prompt 来源与正文，前端靠它选中后填框 ----
	cmds := h.app.Templates.Commands()
	if len(cmds) != 1 {
		t.Fatalf("应有 1 条模板命令，实际 %d 条", len(cmds))
	}
	if cmds[0].CommandSource != "prompt" {
		t.Errorf("模板命令来源应是 prompt，实际 %q", cmds[0].CommandSource)
	}
	if cmds[0].CommandBody != body {
		t.Errorf("命令应携带正文供填框，实际 %q", cmds[0].CommandBody)
	}

	// ---- 删除：删一次就没了，再删一次不报错（幂等） ----
	if err := h.app.Templates.Delete("review"); err != nil {
		t.Fatalf("删除模板失败：%v", err)
	}
	if rows := h.app.Templates.List().Rows; len(rows) != 0 {
		t.Errorf("删除后应为空，实际 %d 条", len(rows))
	}
	if err := h.app.Templates.Delete("review"); err != nil {
		t.Errorf("删除不存在的模板应幂等，实际报错：%v", err)
	}

	// ---- 空名拒绝：清洗后为空的名落盘会拼出 .md 这种摸不着头脑的文件名 ----
	if err := h.app.Templates.Save(domain.PromptTemplateREQ{
		TemplateName: "---/*",
		TemplateBody: "正文",
	}); err == nil {
		t.Fatal("模板名清洗后为空时应当拒绝保存")
	}
}
