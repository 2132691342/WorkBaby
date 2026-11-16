// 技能：推荐清单与磁盘资产互相咬合，技能生命周期与提示里的索引整条可跑。
package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service"
)

func TestSkills(t *testing.T) {
	// 清单咬合：少一边就是死条目——清单里的目录必须在 assets\skills 下存在，
	// 磁盘上的目录必须进清单；SKILL.md 的 description 必须能被解析出来（触发全靠它）。
	t.Run("清单咬合", func(t *testing.T) {
		assets := repoAssetsDir(t)
		skillsDir := filepath.Join(assets, "skills")
		entries, err := os.ReadDir(skillsDir)
		if err != nil {
			t.Fatalf("读内置技能资产失败：%v", err)
		}
		onDisk := map[string]struct{}{}
		for _, entry := range entries {
			if entry.IsDir() {
				onDisk[entry.Name()] = struct{}{}
			}
		}
		for _, item := range service.RecommendedSkills {
			if _, ok := onDisk[item.SkillDirName]; !ok {
				t.Errorf("推荐清单里的技能在 assets\\skills 下不存在：%s", item.SkillDirName)
			}
		}
		for name := range onDisk {
			if service.RecommendedSkillOf(name) == nil {
				t.Errorf("assets\\skills 下的 %s 没进推荐清单，界面不会展示它", name)
			}
		}
		// description 是引擎决定「何时触发」的唯一依据，块标量（| 与 >）接不上就只剩一个字符。
		for name := range onDisk {
			raw, err := os.ReadFile(filepath.Join(skillsDir, name, "SKILL.md"))
			if err != nil {
				t.Errorf("技能 %s 没有 SKILL.md：%v", name, err)
				continue
			}
			fm := pkg.ParseFrontmatter(string(raw))
			if fm.Name == "" {
				t.Errorf("技能 %s 解析不出 name", name)
			}
			if len(fm.Description) < 10 {
				t.Errorf("技能 %s 的描述解析成了 %d 个字符（块标量没接上）：%q", name, len(fm.Description), fm.Description)
			}
		}
	})

	// 渐进披露：启用技能的索引进系统提示（名字 + 说明 + 路径），停用的不进，
	// 空清单连标题都不出现——空标题会让模型以为「有技能但看不见」。
	t.Run("提示里的技能索引", func(t *testing.T) {
		h := newHarness(t)
		script := newScriptServer(t, replyChunks("收到"))
		bindWorkspaceAndModel(t, h, script.server.URL)

		// 空技能目录：没有「可用技能」这一节。
		env, code := h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "空目录这一轮"})
		h.mustOK(env, code, "send-empty", nil)
		waitIdle(t, h)
		bodies := script.requestBodies()
		if len(bodies) != 1 {
			t.Fatalf("上游应收到 1 次请求，实际 %d", len(bodies))
		}
		if strings.Contains(bodies[0], "## 可用技能") {
			t.Fatalf("没有技能时不该出现「## 可用技能」标题：%s", bodies[0])
		}

		// 建一条启用的技能，它必须带着名字、说明与绝对路径进提示。
		env, code = h.call("POST", "/skills/create", domain.SkillCreateREQ{
			SkillName: "prompt-probe", SkillDesc: "验证索引进提示", SkillBody: "正文很长，不该整段进提示",
		})
		h.mustOK(env, code, "create", nil)
		script.resetBodies()
		env, code = h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "有技能这一轮"})
		h.mustOK(env, code, "send-enabled", nil)
		waitIdle(t, h)
		bodies = script.requestBodies()
		if len(bodies) != 1 {
			t.Fatalf("上游应收到 1 次请求，实际 %d", len(bodies))
		}
		prompt := bodies[0]
		if !strings.Contains(prompt, "## 可用技能") {
			t.Fatalf("启用技能后应出现「## 可用技能」标题：%s", prompt)
		}
		active := filepath.Join(h.layout.SkillsDir, "prompt-probe", "SKILL.md")
		// 上游请求体是 JSON：路径里的反斜杠会被转义，比对前先按同样规则折一遍。
		escapedActive := strings.ReplaceAll(active, `\`, `\\`)
		for _, want := range []string{"prompt-probe", "验证索引进提示", escapedActive} {
			if !strings.Contains(prompt, want) {
				t.Errorf("提示里应含 %q：%s", want, prompt)
			}
		}
		// 正文不整段进提示：那是渐进披露的全部意义。
		if strings.Contains(prompt, "正文很长，不该整段进提示") {
			t.Fatalf("技能正文不该整段进系统提示：%s", prompt)
		}

		// 停用 = 搬目录，下一轮（不重启）就不该再出现。
		if err := h.app.Skills.Toggle(active, false); err != nil {
			t.Fatalf("停用技能失败：%v", err)
		}
		script.resetBodies()
		env, code = h.call("POST", "/chat/send", domain.ChatSendREQ{Message: "停用后这一轮"})
		h.mustOK(env, code, "send-disabled", nil)
		waitIdle(t, h)
		bodies = script.requestBodies()
		if len(bodies) != 1 {
			t.Fatalf("上游应收到 1 次请求，实际 %d", len(bodies))
		}
		if strings.Contains(bodies[0], "prompt-probe") {
			t.Fatalf("停用的技能不该出现在提示里：%s", bodies[0])
		}
		if strings.Contains(bodies[0], "## 可用技能") {
			t.Fatalf("唯一一条技能停用后不该留空标题：%s", bodies[0])
		}
		// 停用区的正文也不该读得到：豁免只认当前启用的那几条路径。
		hidden := filepath.Join(h.layout.SkillsOffDir, "prompt-probe", "SKILL.md")
		hiddenBlocked := h.app.Chat.Tools.Run(context.Background(), "read",
			`{"path":"`+strings.ReplaceAll(hidden, `\`, `\\`)+`"}`)
		if !hiddenBlocked.IsError {
			t.Fatalf("停用技能的正文应读不到，实际：%s", hiddenBlocked.Text)
		}

		// 正文可读：模型按索引上的路径 read 得到内容；越界的非技能路径照旧被拒。
		env, code = h.call("POST", "/skills/toggle", domain.SkillToggleREQ{
			SkillPath: filepath.Join(h.layout.SkillsOffDir, "prompt-probe", "SKILL.md"), SkillEnabled: true,
		})
		h.mustOK(env, code, "toggle-on", nil)
		if _, err := h.app.Chat.Send("", "让索引刷新", nil); err != nil {
			t.Fatalf("重新开跑失败：%v", err)
		}
		waitIdle(t, h)
		read := h.app.Chat.Tools.Run(context.Background(), "read", `{"path":"`+escapedActive+`"}`)
		if read.IsError {
			t.Fatalf("启用技能的 SKILL.md 应可读：%s", read.Text)
		}
		if !strings.Contains(read.Text, "验证索引进提示") {
			t.Fatalf("读到的应是技能正文，实际：%s", read.Text)
		}
		outside := filepath.Join(t.TempDir(), "outside.txt")
		writeFile(t, outside, "越界")
		escapedOutside := strings.ReplaceAll(outside, `\`, `\\`)
		denied := h.app.Chat.Tools.Run(context.Background(), "read", `{"path":"`+escapedOutside+`"}`)
		if !denied.IsError {
			t.Fatalf("工作区外的非技能路径应被拒，实际读到：%s", denied.Text)
		}
		if !strings.Contains(denied.Text, "路径不在工作区内") {
			t.Fatalf("拒绝文案应说清是路径越界，实际：%s", denied.Text)
		}
		// 只读是真的只读：write 不能借这条豁免覆盖技能正文。
		overwrite := h.app.Chat.Tools.Run(context.Background(), "write",
			`{"path":"`+escapedActive+`","content":"改掉"}`)
		if !overwrite.IsError {
			t.Fatalf("技能正文不该被 write 覆盖：%s", overwrite.Text)
		}
		if raw, err := os.ReadFile(active); err != nil || strings.Contains(string(raw), "改掉") {
			t.Fatalf("技能正文应保持原样：%v %q", err, string(raw))
		}
	})
	// 开关=搬目录+记账、重置后 SKILL.md 仍是可读文件、删除清整目录、两区重复只出一行。
	t.Run("生命周期", func(t *testing.T) {
		assets := repoAssetsDir(t)
		app, skillLayout, store := catalogHarness(t, assets)
		item := firstRecommendedSkill(t)
		active := filepath.Join(skillLayout.SkillsDir, item.SkillDirName)
		hidden := filepath.Join(skillLayout.SkillsOffDir, item.SkillDirName)

		// 壳不自动落位——技能目录只在他点了安装后才出现东西。
		if pkg.Exists(active) || pkg.Exists(hidden) {
			t.Fatalf("壳不该自动把 %s 写进技能目录", item.SkillDirName)
		}

		// 安装：点一次才落一份，再点一次拒绝（已有就不覆盖）。
		if err := app.Skills.Install(item.SkillDirName); err != nil {
			t.Fatalf("安装技能失败：%v", err)
		}
		if !pkg.Exists(filepath.Join(active, "SKILL.md")) {
			t.Fatalf("安装后 %s 应落在启用区", item.SkillDirName)
		}
		if err := app.Skills.Install(item.SkillDirName); err == nil {
			t.Error("重复安装应被拒绝，而不是默默覆盖用户的改动")
		}

		// 安装不覆盖用户已有内容。
		const userEdit = "---\nname: x\n---\n用户改过\n"
		probe := filepath.Join(active, "SKILL.md")
		if err := os.WriteFile(probe, []byte(userEdit), 0o644); err != nil {
			t.Fatalf("写探针文件失败：%v", err)
		}
		_ = app.Skills.Install(item.SkillDirName)
		raw, err := os.ReadFile(probe)
		if err != nil {
			t.Fatalf("重读失败：%v", err)
		}
		if string(raw) != userEdit {
			t.Fatal("安装不能覆盖用户已改过的技能文件")
		}

		// 开关 = 搬目录 + 台账记账。
		if err := app.Skills.Toggle(filepath.Join(active, "SKILL.md"), false); err != nil {
			t.Fatalf("停用技能失败：%v", err)
		}
		if pkg.Exists(active) || !pkg.Exists(hidden) {
			t.Fatal("停用后目录应从启用区搬到禁用区")
		}
		if enabled, ok := store.SkillEnabled(item.SkillDirName); !ok || enabled {
			t.Fatalf("停用应记账为 false，实际 ok=%v enabled=%v", ok, enabled)
		}
		if err := app.Skills.Toggle(filepath.Join(hidden, "SKILL.md"), true); err != nil {
			t.Fatalf("启用技能失败：%v", err)
		}
		if pkg.Exists(hidden) || !pkg.Exists(active) {
			t.Fatal("启用后目录应从禁用区搬回启用区")
		}
		if enabled, ok := store.SkillEnabled(item.SkillDirName); !ok || !enabled {
			t.Fatalf("启用应记账为 true，实际 ok=%v enabled=%v", ok, enabled)
		}

		// 重置后 SKILL.md 必须仍是可读文件，不能变成目录。
		onDir := active
		onFile := filepath.Join(onDir, "SKILL.md")
		if err := app.Skills.Reset(onFile); err != nil {
			t.Fatalf("重置失败：%v", err)
		}
		info, err := os.Stat(onFile)
		if err != nil {
			t.Fatalf("重置后 SKILL.md 不存在：%v", err)
		}
		if info.IsDir() {
			t.Fatal("重置后 SKILL.md 变成了目录")
		}
		if _, err := os.ReadFile(onFile); err != nil {
			t.Fatalf("重置后 SKILL.md 读不出来：%v", err)
		}

		// 删除清整个目录；传目录路径时拒绝且不动盘。
		if err := app.Skills.Delete(onFile); err != nil {
			t.Fatalf("删除失败：%v", err)
		}
		if pkg.Exists(onDir) {
			t.Errorf("删除后技能目录仍在：%s", onDir)
		}
		before, _ := os.ReadDir(skillLayout.SkillsDir)
		if err := app.Skills.Delete(skillLayout.SkillsDir); err == nil {
			t.Error("传入目录路径时应当拒绝删除")
		}
		after, _ := os.ReadDir(skillLayout.SkillsDir)
		if len(before) != len(after) {
			t.Errorf("被拒的删除不应改动技能目录：前 %d 个，后 %d 个", len(before), len(after))
		}

		// 同一个技能在两个区都有时清单只出一行，不重复展示。
		if err := app.Skills.Install(item.SkillDirName); err != nil {
			t.Fatalf("重新安装技能失败：%v", err)
		}
		if err := pkg.EnsureDir(hidden); err != nil {
			t.Fatalf("建目录失败：%v", err)
		}
		raw, err = os.ReadFile(filepath.Join(active, "SKILL.md"))
		if err != nil {
			t.Fatalf("读原件失败：%v", err)
		}
		if err := os.WriteFile(filepath.Join(hidden, "SKILL.md"), raw, 0o644); err != nil {
			t.Fatalf("写副本失败：%v", err)
		}
		rowCount := map[string]int{}
		for _, row := range app.Skills.List().Skills {
			rowCount[row.SkillDirName]++
		}
		for name, count := range rowCount {
			if count > 1 {
				t.Errorf("技能 %s 在清单里出现了 %d 次", name, count)
			}
		}
	})
}
