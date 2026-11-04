package install

import (
	"fmt"
	"strings"
)

const diffContext = 3

type diffOp struct {
	kind   byte // ' '、'-'、'+'
	line   string
	ai, bi int // 這一步之前已經走過的舊／新行數
}

// unifiedDiff 產生以行為單位的 unified diff（上下文 3 行），給使用者在寫入前確認。
func unifiedDiff(name string, a, b []byte) string {
	ops := diffLines(splitLines(a), splitLines(b))
	var out strings.Builder
	for i := 0; i < len(ops); {
		if ops[i].kind == ' ' {
			i++
			continue
		}
		// 從這個變更往前帶上下文，往後吃到「連續 2×context 行沒變」為止。
		start, end := max(0, i-diffContext), i
		for same := 0; end < len(ops) && same <= 2*diffContext; end++ {
			if ops[end].kind == ' ' {
				same++
			} else {
				same = 0
			}
		}
		for end > i && ops[end-1].kind == ' ' {
			end--
		}
		end = min(len(ops), end+diffContext)

		if out.Len() == 0 {
			fmt.Fprintf(&out, "--- %s\n+++ %s\n", name, name)
		}
		var na, nb int
		for _, op := range ops[start:end] {
			if op.kind != '+' {
				na++
			}
			if op.kind != '-' {
				nb++
			}
		}
		fmt.Fprintf(&out, "@@ -%s +%s @@\n", hunkRange(ops[start].ai, na), hunkRange(ops[start].bi, nb))
		for _, op := range ops[start:end] {
			out.WriteByte(op.kind)
			out.WriteString(op.line)
			out.WriteByte('\n')
		}
		i = end
	}
	return out.String()
}

func hunkRange(before, n int) string {
	if n == 0 {
		return fmt.Sprintf("%d,0", before)
	}
	return fmt.Sprintf("%d,%d", before+1, n)
}

func splitLines(b []byte) []string {
	s := strings.TrimSuffix(string(b), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// diffLines 先去掉頭尾相同的行，中間用 LCS 對齊；settings.json 的改動集中，中間段很小。
func diffLines(a, b []string) []diffOp {
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	x, y := a[p:len(a)-s], b[p:len(b)-s]

	// lcs[i][j] = x[i:] 與 y[j:] 的最長共同子序列長度
	lcs := make([][]int32, len(x)+1)
	for i := range lcs {
		lcs[i] = make([]int32, len(y)+1)
	}
	for i := len(x) - 1; i >= 0; i-- {
		for j := len(y) - 1; j >= 0; j-- {
			if x[i] == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}

	var ops []diffOp
	ai, bi := 0, 0
	emit := func(kind byte, line string) {
		ops = append(ops, diffOp{kind, line, ai, bi})
		if kind != '+' {
			ai++
		}
		if kind != '-' {
			bi++
		}
	}
	for _, l := range a[:p] {
		emit(' ', l)
	}
	i, j := 0, 0
	for i < len(x) || j < len(y) {
		switch {
		case i < len(x) && j < len(y) && x[i] == y[j]:
			emit(' ', x[i])
			i, j = i+1, j+1
		case i < len(x) && (j == len(y) || lcs[i+1][j] >= lcs[i][j+1]):
			emit('-', x[i])
			i++
		default:
			emit('+', y[j])
			j++
		}
	}
	for _, l := range a[len(a)-s:] {
		emit(' ', l)
	}
	return ops
}
