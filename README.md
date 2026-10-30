# commit-dnd — Dungeons & Diffs

用 commit 與 AI 工作（Claude Code、Codex）養寵物、闖無盡地下城的 CLI 遊戲。

目前進度：**迭代 1（寵物與 git）**——孵蛋、commit 給經驗與升級、每日補給、飽足／心情／HP 隨時間變化、冒險日誌。

## 安裝

需要 Go 1.25 以上（`golang.org/x/sys` 的最低需求）。

```bash
make install   # 等同 go install ./cmd/dnd，安裝到 $(go env GOPATH)/bin 或 GOBIN
```

`$(go env GOPATH)/bin` 不在 PATH 上時，請自行加入，才能直接打 `dnd`。git hook 寫的是絕對路徑，不受 PATH 影響。

## 開始玩

```bash
dnd hatch            # 選物種（史萊姆、幼龍、貓頭鷹）並取名；也可以用 --species slime --name 小橘
cd 你的-repo
dnd install git      # 在這個 repo 的 post-commit 加入 dnd
git commit -m "feat: ..."
dnd status           # 看看寵物
dnd log              # 冒險日誌
```

- **經驗**：依 Conventional Commits 類型給分（`feat` 30、`fix` 25、`test`／`refactor`／`perf` 20，其餘 10），加上改動行數的加成（上限 20）。
- **每日補給**：每天前 3 筆 commit 經驗加倍。
- **飽足**：每小時下降，commit 會餵食；太餓時 commit 的經驗減半。
- **心情**：每小時下降，`dnd event user pet` 可以摸摸牠。
- amend、cherry-pick、rebase 不會重複計分；數值都在 `content/balance.yaml`。

## 指令

| 指令 | 說明 |
| --- | --- |
| `dnd hatch [--species id] [--name 名字]` | 孵出寵物；沒給參數時互動詢問 |
| `dnd status` | 寵物現在的狀態（不寫檔） |
| `dnd log [-n 20]` | 最近的冒險日誌（`-n 0` 顯示全部） |
| `dnd install git [--repo .]` | 在 repo 的 post-commit 加入 dnd；已有 hook 時只插入標記區塊 |
| `dnd uninstall git [--repo .]` | 移除 dnd 區塊；檔案只剩 shebang 時整個刪除 |
| `dnd event <source> <type> [key=value...]` | 手動寫入一個事件（開發用）。source：`git`、`claude`、`codex`、`user`、`tick` |
| `dnd debug rebuild` | 從 events.jsonl 重播並覆寫 state.json；只在 `DND_DEBUG=1` 時出現 |

`dnd hook git` 是給 post-commit 呼叫的隱藏指令：不輸出、永遠成功，錯誤寫進 `$DND_HOME/errors.log`。

## 資料位置

| 檔案 | 用途 |
| --- | --- |
| `$DND_HOME/events.jsonl` | 唯一的事實來源，每行一個事件，只追加 |
| `$DND_HOME/state.json` | 最新狀態的快取，可以刪掉，用 `dnd debug rebuild` 重建 |
| `$DND_HOME/errors.log` | hook 失敗的紀錄（hook 本身不會讓 commit 失敗） |
| `$DND_HOME/lock` | 寫入用的檔案鎖 |

`DND_HOME` 未設定時為 `~/.dnd`。**開發測試一律用 `DND_HOME=./tmp/dnd`**（`tmp/` 已在 `.gitignore`），避免弄壞正式遊玩的寵物。

## 開發

```bash
make test   # go test ./...
make ci     # gofmt 檢查、go vet、go test -race（commit 前跑）
go test ./internal/view -update                          # 改了顯示格式後更新 golden 檔
go test ./internal/adapter/git -run '^$' -fuzz FuzzParse # 模糊測試 git 輸出解析
```

GitHub Actions（`.github/workflows/ci.yml`）在每個 PR 與 push 到 `main` 時跑同一個 `make ci`。

架構：adapter 把外部輸入轉成 `event.Event` → `engine.Apply`（純函式）算出新狀態 → `store` 在檔案鎖內追加事件並原子寫入 state.json；`view` 只負責把 State 排成文字。

```
cmd/dnd/                 CLI（Cobra），只負責組裝
content/                 嵌入的 YAML：balance（數值）、species（物種與 ASCII 圖）、logs（日誌模板）
internal/adapter/git/    post-commit 時的 git 輸出 → commit 事件（testdata/ 是真實樣本）
internal/engine/         Apply 與 State
internal/event/          Event 型別
internal/home/           DND_HOME 解析
internal/install/        git post-commit hook 的安裝與移除
internal/store/          events.jsonl / state.json / 檔案鎖
internal/view/           status、log 的排版（testdata/golden/ 守住輸出）
docs/DESIGN-NOTES.md     已採用的設計決定與待決事項
```

### 效能紀錄

2026-10-30，Apple Silicon，`~/go/bin/dnd`，各跑 50 次（用 Python 計時，hyperfine 未安裝）：

| 指令 | 目標 | 平均 | p95 |
| --- | --- | --- | --- |
| `dnd status` | < 20 ms | 3.4 ms | 4.3 ms |
| `dnd hook git` | < 100 ms | 18.3 ms | 20.4 ms（其中 `git log` 約 10 ms） |
