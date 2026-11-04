# 使用與開發說明

這個專案是什麼，見 [README](../README.md)。

目前進度：**迭代 4（收服、合體技、圖鑑與隊伍）**——孵蛋、commit 給經驗與升級、每日補給、飽足／心情／HP 隨時間變化、冒險日誌；Claude Code 的 AI 回合帶寵物闖地下城，commit 救援與鑑定裝備；`fix:` commit 收服怪物組隊，AI 工作中的 commit 發動合體技。

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
- Codex 的通知裡沒有個別的工具呼叫，所以 **Codex 的每個回合＝寵物先攻擊一次，再結束回合**（遇怪或被打）。
- 每層打倒 6 隻往下一層；每 10 層有 Boss，打倒後可以用 `dnd warp <樓層>` 傳送回來。
- 打倒怪物會掉「未鑑定」的裝備；每次 commit 鑑定一件，比身上的強才換上。
- HP 歸零就倒地：未鑑定的裝備全部遺失，AI 也不能再帶牠前進或回血，**下一次 commit 會把牠救起來**，在同一層繼續。
- 閒置 8 小時（下班、睡覺）會紮營，HP 回滿。
- 屬性：火剋草、草剋水、水剋火。怪物、樓層、裝備定義在 `content/monsters.yaml`、`floors.yaml`、`items.yaml`。

**收服、合體技、圖鑑與隊伍**：
- **收服**：`fix:` 的 commit 會試著收服眼前的一般怪物（Boss 不行）。機率是 40%，怪物越虛弱越高（每損失 10% HP 加 4%，最高近 80%）。成功時怪物離開戰鬥、算這層打倒一隻，但不掉裝備；倒地中的那筆 commit 只會救援，不會收服。
- **隊伍**：最多 3 隻、同種只能一隻。收服時隊伍有空位就自動加入；每位隊員讓寵物攻擊增加（該怪物在最深收服樓層的攻擊力 × 25%，至少 +1），所以往深處收服的同種怪物會讓隊員變強。用 `dnd party add|remove <怪物 id>` 調整，id 見 `dnd dex`。
- **合體技**：commit 前 30 分鐘內有 AI 活動（Claude 的回合或工具呼叫）時，這筆 commit 會對眼前的怪物（含 Boss）造成寵物攻擊 ×200% 的傷害，每位隊員再 +50%，打倒照常掉落。同一筆 `fix:` 收服成功就不再發動。
- **圖鑑**：`dnd dex` 列出所有怪物：收服過的、遇過還沒收服的、沒遇過的（顯示 `???`）。早期樓層的怪物很快就遇不到了，想補齊圖鑑可以用 `dnd warp 0` 回入口重新走一趟。
- 數值在 `content/balance.yaml` 的 `party` 區塊，日誌在 `content/logs.yaml` 的 `party` 區塊。

**接上 Claude Code**：執行 `dnd install claude`，確認差異後寫入。Claude Code 會自動重新載入設定，不需要重啟；底部 statusline 會顯示寵物與今日 AI 回合。設定檔位置依序為 `--settings`、`$CLAUDE_CONFIG_DIR/settings.json`、`~/.claude/settings.json`。

**接上 Codex**：執行 `dnd install codex`，確認差異後寫入。只會改 `config.toml` 最上層的 `notify` 一行（設成 `["<dnd 路徑>", "hook", "codex"]`），其他設定、註解與順序都不動；已經開著的 Codex 要重新啟動才會讀到。設定檔位置依序為 `--config`、`$CODEX_HOME/config.toml`、`~/.codex/config.toml`。原本就有自己的 `notify`（例如桌面通知）時，它會存到 `$DND_HOME/codex/original-notify.json`，`dnd hook codex` 寫完寵物的事件後，再用同樣的參數執行它；兩邊的失敗互不影響。`dnd uninstall codex` 會還原原本那一行。

## 指令

| 指令 | 說明 |
| --- | --- |
| `dnd hatch [--species id] [--name 名字]` | 孵出寵物；沒給參數時互動詢問 |
| `dnd status` | 寵物現在的狀態（不寫檔） |
| `dnd log [-n 20]` | 最近的冒險日誌（`-n 0` 顯示全部） |
| `dnd dungeon` | 地下城：樓層與這層進度、眼前的怪物、倒地狀態、攻防、裝備、隊伍摘要、未鑑定背包、戰績（不寫檔） |
| `dnd warp <樓層>` | 傳送到入口（`0`）或已解鎖的 Boss 層（打倒過 Boss 的 B10、B20…）；倒地中不能傳送 |
| `dnd dex` | 圖鑑：每種怪物收服幾次、最深收服樓層、是否在隊伍中；遇過還沒收服的顯示遇過次數，沒遇過的顯示 `???`（不寫檔） |
| `dnd party` | 隊伍：隊員與攻擊加成、可換上的候補、合體技現在是否就緒（不寫檔） |
| `dnd party add <怪物 id>` | 讓收服過的怪物加入隊伍（最多 3 隻、同種一隻）；不合法時印出原因、狀態不變 |
| `dnd party remove <怪物 id>` | 讓隊員離開隊伍 |
| `dnd statusline` | Claude Code 底部的一行（等級、XP、HP、飽足、補給、樓層／倒地、今日 AI 回合）；不寫檔、永遠成功 |
| `dnd doctor [--repo .] [--settings 路徑] [--codex-config 路徑]` | 檢查資料、寵物、hook 錯誤紀錄、git hook、Claude Code 與 Codex 設定；只讀，有問題（✗）時 exit 1。沒用 Codex 時只顯示提醒（!） |
| `dnd install git [--repo .]` | 在 repo 的 post-commit 加入 dnd；已有 hook 時只插入標記區塊 |
| `dnd uninstall git [--repo .]` | 移除 dnd 區塊；檔案只剩 shebang 時整個刪除 |
| `dnd install claude [--settings 路徑] [--yes]` | 在 Claude Code 的 settings.json 加入 PostToolUse／Stop hook（背景執行）與 statusLine；先印差異、確認、備份成 `settings.json.dnd-backup-時間`，再寫入 |
| `dnd uninstall claude [--settings 路徑] [--yes]` | 移除 dnd 的 hook 與 statusLine，還原安裝前的 statusLine；同樣先印差異、確認、備份 |
| `dnd install codex [--config 路徑] [--yes]` | 把 Codex `config.toml` 的 `notify` 設成 dnd（原本有別的 notify 時改成串接）；先印差異、確認、備份成 `config.toml.dnd-backup-時間`，再寫入 |
| `dnd uninstall codex [--config 路徑] [--yes]` | notify 是 dnd 時，還原安裝前的 notify（沒有就刪掉那一行）；同樣先印差異、確認、備份 |
| `dnd event <source> <type> [key=value...] [--count N]` | 手動寫入事件（開發用）。source：`git`、`claude`、`codex`、`user`、`tick`；寫法見下方「除錯指令」 |
| `dnd debug …` | 除錯指令，只在 `DND_DEBUG=1` 時出現，見下方「除錯指令」 |

`dnd hook git`（post-commit）、`dnd hook claude`（Claude Code 的 PostToolUse／Stop）與 `dnd hook codex <payload>`（Codex 的 notify，JSON 在最後一個參數）是隱藏指令：不輸出、永遠成功，錯誤寫進 `$DND_HOME/errors.log`。

**Claude Code 的 AI 活動**：每個 AI 回合（Stop）與工具呼叫（PostToolUse）都會被記錄。迭代 2 只計數、不影響寵物；AI 不能幫寵物回血或給經驗。從迭代 3 起，AI 回合會推進地下城。

**statusline 串接**：安裝前若已有自己的 statusLine，原本的設定會存到 `$DND_HOME/claude/original-statusline.json`。`dnd statusline` 會先用同一份輸入執行它，再把寵物資訊接在同一行後面；`dnd uninstall claude` 時還原。

## 資料位置

| 檔案 | 用途 |
| --- | --- |
| `$DND_HOME/events.jsonl` | 唯一的事實來源，每行一個事件，只追加 |
| `$DND_HOME/state.json` | 最新狀態的快取，可以刪掉，用 `dnd debug rebuild` 重建 |
| `$DND_HOME/errors.log` | hook 失敗的紀錄（hook 本身不會讓 commit 或 Claude Code 失敗） |
| `$DND_HOME/claude/original-statusline.json` | 安裝前原本的 Claude Code statusLine（有才會存在） |
| `$DND_HOME/codex/original-notify.json` | 安裝前原本的 Codex notify（陣列與原文；有才會存在） |
| `$DND_HOME/capture/claude/` | 手動建立這個目錄時，`dnd hook claude` 會把收到的原始 JSON 存在這裡（收集測試樣本用） |
| `$DND_HOME/capture/codex/` | 同上，`dnd hook codex` 依 payload 的 `type` 存檔（內含對話內容，放進 testdata 前要去識別化） |
| `$DND_HOME/lock` | 寫入用的檔案鎖 |
| `$DND_HOME/debug-time-offset` | `dnd debug time` 的時間偏移；只在 `DND_DEBUG=1` 時生效，殘留時 `dnd doctor` 會提醒 |
| `$DND_HOME/snapshots/` | `dnd debug snapshot` 存的快照 |

`DND_HOME` 未設定時為 `~/.dnd`。**開發測試一律用 `DND_HOME=./tmp/dnd`**（`tmp/` 已在 `.gitignore`），避免弄壞正式遊玩的寵物。

## 除錯指令

設定 `DND_DEBUG=1` 才會出現 `dnd debug`，正式玩法裡沒有。**一律搭配 `DND_HOME=./tmp/dnd`**，不要拿正式的寵物測試：

```bash
export DND_HOME=./tmp/dnd DND_DEBUG=1
dnd hatch --species elf --name 測試
```

| 指令 | 說明 |
| --- | --- |
| `dnd debug time [+8h\|-1h\|reset]` | 快轉或倒轉「現在」（累加；不帶參數顯示目前偏移）。測飽足衰減、紮營、HP 自動回滿。偏移存在 `DND_HOME`，只在 `DND_DEBUG=1` 時生效 |
| `dnd debug xp <N>` | 直接給 N 點 XP，照正常流程升級 |
| `dnd debug floor <N>` | 跳到第 N 層（`0` 是入口）：清掉眼前的怪物、這層進度歸零，更新最深樓層。測怪物強度與推不動的感覺 |
| `dnd debug event <source> <type> [key=value...] [--count N]` | 和 `dnd event` 同一個指令，不開 Claude Code 也能模擬 AI 回合，例如 `debug event claude turn --count 10`、`debug event claude tool_use tool=Edit`、`debug event git commit kind=fix added=10`（commit 會自動帶不重複的 hash，才會計分） |
| `dnd debug snapshot save <名稱> [--force]` | 把 events.jsonl、state.json、時間偏移、errors.log 存到 `$DND_HOME/snapshots/<名稱>/` |
| `dnd debug snapshot load <名稱>` | 載回快照；先把目前的資料自動另存成 `before-load-時間` |
| `dnd debug snapshot list` | 列出快照 |
| `dnd debug stats` | 重播 events.jsonl，依當地日期列出每天的 XP、commit、AI 回合、倒地、最深樓層、下探層數，最後一行總計與平均每天下探層數 |
| `dnd debug rebuild` | 從 events.jsonl 重播並覆寫 state.json |

`debug xp`、`debug floor` 寫入的是 `debug_xp`／`debug_floor` 事件，會留在 events.jsonl（日誌開頭標「【除錯】」），重播結果一致。

## 開發

```bash
make test   # go test ./...
make ci     # gofmt 檢查、go vet、go test -race（commit 前跑）
go test ./internal/view -update                          # 改了顯示格式後更新 golden 檔
go test ./cmd/dnd -run DebugStats -update                # 改了 dnd debug stats 的格式後更新 golden 檔
go test ./internal/adapter/git -run '^$' -fuzz FuzzParse # 模糊測試 git 輸出解析
go test ./internal/adapter/codex -run '^$' -fuzz FuzzParse        # Codex notify payload
go test ./internal/install -run '^$' -fuzz FuzzCodexInstall       # config.toml 的 notify 編輯
```

GitHub Actions（`.github/workflows/ci.yml`）在每個 PR 與 push 到 `main` 時跑同一個 `make ci`。

架構：adapter 把外部輸入轉成 `event.Event` → `engine.Apply`（純函式）算出新狀態 → `store` 在檔案鎖內追加事件並原子寫入 state.json；`view` 只負責把 State 排成文字。

```
cmd/dnd/                 CLI（Cobra），只負責組裝
content/                 嵌入的 YAML：balance（數值）、species（物種）、logs（日誌模板）、monsters／floors／items（地下城）
internal/adapter/claude/ Claude Code hook 的 stdin JSON → turn／tool_use 事件（testdata/ 是去識別化樣本）
internal/adapter/codex/  Codex notify 的 payload → tool_use（Codex）＋turn 事件（testdata/ 依原始碼格式手寫）
internal/adapter/git/    post-commit 時的 git 輸出 → commit 事件（testdata/ 是真實樣本）
internal/clock/          CLI 的「現在」：真實時間 ＋ dnd debug time 的偏移（只在 DND_DEBUG=1 時）
internal/engine/         Apply 與 State；dungeon.go 地下城、capture.go 收服／合體技／圖鑑／隊伍；debug.go 是 debug_xp／debug_floor
internal/event/          Event 型別
internal/home/           DND_HOME 解析
internal/install/        git post-commit hook、Claude Code settings.json、Codex config.toml 的安裝、移除與 doctor 檢查
internal/store/          events.jsonl / state.json / 檔案鎖
internal/view/           status、statusline、dungeon、dex、party、log 的排版（testdata/golden/ 守住輸出）
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
