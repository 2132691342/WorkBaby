package service

// stash 全套：存、取、列、丢。暂存区之外再要一层「先收起来」，
// 改动到一半不能提交又必须切分支时，它是唯一无损出口。

import (
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// StashPush 把全部改动（含未跟踪文件）收进 stash。
func (g *Git) StashPush(req domain.GitStashREQ) (domain.GitStatusRESP, error) {
	args := []string{"stash", "push", "--include-untracked"}
	if msg := strings.TrimSpace(req.StashMessage); msg != "" {
		args = append(args, "-m", msg)
	}
	if _, err := g.runWriteOrFail(args...); err != nil {
		return domain.GitStatusRESP{}, err
	}
	return g.Status(), nil
}

// StashPop 恢复最新一条并从栈里删掉；冲突是正常结果，交给界面走解决流程。
func (g *Git) StashPop() (domain.GitStatusRESP, error) {
	if _, err := g.runWriteOrFail("stash", "pop"); err != nil {
		status := g.Status()
		if status.HasConflict {
			return status, nil
		}
		return domain.GitStatusRESP{}, err
	}
	return g.Status(), nil
}

// StashList 列出 stash，最新在前。
func (g *Git) StashList() domain.GitStashListRESP {
	out := domain.GitStashListRESP{Stashes: []domain.GitStashVO{}}
	text, err := g.runOrFail("stash", "list", "--format=%gd%x1f%gs")
	if err != nil {
		return out
	}
	for _, line := range pkg.SplitLines(text) {
		ref, subject, ok := strings.Cut(line, "\x1f")
		if !ok {
			continue
		}
		out.Stashes = append(out.Stashes, domain.GitStashVO{
			StashRef:     strings.TrimSpace(ref),
			StashSubject: strings.TrimSpace(subject),
		})
	}
	return out
}

// StashDrop 丢掉指定 stash：不可逆，确认词必须等于引用名。
func (g *Git) StashDrop(req domain.GitStashREQ) (domain.GitStatusRESP, error) {
	ref := strings.TrimSpace(req.StashRef)
	if ref == "" {
		return domain.GitStatusRESP{}, pkg.Err(pkg.CodeBadRequest, "缺少 stash 引用名")
	}
	if req.Confirm != ref {
		return domain.GitStatusRESP{}, pkg.Err(pkg.CodeBadRequest, "丢弃 stash 需要确认引用名："+ref)
	}
	if _, err := g.runWriteOrFail("stash", "drop", ref); err != nil {
		return domain.GitStatusRESP{}, err
	}
	return g.Status(), nil
}
