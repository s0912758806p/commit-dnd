package install

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// 保留原文的 JSON 編輯器，給 settings.json 用。
//
// 解析時每個值都記住自己的原始文字；輸出時沒被動過的值原封不動吐回去，
// 只有被改過（或底下有東西被改過）的物件／陣列才用偵測到的縮排重新排版。
// 所以不相關的 key 順序、數字寫法、字串跳脫與縮排都不會變。

type jnode struct {
	kind  byte     // '{'、'['，或 0 表示純值（字串、數字、true/false/null）
	keys  [][]byte // 物件 key 的原始文字（含引號）
	items []*jnode // 物件的值或陣列元素
	raw   []byte   // 原始文字；容器被改過時不再使用
	dirty bool     // 這個容器本身被改過（新增、刪除、替換成員）
}

func parseJSON(b []byte) (*jnode, error) {
	if !json.Valid(b) {
		return nil, fmt.Errorf("不是合法的 JSON")
	}
	n, _ := parseValue(b, skipWS(b, 0))
	return n, nil
}

func skipWS(b []byte, i int) int {
	for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\n' || b[i] == '\r') {
		i++
	}
	return i
}

// parseValue 解析從 b[i] 開始的值（呼叫前已確認整份是合法 JSON），回傳節點與結束位置。
func parseValue(b []byte, i int) (*jnode, int) {
	start := i
	switch b[i] {
	case '{', '[':
		n := &jnode{kind: b[i]}
		closer := byte('}')
		if n.kind == '[' {
			closer = ']'
		}
		i = skipWS(b, i+1)
		for b[i] != closer {
			if n.kind == '{' {
				end := scanString(b, i)
				n.keys = append(n.keys, b[i:end])
				i = skipWS(b, skipWS(b, end)+1) // 跳過 ':'
			}
			var v *jnode
			v, i = parseValue(b, i)
			n.items = append(n.items, v)
			if i = skipWS(b, i); b[i] == ',' {
				i = skipWS(b, i+1)
			}
		}
		n.raw = b[start : i+1]
		return n, i + 1
	case '"':
		end := scanString(b, i)
		return &jnode{raw: b[start:end]}, end
	default:
		for i < len(b) && !strings.ContainsRune(",}] \t\r\n", rune(b[i])) {
			i++
		}
		return &jnode{raw: b[start:i]}, i
	}
}

// scanString 回傳從 b[i]（開頭引號）開始的字串結束位置（結尾引號之後）。
func scanString(b []byte, i int) int {
	for i++; b[i] != '"'; i++ {
		if b[i] == '\\' {
			i++
		}
	}
	return i + 1
}

// newJSON 把 Go 值轉成節點；容器全部標成 dirty，輸出時會照檔案的縮排排版。
func newJSON(v any) *jnode {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err) // 只用在固定結構與字串，不會失敗
	}
	n, _ := parseJSON(bytes.TrimSpace(buf.Bytes()))
	n.markAll()
	return n
}

func (n *jnode) markAll() {
	if n.kind != 0 {
		n.dirty = true
	}
	for _, c := range n.items {
		c.markAll()
	}
}

func (n *jnode) changed() bool {
	if n.dirty {
		return true
	}
	for _, c := range n.items {
		if c.changed() {
			return true
		}
	}
	return false
}

// str 回傳字串值；不是字串時 ok 為 false。
func (n *jnode) str() (string, bool) {
	if n == nil || n.kind != 0 || len(n.raw) == 0 || n.raw[0] != '"' {
		return "", false
	}
	var s string
	err := json.Unmarshal(n.raw, &s)
	return s, err == nil
}

func keyName(raw []byte) string {
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

func (n *jnode) index(key string) int {
	for i, k := range n.keys {
		if keyName(k) == key {
			return i
		}
	}
	return -1
}

func (n *jnode) get(key string) *jnode {
	if i := n.index(key); i >= 0 {
		return n.items[i]
	}
	return nil
}

// set 替換既有 key 的值（位置不變），沒有就加在最後。
func (n *jnode) set(key string, v *jnode) {
	n.dirty = true
	if i := n.index(key); i >= 0 {
		n.items[i] = v
		return
	}
	k, _ := json.Marshal(key)
	n.keys = append(n.keys, k)
	n.items = append(n.items, v)
}

func (n *jnode) del(key string) {
	if i := n.index(key); i >= 0 {
		n.removeAt(i)
	}
}

func (n *jnode) removeAt(i int) {
	n.dirty = true
	if n.kind == '{' {
		n.keys = append(n.keys[:i:i], n.keys[i+1:]...)
	}
	n.items = append(n.items[:i:i], n.items[i+1:]...)
}

func (n *jnode) push(v *jnode) {
	n.dirty = true
	n.items = append(n.items, v)
}

// render 輸出節點；indent 為空字串時輸出單行。
func (n *jnode) render(indent string) []byte {
	var buf bytes.Buffer
	n.write(&buf, indent, 0)
	return buf.Bytes()
}

func (n *jnode) write(buf *bytes.Buffer, indent string, depth int) {
	if n.kind == 0 || !n.changed() {
		buf.Write(n.raw)
		return
	}
	closer := byte('}')
	if n.kind == '[' {
		closer = ']'
	}
	buf.WriteByte(n.kind)
	for i, c := range n.items {
		if i > 0 {
			buf.WriteByte(',')
		}
		if indent != "" {
			buf.WriteByte('\n')
			buf.WriteString(strings.Repeat(indent, depth+1))
		}
		if n.kind == '{' {
			buf.Write(n.keys[i])
			buf.WriteByte(':')
			if indent != "" {
				buf.WriteByte(' ')
			}
		}
		c.write(buf, indent, depth+1)
	}
	if indent != "" && len(n.items) > 0 {
		buf.WriteByte('\n')
		buf.WriteString(strings.Repeat(indent, depth))
	}
	buf.WriteByte(closer)
}

// detectIndent 從第一個有縮排的行猜出縮排單位；整份只有一行時回傳空字串（維持單行）。
func detectIndent(b []byte) string {
	t := bytes.TrimSpace(b)
	if !bytes.Contains(t, []byte("\n")) {
		if len(t) == 0 {
			return "  "
		}
		return ""
	}
	for _, line := range bytes.Split(t, []byte("\n"))[1:] {
		if w := len(line) - len(bytes.TrimLeft(line, " \t")); w > 0 && w < len(line) {
			return string(line[:w])
		}
	}
	return "  "
}
