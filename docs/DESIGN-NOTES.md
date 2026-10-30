# 設計筆記

DEVPLAN（2026-10-30）之外、實作時定下的決定。每條註明在哪一輪採用；推翻時在該條頂端加註，不改寫原文。

## 已採用（迭代 0）

1. **鎖涵蓋整個更新**：DEVPLAN 的 `Store.Commit` 只在寫入時取鎖，`Load → Apply → Commit` 之間會被並行的 hook 互相蓋掉。
   改為 `store.Update(e)`：取鎖 → 讀 state → 指派 Seq → `engine.Apply` → 追加 events.jsonl → 原子寫 state.json → 放鎖。
   取鎖逾時 `store.DefaultLockTimeout`（50 ms）回傳 `ErrBusy`，由呼叫端決定是否靜默放棄。
2. **Event 新增欄位**：`V`（事件格式版本）、`Seq`（store 在鎖內指派，連續遞增）、`Args map[string]string`（hatch 物種、warp 樓層、debug 數值等參數）。
3. **亂數種子**（迭代 3 實作）：`hash(最後一次 commit hash, 樓層, Seq)`，讓沒有 commit hash 的 AI 回合也有確定的種子。
4. **tick 不落地，衰減必須路徑無關**：status／statusline 只在記憶體套用 tick。衰減以「累積未滿一週期的時間」（`Pet.MoodCarry`）實作，
   一次推進 Δ1+Δ2 與分兩次結果完全相同；`engine.TestDecayIsPathIndependent` 守住這條。之後新增的任何時間效果都要加進這個測試。
5. **時間倒流**：`e.At < UpdatedAt` 時 Δ 視為 0，`UpdatedAt` 只前進。`debug time` 的偏移（迭代 1）存在 `DND_HOME`，由 adapter 的 `now` 統一加上。
6. **數值一律整數**：心情等以千分位表示；XP 曲線 `18 × N^1.3` 在 content 載入時預先算成整數表（迭代 1）。
7. **content 版本**：`State.ContentHash` 記錄套用時的 balance 雜湊。Rebuild 的保證是「同一份 content 下相同」；調數值後重播得到不同結果是預期行為（`dnd sim` 的用途）。
8. **state.json 帶 `events_bytes`**：與 events.jsonl 大小不符代表上次寫到一半，下次 Update 先重播補上；結尾的半行會被截掉，中間行損毀則回報錯誤。
9. **YAML 套件**：`go.yaml.in/yaml/v3`（`gopkg.in/yaml.v3` 已封存）；`content/` 放在 repo 根目錄以符合 DEVPLAN 的 `content/*.yaml` 路徑。
10. **Go 版本**：依賴（`golang.org/x/sys`）要求 Go 1.25，因此最低版本由 DEVPLAN 的 1.24 提高為 1.25。

## 待決

- **post-commit 重複觸發**（迭代 1）：`rebase`、`cherry-pick`、`commit --amend` 都會跑 post-commit。候選做法：rebase 進行中（`.git/rebase-merge`／`rebase-apply` 存在）不計分；amend 以「HEAD 的 parent 是否與上次相同」判斷。也要處理 `core.hooksPath` 與既有 post-commit hook 的串接。
- **改動行數灌水**（迭代 1）：lockfile、產生碼、`--allow-empty`。候選做法：行數取 log 並設上限、排除常見 lockfile。
- **hook 類指令一律 exit 0**（迭代 1）：第一個 hook 指令出現時，在 CLI 加一層包裝吞掉錯誤並以 fuzz 測試守住。
- **statusline 唯讀**（迭代 2）：不取鎖、不寫檔；使用者已有 statusLine 時，`install claude` 用 chain 包裝。
- **XP 曲線語意**（迭代 1）：`18 × N^1.3` 是「N → N+1 所需」還是累積值；驗收「第一次 commit 升到 Lv 2」要求第一筆 commit 至少給 18 XP。
- **未定義的規則**：物種清單、每日補給、紮營、`commit_mix`、迭代 6 的「待決事項」都沒有設計文件，邊做邊定，定了就記在上面「已採用」。
- **既有寵物的 `commit_mix`**（迭代 5）：迭代 1 就孵蛋，迭代 5 才建 `commit_mix`，需要對已孵的寵物補算一次。
