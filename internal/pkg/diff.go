// 文件：行级 unified diff（LCS 算法），仅用于工具结果的人眼审阅。

package pkg

import (
	"strconv"
	"strings"
)

// maxDiffLines 参与 LCS 的最大行数（双向合计）。
// LCS 是 O(n*m) 时间与空间，超限退化为统计摘要——diff 只服务于人眼审阅，
// 不值得为超大文件拖慢工具执行。
const maxDiffLines = 1200

// diffContext 每个 hunk 上下保留的上下文行数（unified 格式惯例）。
const diffContext = 3

// DiffResult 行级差异结果。
type DiffResult struct {
	Text      string // unified diff 文本；Truncated 时为空
	Added     int    // 新增行数
	Removed   int    // 删除行数
	Truncated bool   // 超过 maxDiffLines，未生成正文
}

// UnifiedDiff 比较 before/after 生成 unified diff（路径仅用于文本头，可为空）。
func UnifiedDiff(path, before, after string) DiffResult {
	a := splitLines(before)
	b := splitLines(after)
	if len(a)+len(b) > maxDiffLines {
		return DiffResult{Added: len(b), Removed: len(a), Truncated: true}
	}
	ops := lcsOps(a, b)
	var res DiffResult
	var sb strings.Builder
	if path != "" {
		sb.WriteString("--- a/")
		sb.WriteString(path)
		sb.WriteString("\n+++ b/")
		sb.WriteString(path)
		sb.WriteString("\n")
	}
	for _, h := range hunks(ops, a, b) {
		sb.WriteString(h)
	}
	res.Text = sb.String()
	for _, op := range ops {
		switch op.kind {
		case opAdd:
			res.Added++
		case opDel:
			res.Removed++
		}
	}
	return res
}

// splitLines 拆分行；丢弃末尾空串（文件以换行结尾时的产物）。
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// ===== LCS 编辑脚本 =====

type opKind int8

const (
	opKeep opKind = iota
	opAdd
	opDel
)

type op struct {
	kind opKind
	i    int // a 的下标
	j    int // b 的下标
}

// lcsOps 动态规划求最长公共子序列，回溯出 keep/add/del 序列。
func lcsOps(a, b []string) []op {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	ops := make([]op, 0, n+m)
	var walk func(i, j int)
	walk = func(i, j int) {
		if i < n && j < m && a[i] == b[j] {
			walk(i+1, j+1)
			ops = append(ops, op{kind: opKeep, i: i, j: j})
			return
		}
		if j < m && (i == n || dp[i][j+1] >= dp[i+1][j]) {
			walk(i, j+1)
			ops = append(ops, op{kind: opAdd, j: j})
			return
		}
		if i < n {
			walk(i+1, j)
			ops = append(ops, op{kind: opDel, i: i})
		}
	}
	walk(0, 0)
	return ops
}

// hunks 把编辑脚本按 diffContext 聚合成 unified hunk 文本。
func hunks(ops []op, a, b []string) []string {
	changed := make([]bool, len(ops))
	for i, o := range ops {
		if o.kind != opKeep {
			changed[i] = true
			// 上下文窗口内的 keep 也纳入该 hunk
			for k := 1; k <= diffContext; k++ {
				if i-k >= 0 {
					changed[i-k] = true
				}
				if i+k < len(ops) {
					changed[i+k] = true
				}
			}
		}
	}
	var out []string
	ai, bi := 0, 0
	for start := 0; start < len(ops); {
		if !changed[start] {
			advance(&ai, &bi, ops[start])
			start++
			continue
		}
		end := start
		for end < len(ops) && changed[end] {
			end++
		}
		// hunk 起止行号：从 start 回溯到该段的首个操作
		startA, startB := ai, bi
		var sb strings.Builder
		body := 0
		for i := start; i < end; i++ {
			switch ops[i].kind {
			case opKeep:
				sb.WriteString("  " + a[ops[i].i] + "\n")
			case opAdd:
				sb.WriteString("+" + b[ops[i].j] + "\n")
				body++
			case opDel:
				sb.WriteString("-" + a[ops[i].i] + "\n")
				body++
			}
		}
		if body > 0 {
			out = append(out, hunkHeader(startA+1, countOps(ops[start:end], opKeep, opDel),
				startB+1, countOps(ops[start:end], opKeep, opAdd))+sb.String())
		}
		for i := start; i < end; i++ {
			advance(&ai, &bi, ops[i])
		}
		start = end
	}
	return out
}

// advance 推进 a/b 的原始行下标。
func advance(ai, bi *int, o op) {
	switch o.kind {
	case opKeep:
		*ai++
		*bi++
	case opAdd:
		*bi++
	case opDel:
		*ai++
	}
}

// countOps 统计一段操作中 keep 与指定 kind 的总行数（hunk 长度）。
func countOps(ops []op, keep, other opKind) int {
	n := 0
	for _, o := range ops {
		if o.kind == keep || o.kind == other {
			n++
		}
	}
	return n
}

// hunkHeader 生成 @@ -l,s +l,s @@ 头。
func hunkHeader(aStart, aLen, bStart, bLen int) string {
	return "@@ -" + strconv.Itoa(aStart) + "," + strconv.Itoa(aLen) +
		" +" + strconv.Itoa(bStart) + "," + strconv.Itoa(bLen) + " @@\n"
}
