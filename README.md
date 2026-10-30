# commit-dnd — Dungeons & Diffs

用 commit 與 AI 工作（Claude Code、Codex）養寵物、闖無盡地下城的 CLI 遊戲。

目前進度：**迭代 0（專案骨架）**——只有事件寫入、狀態重建與 `pet` 事件，還不能真正遊玩。

## 安裝

需要 Go 1.25 以上（`golang.org/x/sys` 的最低需求）。

```bash
make install   # 等同 go install ./cmd/dnd
```

## 資料位置

| 檔案 | 用途 |
| --- | --- |
| `$DND_HOME/events.jsonl` | 唯一的事實來源，每行一個事件，只追加 |
| `$DND_HOME/state.json` | 最新狀態的快取，可以刪掉，用 `dnd debug rebuild` 重建 |
| `$DND_HOME/lock` | 寫入用的檔案鎖 |

`DND_HOME` 未設定時為 `~/.dnd`。**開發測試一律用 `DND_HOME=./tmp/dnd`**（`tmp/` 已在 `.gitignore`），避免弄壞正式遊玩的寵物。

## 指令

| 指令 | 說明 |
| --- | --- |
| `dnd event <source> <type> [key=value...]` | 手動寫入一個事件（開發用）。source：`git`、`claude`、`codex`、`user`、`tick` |
| `dnd debug rebuild` | 從 events.jsonl 重播並覆寫 state.json；只在 `DND_DEBUG=1` 時出現 |

## 開發

```bash
make test   # go test ./...
make ci     # gofmt 檢查、go vet、go test -race（commit 前跑）
```

GitHub Actions（`.github/workflows/ci.yml`）在每個 PR 與 push 到 `main` 時跑同一個 `make ci`。

架構：adapter 把外部輸入轉成 `event.Event` → `engine.Apply`（純函式）算出新狀態 → `store` 在檔案鎖內追加事件並原子寫入 state.json。

```
cmd/dnd/           CLI（Cobra），只負責組裝
content/           嵌入的 YAML 遊戲數值（balance.yaml）與 content.Pack
internal/event/    Event 型別
internal/engine/   Apply 與 State
internal/store/    events.jsonl / state.json / 檔案鎖
internal/home/     DND_HOME 解析
docs/DESIGN-NOTES.md  已採用的設計決定與待決事項
```

### 效能紀錄

（迭代 1 起以 `hyperfine` 量測 `dnd status` 與 hook，記錄於此。）
