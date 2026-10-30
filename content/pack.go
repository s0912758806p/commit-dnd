// Package content 載入嵌入在執行檔裡的 YAML 遊戲內容（content.Pack）。
//
// YAML 以 go:embed 編進執行檔，所以調整數值只要重新編譯。Pack.Hash 是內容的雜湊，
// 引擎把它寫進 State.ContentHash，用來偵測「用不同數值重播」造成的差異。
// 測試可以直接組一個 &Pack{...} 注入，不必經過 YAML。
package content

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"sync"

	"go.yaml.in/yaml/v3"
)

//go:embed balance.yaml
var balanceYAML []byte

type Pack struct {
	Balance Balance
	Hash    string // sha256 前 12 碼
}

type Balance struct {
	Mood Mood `yaml:"mood"`
}

type Mood struct {
	Start             int `yaml:"start"`
	Max               int `yaml:"max"`
	PerPet            int `yaml:"per_pet"`
	DecayEveryMinutes int `yaml:"decay_every_minutes"`
	DecayAmount       int `yaml:"decay_amount"`
}

// Parse 解析 balance.yaml 並驗證數值；未知欄位視為錯誤，避免打錯字默默失效。
func Parse(balance []byte) (*Pack, error) {
	var p Pack
	dec := yaml.NewDecoder(bytes.NewReader(balance))
	dec.KnownFields(true)
	if err := dec.Decode(&p.Balance); err != nil {
		return nil, fmt.Errorf("balance.yaml: %w", err)
	}
	if err := p.Balance.validate(); err != nil {
		return nil, fmt.Errorf("balance.yaml: %w", err)
	}
	sum := sha256.Sum256(balance)
	p.Hash = hex.EncodeToString(sum[:])[:12]
	return &p, nil
}

func (b Balance) validate() error {
	m := b.Mood
	switch {
	case m.Max <= 0:
		return fmt.Errorf("mood.max 必須 > 0")
	case m.Start < 0 || m.Start > m.Max:
		return fmt.Errorf("mood.start 必須在 0..mood.max 之間")
	case m.DecayEveryMinutes <= 0:
		return fmt.Errorf("mood.decay_every_minutes 必須 > 0")
	case m.DecayAmount < 0 || m.PerPet < 0:
		return fmt.Errorf("mood 的增減量不可為負")
	}
	return nil
}

var defaultPack = sync.OnceValues(func() (*Pack, error) { return Parse(balanceYAML) })

// Default 回傳嵌入的正式內容；只解析一次。
func Default() (*Pack, error) { return defaultPack() }
