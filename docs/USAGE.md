# 使用與開發說明

這個專案是什麼，見 [README](../README.md)。

目前進度：**迭代 3（無盡地下城）**——孵蛋、commit 給經驗與升級、每日補給、飽足／心情／HP 隨時間變化、冒險日誌；Claude Code 的 AI 回合帶寵物闖地下城，commit 救援與鑑定裝備。

## 安裝

需要 Go 1.25 以上（`golang.org/x/sys` 的最低需求）。

```bash
make install   # 等同 go install ./cmd/dnd，安裝到 $(go env GOPATH)/bin 或 GOBIN
```

`$(go env GOPATH)/bin` 不在 PATH 上時，請自行加入，才能直接打 `dnd`。git hook 寫的是絕對路徑，不受 PATH 影響。

## 開始玩

```bash
dnd hatch            # 選種族並取名；也可以用 --species human --name 小橘
cd 你的-repo
dnd install git      # 在這個 repo 的 post-commit 加入 dnd
git commit -m "feat: ..."
dnd status           # 看看寵物
dnd log              # 冒險日誌
```

- **種族**：人族、精靈族、獸人族、鼠人族、矮人族、不死族、惡魔族、龍人族（`content/species.yaml`）。
- **經驗**：依 Conventional Commits 類型給分（`feat` 30、`fix` 25、`test`／`refactor`／`perf` 20，其餘 10），加上改動行數的加成（上限 20）。
- **每日補給**：每天前 3 筆 commit 經驗加倍。
- **飽足**：每小時下降，commit 會餵食；太餓時 commit 的經驗減半。
- **心情**：每小時下降，`dnd event user pet` 可以摸摸牠。
- amend、cherry-pick、rebase 不會重複計分；數值都在 `content/balance.yaml`。

**無盡地下城**（接上 Claude Code 之後）：
- Claude 的每個回合，寵物會遇到下一隻怪物或被眼前的怪物攻擊；Claude 改檔案（Edit、Write）時，寵物會出手攻擊。
- 每層打倒 6 隻往下一層；每 10 層有 Boss，打倒後可以用 `dnd warp <樓層>` 傳送回來。
- 打倒怪物會掉「未鑑定」的裝備；每次 commit 鑑定一件，比身上的強才換上。
- HP 歸零就倒地：未鑑定的裝備全部遺失，AI 也不能再帶牠前進或回血，**下一次 commit 會把牠救起來**，在同一層繼續。
- 閒置 8 小時（下班、睡覺）會紮營，HP 回滿。
- 屬性：火剋草、草剋水、水剋火。怪物、樓層、裝備定義在 `content/monsters.yaml`、`floors.yaml`、`items.yaml`。

**接上 Claude Code**：執行 `dnd install claude`，確認差異後寫入。Claude Code 會自動重新載入設定，不需要重啟；底部 statusline 會顯示寵物與今日 AI 回合。設定檔位置依序為 `--settings`、`$CLAUDE_CONFIG_DIR/settings.json`、`~/.claude/settings.json`。

## 指令

| 指令 | 說明 |
| --- | --- |
| `dnd hatch [--species id] [--name 名字]` | 孵出寵物；沒給參數時互動詢問 |
| `dnd status` | 寵物現在的狀態（不寫檔） |
| `dnd log [-n 20]` | 最近的冒險日誌（`-n 0` 顯示全部） |
| `dnd statusline` | Claude Code 底部的一行（等級、XP、HP、飽足、補給、今日 AI 回合）；不寫檔、永遠成功 |
| `dnd doctor [--repo .] [--settings 路徑]` | 檢查資料、寵物、hook 錯誤紀錄、git hook 與 Claude Code 設定；只讀，有問題時 exit 1 |
| `dnd install git [--repo .]` | 在 repo 的 post-commit 加入 dnd；已有 hook 時只插入標記區塊 |
| `dnd uninstall git [--repo .]` | 移除 dnd 區塊；檔案只剩 shebang 時整個刪除 |
| `dnd install claude [--settings 路徑] [--yes]` | 在 Claude Code 的 settings.json 加入 PostToolUse／Stop hook（背景執行）與 statusLine；先印差異、確認、備份成 `settings.json.dnd-backup-時間`，再寫入 |
| `dnd uninstall claude [--settings 路徑] [--yes]` | 移除 dnd 的 hook 與 statusLine，還原安裝前的 statusLine；同樣先印差異、確認、備份 |
| `dnd event <source> <type> [key=value...]` | 手動寫入一個事件（開發用）。source：`git`、`claude`、`codex`、`user`、`tick` |
| `dnd debug rebuild` | 從 events.jsonl 重播並覆寫 state.json；只在 `DND_DEBUG=1` 時出現 |

`dnd hook git`（post-commit）與 `dnd hook claude`（Claude Code 的 PostToolUse／Stop）是隱藏指令：不輸出、永遠成功，錯誤寫進 `$DND_HOME/errors.log`。

**Claude Code 的 AI 活動**：每個 AI 回合（Stop）與工具呼叫（PostToolUse）都會被記錄。迭代 2 只計數、不影響寵物；AI 不能幫寵物回血或給經驗。從迭代 3 起，AI 回合會推進地下城。

**statusline 串接**：安裝前若已有自己的 statusLine，原本的設定會存到 `$DND_HOME/claude/original-statusline.json`。`dnd statusline` 會先用同一份輸入執行它，再把寵物資訊接在同一行後面；`dnd uninstall claude` 時還原。

## 資料位置

| 檔案 | 用途 |
| --- | --- |
| `$DND_HOME/events.jsonl` | 唯一的事實來源，每行一個事件，只追加 |
| `$DND_HOME/state.json` | 最新狀態的快取，可以刪掉，用 `dnd debug rebuild` 重建 |
| `$DND_HOME/errors.log` | hook 失敗的紀錄（hook 本身不會讓 commit 或 Claude Code 失敗） |
| `$DND_HOME/claude/original-statusline.json` | 安裝前原本的 Claude Code statusLine（有才會存在） |
| `$DND_HOME/capture/claude/` | 手動建立這個目錄時，`dnd hook claude` 會把收到的原始 JSON 存在這裡（收集測試樣本用） |
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
content/                 嵌入的 YAML：balance（數值）、species（物種）、logs（日誌模板）
internal/adapter/claude/ Claude Code hook 的 stdin JSON → turn／tool_use 事件（testdata/ 是去識別化樣本）
internal/adapter/git/    post-commit 時的 git 輸出 → commit 事件（testdata/ 是真實樣本）
internal/engine/         Apply 與 State
internal/event/          Event 型別
internal/home/           DND_HOME 解析
internal/install/        git post-commit hook 的安裝與移除
internal/store/          events.jsonl / state.json / 檔案鎖
internal/view/           status、log 的排版（testdata/golden/ 守住輸出）
docs/DESIGN-NOTES.md     已採用的設計決定與待決事項
docs/hero.svg            README 的主視覺
```

### 效能紀錄

2026-10-30，Apple Silicon，`~/go/bin/dnd`，各跑 50 次（用 Python 計時，hyperfine 未安裝）：

| 指令 | 目標 | 平均 | p95 |
| --- | --- | --- | --- |
| `dnd status` | < 20 ms | 3.4 ms | 4.3 ms |
| `dnd hook git` | < 100 ms | 18.3 ms | 20.4 ms（其中 `git log` 約 10 ms） |

2026-11-04，同一台機器，未安裝 hyperfine，用 Python 各跑 50 次（量測時另一個 session 正在平行編譯與測試）：

| 指令 | 目標 | 平均 | p95 |
| --- | --- | --- | --- |
| `dnd hook claude`（PostToolUse） | < 100 ms | 23.7 ms | 49.5 ms |
| `dnd hook claude`（Stop） | < 100 ms | 21.6 ms | 40.3 ms |
| `dnd statusline`（未串接） | — | 11.1 ms | 20.8 ms |
