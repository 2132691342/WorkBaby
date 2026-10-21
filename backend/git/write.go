// 受控写入：白名单动作全部显式列参数（specs/08）；写入不可逆，
// 因此不解析不猜测——参数逐个校验，命中禁止清单一律拒绝。
package git

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// writeTimeout 写操作超时：commit 可能触发钩子，push 可能真的慢，都比读给得足。
const writeTimeout = 60 * time.Second

// pushTimeout 推送超时：远端慢时一分钟都算客气，超时报错让用户自己重试。
const pushTimeout = 120 * time.Second

// writeRun 执行一条 git 写命令；失败折成 1019，归因合并 stdout 与 stderr（git 的失败原因
// 大多写在 stdout，只看 stderr 只会报 exit status 1），git 原文进 details。
func writeRun(dir string, timeout time.Duration, args ...string) (result, error) {
	return writeRunEnv(dir, timeout, nil, args...)
}

// writeRunEnv 同 writeRun，但可以追加环境变量。
func writeRunEnv(dir string, timeout time.Duration, env []string, args ...string) (result, error) {
	res, err := runEnv(dir, timeout, env, args...)
	if err == nil {
		return res, nil
	}
	if !res.failed() {
		// runEnv 在拿到退出码前就失败了（超时 / 起不了进程）：原样上交，别改语义。
		return res, err
	}
	return res, pkg.New("1019", "这个 Git 操作没有完成", res.output())
}

// requireScope 统一的前置校验：装了 git、dir 在仓库里，并交出定位结果。
func requireScope(dir string) (*scope, error) {
	if !Available() {
		return nil, pkg.New("1015", "这台电脑上还没有 Git，装好后就能看历史", "")
	}
	sc, err := openScope(dir)
	if err != nil {
		return nil, err
	}
	if sc == nil {
		return nil, pkg.New("1015", "这个工作目录还不是 Git 仓库，初始化后才能写入", "")
	}
	return sc, nil
}

// Stage 把 paths 放进待提交区；paths 为空表示全部改动。
func Stage(dir string, paths []string) (*domain.GitStatusVO, error) {
	sc, err := requireScope(dir)
	if err != nil {
		return nil, err
	}
	args, err := sc.pathsArg(paths)
	if err != nil {
		return nil, err
	}
	if _, err := writeRun(sc.root, writeTimeout, append([]string{"add", "--"}, args...)...); err != nil {
		return nil, err
	}
	return Status(dir)
}

// Unstage 把 paths 从待提交区取出；只动索引，不动工作区文件。
func Unstage(dir string, paths []string) (*domain.GitStatusVO, error) {
	sc, err := requireScope(dir)
	if err != nil {
		return nil, err
	}
	args, err := sc.pathsArg(paths)
	if err != nil {
		return nil, err
	}
	// 首次提交前还没有 HEAD，reset HEAD 会失败；那时用不带 HEAD 的 --cached 形式清索引。
	prefix := []string{"reset", "HEAD"}
	if !sc.hasHead() {
		prefix = []string{"reset"}
	}
	cmd := append(prefix, "--")
	cmd = append(cmd, args...)
	if _, err := writeRun(sc.root, writeTimeout, cmd...); err != nil {
		return nil, err
	}
	return Status(dir)
}

// ResolveConflict 把冲突文件标记为已解决（git add 之后冲突状态消失，但还没提交）。
// 未跟踪的新文件可能不在「冲突清单」里，因此不按冲突名单限制，只按路径是否落进工作区。
func ResolveConflict(dir string, paths []string) (*domain.GitStatusVO, error) {
	return Stage(dir, paths)
}

// Discard 丢弃 paths 在工作区的改动（tracked 回到 HEAD，未跟踪直接删）。
// 这是可逆性最低的读侧动作，因此必须有显式路径：空 paths 不允许。
func Discard(dir string, paths []string) (*domain.GitStatusVO, error) {
	sc, err := requireScope(dir)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, pkg.New("1018", "丢弃改动必须显式指定文件", "不提供「全部丢弃」：那等于一次不可逆的清空")
	}
	args, err := sc.pathsArg(paths)
	if err != nil {
		return nil, err
	}
	st, err := Status(dir)
	if err != nil {
		return nil, err
	}
	// 分成两边：已跟踪的走 checkout 回 HEAD，未跟踪的只能删文件。
	var tracked, untracked []string
	want := map[string]bool{}
	for _, rel := range args {
		want[rel] = true
	}
	for _, f := range st.Files {
		rootRel := sc.relFromRoot(f.Path)
		if !want[rootRel] {
			continue
		}
		if f.Status == domain.GitChangeUntracked {
			untracked = append(untracked, rootRel)
		} else {
			tracked = append(tracked, rootRel)
		}
	}
	if len(tracked) > 0 {
		// unstaged 走 checkout --；已入栈的还要先 reset，否则工作区回不去。
		if _, err := writeRun(sc.root, writeTimeout, append([]string{"checkout", "--"}, tracked...)...); err != nil {
			return nil, err
		}
	}
	for _, rel := range untracked {
		full, jerr := pkg.SafeJoin(sc.root, rel)
		if jerr != nil {
			return nil, jerr
		}
		if err := os.RemoveAll(full); err != nil {
			return nil, pkg.Wrap("1007", "删除未跟踪文件失败", err)
		}
	}
	return Status(dir)
}

// Commit 用用户填的信息提交；信息非空且不超上限，提交后返回重读的状态与新 hash。
func Commit(dir, message string, amend bool) (*domain.GitCommitRESP, error) {
	sc, err := requireScope(dir)
	if err != nil {
		return nil, err
	}
	msg := strings.TrimSpace(message)
	if msg == "" {
		return nil, pkg.New("1016", "提交信息还没填，写一句这次改了什么", "")
	}
	if len([]rune(msg)) > domain.GitCommitMsgLimit {
		return nil, pkg.New("1016", "提交信息太长了，缩短到一句摘要", "")
	}
	args := []string{"commit", "-m", msg}
	if amend {
		// --amend 改写已有提交：只允许配 -m，不允许 --no-verify 跳过钩子。
		args = append(args, "--amend")
	}
	// 不带 --no-verify：跳过钩子不该由界面替用户决定。
	if _, err := writeRun(sc.root, writeTimeout, args...); err != nil {
		return nil, commitFailure(sc, err)
	}
	st, err := Status(dir)
	if err != nil {
		return nil, err
	}
	return &domain.GitCommitRESP{Status: st, Hash: sc.headHash()}, nil
}

// commitFailure 把 git 的多种拒绝形态翻成用户能照着做的下一步。
// 判定靠输出里的稳定关键词（这些串不受界面语言影响），不靠退出码——它们全是 1。
func commitFailure(sc *scope, err error) error {
	text := strings.ToLower(errText(err))
	switch {
	case strings.Contains(text, "nothing to commit"), strings.Contains(text, "no changes added to commit"),
		strings.Contains(text, "nothing added to commit"):
		return pkg.New("1017", "没有可提交的改动，先勾选文件入栈", "")
	case strings.Contains(text, "author identity unknown"), strings.Contains(text, "unable to auto-detect email"):
		return pkg.New("1020", "这台电脑上还没配置 Git 用户名与邮箱，配好后才能提交",
			"在设置页的 Git 一节填入用户名与邮箱，或执行 git config --global user.name/user.email")
	case strings.Contains(text, "unmerged"), strings.Contains(text, "conflict"):
		return pkg.New("1021", "还有文件没解决冲突，先在改动列表里逐个标记已解决", "")
	case strings.Contains(text, "hook"), strings.Contains(text, "pre-commit"):
		return pkg.New("1019", "提交钩子拒绝了这次提交", errText(err))
	}
	return err
}

// CreateBranch 新建分支并切过去；名字走白名单字符集。
func CreateBranch(dir, name string) (*domain.GitStatusVO, error) {
	sc, err := requireScope(dir)
	if err != nil {
		return nil, err
	}
	if !validBranchName(name) {
		return nil, pkg.New("1018", "分支名不合法",
			"只允许字母、数字、点、下划线、斜杠与连字符，且不能以连字符开头")
	}
	if _, err := writeRun(sc.root, writeTimeout, "checkout", "-b", strings.TrimSpace(name)); err != nil {
		return nil, err
	}
	return Status(dir)
}

// SwitchBranch 切到已有分支；不带 --force，避免用索引覆盖工作区改动。
func SwitchBranch(dir, name string) (*domain.GitStatusVO, error) {
	sc, err := requireScope(dir)
	if err != nil {
		return nil, err
	}
	if !validBranchName(name) {
		return nil, pkg.New("1018", "分支名不合法", name)
	}
	if _, err := writeRun(sc.root, writeTimeout, "checkout", strings.TrimSpace(name)); err != nil {
		return nil, checkoutFailure(err)
	}
	return Status(dir)
}

// DeleteBranch 删除已合并的本地分支；未合并的分支 git 会拒绝（这里不提供 -D）。
func DeleteBranch(dir, name string) (*domain.GitStatusVO, error) {
	sc, err := requireScope(dir)
	if err != nil {
		return nil, err
	}
	if !validBranchName(name) {
		return nil, pkg.New("1018", "分支名不合法", name)
	}
	if _, err := writeRun(sc.root, writeTimeout, "branch", "-d", strings.TrimSpace(name)); err != nil {
		return nil, pkg.New("1019", "这个分支删不掉", errText(err)+"\n未合并的分支要先用自己的 Git 工具处理")
	}
	return Status(dir)
}

// Merge 把 name 合并进当前分支；rebase 为真时用变基。
// 冲突不算失败：git 留下未完成的合并，界面要接着引导解决。
func Merge(dir, name string, rebase bool) (*domain.GitStatusVO, error) {
	sc, err := requireScope(dir)
	if err != nil {
		return nil, err
	}
	if !validBranchName(name) {
		return nil, pkg.New("1018", "分支名不合法", name)
	}
	return mergeLike(dir, sc, rebase, "merge", "rebase", strings.TrimSpace(name))
}

// mergeLike 执行 merge 或 rebase 并按结果决定是否把「有冲突」当成功返回。
func mergeLike(dir string, sc *scope, rebase bool, mergeCmd, rebaseCmd, target string) (*domain.GitStatusVO, error) {
	args := []string{mergeCmd, "--no-edit", target}
	if rebase {
		args = []string{rebaseCmd, target}
	}
	if _, err := writeRun(sc.root, writeTimeout, args...); err != nil {
		st, serr := Status(dir)
		// 冲突时 git 退出码是 1，但仓库里已经留下未完成的合并：这是可继续的状态，不是失败。
		if serr == nil && st.Conflict {
			return st, nil
		}
		return nil, mergeFailure(err)
	}
	return Status(dir)
}

// mergeFailure 把合并失败翻成可执行的下一步。
func mergeFailure(err error) error {
	text := strings.ToLower(errText(err))
	switch {
	case strings.Contains(text, "local changes"), strings.Contains(text, "would be overwritten"):
		return pkg.New("1019", "工作区有未提交的改动，先提交或丢弃后再合并", errText(err))
	case strings.Contains(text, "not something we can merge"), strings.Contains(text, "not a valid"):
		return pkg.New("1018", "找不到这个分支", errText(err))
	}
	return err
}

// checkoutFailure 把切分支失败翻成可执行的下一步。
func checkoutFailure(err error) error {
	text := strings.ToLower(errText(err))
	if strings.Contains(text, "local changes") || strings.Contains(text, "would be overwritten") {
		return pkg.New("1019", "工作区有未提交的改动，先提交或丢弃后再切换分支", errText(err))
	}
	return err
}

// UpdateState 继续或中止一个未完成的合并 / 变基。
func UpdateState(dir, action string) (*domain.GitStatusVO, error) {
	sc, err := requireScope(dir)
	if err != nil {
		return nil, err
	}
	st := sc.state()
	if st.Kind == domain.GitStateNone {
		return nil, pkg.New("1018", "当前没有进行中的合并或变基", "")
	}
	verb := strings.TrimSpace(action)
	if verb != domain.GitStateContinue && verb != domain.GitStateAbort {
		return nil, pkg.New("1018", "只支持继续或中止", verb)
	}
	cmd := stateCommand(st.Kind, verb)
	if len(cmd) == 0 {
		return nil, pkg.New("1018", "这个中间状态不支持该操作", st.Kind)
	}
	if _, err := writeRun(sc.root, writeTimeout, cmd...); err != nil {
		text := strings.ToLower(errText(err))
		if verb == domain.GitStateContinue && strings.Contains(text, "unmerged") {
			return nil, pkg.New("1021", "还有文件没解决冲突，先在改动列表里逐个标记已解决", "")
		}
		return nil, err
	}
	return Status(dir)
}

// stateCommand 按中间状态与动作给出具体的 git 子命令。
func stateCommand(kind, action string) []string {
	switch kind {
	case domain.GitStateMerge, domain.GitStateCherryPick, domain.GitStateRevert:
		if action == domain.GitStateContinue {
			return []string{"commit", "--no-edit"}
		}
		// 三种状态各有一个 --abort，拣选与回退是 cherry-pick --abort / revert --abort
		return []string{kind, "--abort"}
	case domain.GitStateRebase:
		if action == domain.GitStateContinue {
			// 不设 GIT_EDITOR=: 时变基会拉起编辑器，界面会永远等下去。
			return []string{"rebase", "--continue"}
		}
		return []string{"rebase", "--abort"}
	}
	return nil
}

// Pull 从远端拉取并合入当前分支。
func Pull(dir string, remote, branch string, rebase bool) (*domain.GitStatusVO, error) {
	sc, err := requireScope(dir)
	if err != nil {
		return nil, err
	}
	args := []string{"pull", "--no-edit"}
	if rebase {
		args = []string{"pull", "--rebase"}
	}
	if r := strings.TrimSpace(remote); r != "" {
		if !validRemoteName(r) {
			return nil, pkg.New("1018", "远端名不合法", r)
		}
		args = append(args, r)
		if b := strings.TrimSpace(branch); b != "" {
			if !validBranchName(b) {
				return nil, pkg.New("1018", "分支名不合法", b)
			}
			args = append(args, b)
		}
	}
	// 拉取过程中禁止 git 弹出交互式提问（凭据 / 编辑器），否则界面会一直等。
	if _, err := writeRunEnv(sc.root, pushTimeout, gitNonInteractive(), args...); err != nil {
		st, serr := Status(dir)
		if serr == nil && st.Conflict {
			return st, nil
		}
		return nil, pushFailure(err)
	}
	return Status(dir)
}

// Push 推送当前分支。第一次推送要建立上游；强制推送分级，纯 --force 必须输分支名确认。
func Push(dir string, req domain.GitPushREQ) (*domain.GitStatusVO, error) {
	sc, err := requireScope(dir)
	if err != nil {
		return nil, err
	}
	cur := sc.branch()
	if cur == "" {
		return nil, pkg.New("1018", "还没有提交，先提交一次再推送", "")
	}
	remote := strings.TrimSpace(req.Remote)
	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = cur
	}
	if !validBranchName(branch) {
		return nil, pkg.New("1018", "分支名不合法", branch)
	}
	if remote != "" && !validRemoteName(remote) {
		return nil, pkg.New("1018", "远端名不合法", remote)
	}
	// 没指定远端时按上游推断；再没有就只认唯一远端，多于一个必须让用户选。
	if remote == "" {
		if up := sc.upstream(); up != "" {
			remote, _ = splitUpstream(up)
		} else {
			remotes := sc.remoteNames()
			switch len(remotes) {
			case 1:
				remote = remotes[0]
			case 0:
				return nil, pkg.New("1023", "这个仓库还没有配置远端",
					"先在设置里添加一个远端地址（git remote add origin <url>）")
			default:
				return nil, pkg.New("1022", "这个仓库有多个远端，请选择推送目标", strings.Join(remotes, ", "))
			}
		}
	}

	args := []string{"push"}
	switch strings.TrimSpace(req.Force) {
	case "", domain.GitForceNone:
	case domain.GitForceLease:
		// 故意不先 fetch：租约比的是本地远端跟踪引用，那个引用只在上一次同步时更新过。
		// 先 fetch 会把租约变成「无条件覆盖」——租约的全部价值就在于「我上次同步之后没人推过」。
		args = append(args, "--force-with-lease")
	case domain.GitForceTrue:
		// 无条件强制推送会静默覆盖别人的提交，必须输入分支名确认。
		if strings.TrimSpace(req.Confirm) != branch {
			return nil, pkg.New("1018", "强制推送需要输入分支名确认", "请输入 "+branch)
		}
		args = append(args, "--force")
	default:
		return nil, pkg.New("1018", "不认识的强制推送级别", req.Force)
	}

	// 没上游或显式要求时建立跟踪，否则第一次推送永远会被 git 拒。
	if req.SetUpstream || sc.upstream() == "" {
		args = append(args, "--set-upstream")
	}
	args = append(args, remote, branch)

	env := gitNonInteractive()
	if _, err := writeRunEnv(sc.root, pushTimeout, env, args...); err != nil {
		return nil, pushFailure(err)
	}
	return Status(dir)
}

// Fetch 取回远端的最新状态，不改工作区也不改本地分支。
// 界面需要它：推送被拒之后，用户得先看见远端多了什么才能决定拉取还是覆盖。
func Fetch(dir, remote string) (*domain.GitStatusVO, error) {
	sc, err := requireScope(dir)
	if err != nil {
		return nil, err
	}
	target := strings.TrimSpace(remote)
	args := []string{"fetch", "--prune"}
	if target != "" {
		if !validRemoteName(target) {
			return nil, pkg.New("1018", "远端名不合法", target)
		}
		args = append(args, target)
	} else if remotes := sc.remoteNames(); len(remotes) == 1 {
		args = append(args, remotes[0])
	}
	if _, err := writeRunEnv(sc.root, pushTimeout, gitNonInteractive(), args...); err != nil {
		return nil, pushFailure(err)
	}
	return Status(dir)
}

// gitNonInteractive 让 git 在缺凭据时直接失败而不是等终端输入：
// GUI 里没有终端，等下去的结局是界面卡到超时。
func gitNonInteractive() []string {
	return []string{"GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=:", "GIT_SEQUENCE_EDITOR=:", "GCM_INTERACTIVE=never"}
}

// pushFailure 把推送失败翻成可执行的下一步；凭据与网络问题各给不同的处置。
func pushFailure(err error) error {
	text := strings.ToLower(errText(err))
	switch {
	case strings.Contains(text, "non-fast-forward"), strings.Contains(text, "fetch first"),
		strings.Contains(text, "updates were rejected"):
		return pkg.New("1024", "远端有本地没有的提交，推送被拒绝",
			"先拉取合并；确认要覆盖远端时用强制推送（会改写远端历史）")
	case strings.Contains(text, "stale info"):
		return pkg.New("1027", "远端在你上次同步之后又变了，带租约的强制推送已拒绝",
			"先「取回远端」看清楚别人推了什么；确认之后再用无条件强制推送覆盖")
	case strings.Contains(text, "no upstream branch"):
		return pkg.New("1022", "当前分支还没有上游分支", "推送时勾选「建立上游」")
	case strings.Contains(text, "authentication failed"), strings.Contains(text, "could not read username"),
		strings.Contains(text, "terminal prompts disabled"), strings.Contains(text, "permission denied"):
		return pkg.New("1025", "远端拒绝了身份验证",
			"这个仓库需要凭据（HTTPS 令牌或 SSH 密钥），请在自己的终端里配置一次")
	case strings.Contains(text, "could not resolve host"), strings.Contains(text, "network"),
		strings.Contains(text, "connection refused"), strings.Contains(text, "timed out"):
		return pkg.New("1026", "连不上远端，检查网络或远端地址", errText(err))
	case strings.Contains(text, "no configured push destination"), strings.Contains(text, "no remote"):
		return pkg.New("1023", "这个仓库还没有配置远端", "请先添加一个远端地址")
	}
	return err
}

// Init 在当前目录初始化仓库；已存在仓库时不做任何事（不覆盖用户的分支配置）。
func Init(dir string, branch string) (*domain.GitStatusVO, error) {
	if !Available() {
		return nil, pkg.New("1015", "这台电脑上还没有 Git，装好后才能初始化仓库", "")
	}
	if !pkg.DirExists(dir) {
		return nil, pkg.New("1003", "工作目录不存在", dir)
	}
	args := []string{"init"}
	if b := strings.TrimSpace(branch); b != "" {
		if !validBranchName(b) {
			return nil, pkg.New("1018", "分支名不合法", b)
		}
		args = append(args, "--initial-branch="+b)
	}
	if _, err := writeRun(dir, writeTimeout, args...); err != nil {
		return nil, err
	}
	return Status(dir)
}

// AddRemote 添加一个远端地址。
func AddRemote(dir, name, url string) (*domain.GitStatusVO, error) {
	sc, err := requireScope(dir)
	if err != nil {
		return nil, err
	}
	name, url = strings.TrimSpace(name), strings.TrimSpace(url)
	if !validRemoteName(name) {
		return nil, pkg.New("1018", "远端名不合法", "只允许字母、数字与 . _ -")
	}
	if !validRemoteURL(url) {
		return nil, pkg.New("1018", "远端地址不合法", "支持 https:// / ssh:// / git@host:path / 本地目录")
	}
	if _, err := writeRun(sc.root, writeTimeout, "remote", "add", name, url); err != nil {
		return nil, pkg.New("1019", "添加远端失败", errText(err)+"\n同名远端可能已存在")
	}
	return Status(dir)
}

// headHash 取当前 HEAD 的完整 hash，供提交后跳到历史页那一条。
func (s *scope) headHash() string {
	res, err := run(s.root, statusTimeout, "rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(res.stdout)
}

// pathsArg 把界面给的工作区相对路径收成仓库根相对的 pathspec；越界直接报错，不降级成「全部」。
func (s *scope) pathsArg(paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// 落进仓库的路径先过安全检查：绝对路径、盘符、`..` 一律拒绝，不降级成「全部」。
		if !validRepoRel(p) {
			return nil, pkg.New("1004", "路径越出工作目录", p)
		}
		out = append(out, s.relFromRoot(p))
	}
	if len(out) == 0 {
		// 空表示「工作区内全部改动」：限定在 prefix 下，不碰仓库别处。
		// pathspec 不能是空串（git 会直接报 fatal），工作区就是仓库根时用 `.`。
		target := s.relFromRoot("")
		if target == "" {
			target = "."
		}
		return []string{target}, nil
	}
	return out, nil
}

// validRepoRel 校验界面给的相对路径：拒绝绝对路径、盘符、`..` 与反斜杠越界。
func validRepoRel(p string) bool {
	p = filepath.ToSlash(strings.TrimSpace(p))
	if p == "" {
		return true
	}
	if strings.HasPrefix(p, "/") || strings.Contains(p, "://") {
		return false
	}
	if len(p) > 1 && p[1] == ':' {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

// validBranchName 分支名白名单：拒绝以 `-` 开头（会被当成选项）、拒绝 `..` 与空白。
// 斜杠允许：`feature/x` 是常见分层；但连续斜杠与结尾斜杠会让 git 造出嵌套 ref。
func validBranchName(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 128 || strings.HasPrefix(s, "-") {
		return false
	}
	if strings.Contains(s, "..") || strings.ContainsAny(s, " \t\n\\~^:?*[") {
		return false
	}
	if strings.HasSuffix(s, "/") || strings.Contains(s, "//") || strings.HasSuffix(s, ".lock") {
		return false
	}
	if strings.HasPrefix(s, "/") {
		return false
	}
	return true
}

// validRemoteName 远端名白名单：git 允许更宽的字符集，这里只放常见的几种。
func validRemoteName(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 64 || strings.HasPrefix(s, "-") {
		return false
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '-'
		if !ok {
			return false
		}
	}
	return true
}

// validRemoteURL 远端地址白名单：只放 git 认得的四种形态，拒绝任何以 `-` 开头的串
// （那会被当成 git 的选项），也拒绝带空白的串。
func validRemoteURL(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 2048 || strings.HasPrefix(s, "-") {
		return false
	}
	if strings.ContainsAny(s, " \t\n\r") {
		return false
	}
	prefixes := []string{"https://", "http://", "ssh://", "git://", "file://", "git@"}
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	// 本地路径形态：盘符或 / 开头，或纯相对目录（不含 : 以免误吞 scp 风格）
	if strings.HasPrefix(s, "/") || (len(s) > 1 && s[1] == ':') {
		return true
	}
	return !strings.Contains(s, ":") && !strings.Contains(s, "..")
}

// splitUpstream 把 `origin/main` 拆成远端名与分支名；`feature/x` 这类带斜杠的分支要保住。
func splitUpstream(up string) (string, string) {
	name, rest, ok := strings.Cut(up, "/")
	if !ok {
		return "", up
	}
	return name, rest
}

// errText 取 AppError 里留给用户的原文（git 自己的输出），没有就退回 Error()。
func errText(err error) string {
	if err == nil {
		return ""
	}
	if ae, ok := err.(*pkg.AppError); ok {
		if ae.Details != "" {
			return ae.Details
		}
		return ae.Message
	}
	return err.Error()
}
