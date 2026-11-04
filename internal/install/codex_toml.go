package install

// 保留原文的 config.toml 編輯：只找最上層（第一個 [table] 之前）每個 key = value 的位置，
// 改 notify 時只替換那一段 bytes，其他內容（註解、空行、順序、引號風格）原樣保留。
//
// 掃描器只需要處理合法的 TOML：呼叫端先用 BurntSushi/toml 完整解析過，改完後再解析一次，
// 確認除了 notify 以外的資料完全相同（verifyCodexEdit），掃描器有誤時寧可報錯也不寫檔。

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// tomlEntry 是最上層的一個 key = value。
type tomlEntry struct {
	key              string // 原文的 key，去掉前後空白
	start            int    // key 所在行的行首
	valStart, valEnd int    // value 的範圍（不含後面的空白與註解）
	end              int    // value 那一行結束（含換行）的位置
}

// scanTopLevel 找出最上層的 key = value；tableAt 是第一個 table 標頭的行首（沒有時為 len(src)）。
func scanTopLevel(src []byte) (entries []tomlEntry, tableAt int, err error) {
	i := bomLen(src)
	for i < len(src) {
		lineStart := i
		for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
			i++
		}
		if i >= len(src) {
			break
		}
		switch src[i] {
		case '\n', '\r':
			i++
			continue
		case '#':
			i = lineEnd(src, i)
			continue
		case '[':
			return entries, lineStart, nil
		}
		keyStart := i
		for i < len(src) && src[i] != '=' {
			switch src[i] {
			case '"', '\'':
				if i, err = skipString(src, i); err != nil {
					return nil, 0, err
				}
				continue
			case '\n':
				return nil, 0, fmt.Errorf("第 %d 行沒有 =", bytes.Count(src[:i], []byte("\n"))+1)
			}
			i++
		}
		if i >= len(src) {
			return nil, 0, errors.New("檔案結尾的 key 沒有值")
		}
		e := tomlEntry{key: strings.TrimSpace(string(src[keyStart:i])), start: lineStart}
		i++
		for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
			i++
		}
		e.valStart = i
		if e.valEnd, e.end, err = scanValue(src, i); err != nil {
			return nil, 0, err
		}
		entries = append(entries, e)
		i = e.end
	}
	return entries, len(src), nil
}

// scanValue 從 value 開頭掃到它那一行結束；陣列可以跨行，裡面可以有註解。
func scanValue(src []byte, i int) (valEnd, end int, err error) {
	depth := 0
	valEnd = i
	for i < len(src) {
		switch c := src[i]; c {
		case '"', '\'':
			if i, err = skipString(src, i); err != nil {
				return 0, 0, err
			}
			valEnd = i
			continue
		case '#':
			i = lineEnd(src, i)
			if depth == 0 {
				return valEnd, i, nil
			}
			continue
		case '\n':
			if depth == 0 {
				return valEnd, i + 1, nil
			}
		case ' ', '\t', '\r':
		case '[', '{':
			depth++
			valEnd = i + 1
		case ']', '}':
			depth--
			valEnd = i + 1
		default:
			valEnd = i + 1
		}
		i++
	}
	return valEnd, len(src), nil
}

// skipString 跳過從 src[i] 開始的字串（基本、literal、多行），回傳結尾引號之後的位置。
func skipString(src []byte, i int) (int, error) {
	q := src[i]
	if bytes.HasPrefix(src[i:], []byte{q, q, q}) {
		j := i + 3
		for j < len(src) {
			if q == '"' && src[j] == '\\' {
				j += 2
				continue
			}
			if bytes.HasPrefix(src[j:], []byte{q, q, q}) {
				j += 3
				for k := 0; k < 2 && j < len(src) && src[j] == q; k++ { // 內容結尾可以有一兩個引號
					j++
				}
				return j, nil
			}
			j++
		}
		return 0, errors.New("多行字串沒有結尾")
	}
	for j := i + 1; j < len(src); j++ {
		switch {
		case src[j] == '\n':
			return 0, errors.New("字串沒有結尾")
		case q == '"' && src[j] == '\\':
			j++
		case src[j] == q:
			return j + 1, nil
		}
	}
	return 0, errors.New("字串沒有結尾")
}

// bomLen 回傳檔案開頭 BOM 的長度（BurntSushi/toml 會略過 UTF-8 與 UTF-16 的 BOM）。
func bomLen(src []byte) int {
	for _, bom := range []string{"\xef\xbb\xbf", "\xfe\xff", "\xff\xfe"} {
		if bytes.HasPrefix(src, []byte(bom)) {
			return len(bom)
		}
	}
	return 0
}

// lineEnd 回傳 i 所在行的換行之後的位置。
func lineEnd(src []byte, i int) int {
	if n := bytes.IndexByte(src[i:], '\n'); n >= 0 {
		return i + n + 1
	}
	return len(src)
}

// isNotifyKey 回報 key 是否就是最上層的 notify（bare 或加引號）。
func isNotifyKey(key string) bool {
	return key == "notify" || key == `"notify"` || key == `'notify'`
}

// tomlArray 把字串陣列排成單行的 TOML 陣列（基本字串）。
func tomlArray(argv []string) string {
	parts := make([]string, len(argv))
	for i, s := range argv {
		parts[i] = tomlString(s)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// decodeTOML 完整解析 config.toml。
func decodeTOML(src []byte) (map[string]any, error) {
	data := map[string]any{}
	if _, err := toml.Decode(string(src), &data); err != nil {
		return nil, err
	}
	return data, nil
}

// notifyArgv 取出 data 的 notify；不是字串陣列時回傳錯誤。
func notifyArgv(data map[string]any) (argv []string, ok bool, err error) {
	v, ok := data["notify"]
	if !ok {
		return nil, false, nil
	}
	arr, isArr := v.([]any)
	if !isArr {
		return nil, true, errors.New("notify 不是陣列")
	}
	argv = make([]string, len(arr))
	for i, a := range arr {
		s, isStr := a.(string)
		if !isStr {
			return nil, true, errors.New("notify 不是字串陣列")
		}
		argv[i] = s
	}
	return argv, true, nil
}

// verifyCodexEdit 重新解析改好的內容：notify 必須是 want（nil 代表沒有 notify），
// 其他資料必須跟原本完全一樣。
func verifyCodexEdit(newSrc []byte, old map[string]any, want []string) error {
	data, err := decodeTOML(newSrc)
	if err != nil {
		return err
	}
	got, has, err := notifyArgv(data)
	if err != nil {
		return err
	}
	if has != (want != nil) || want != nil && !slices.Equal(got, want) {
		return fmt.Errorf("notify 是 %q，應為 %q", got, want)
	}
	a, b := maps.Clone(old), maps.Clone(data)
	delete(a, "notify")
	delete(b, "notify")
	if !reflect.DeepEqual(a, b) {
		return errors.New("notify 以外的設定被改到了")
	}
	return nil
}
