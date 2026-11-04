# 設計筆記

DEVPLAN（2026-10-30）之外、實作時定下的決定。每條註明在哪一輪採用；推翻時在該條頂端加註，不改寫原文。

## 已採用（迭代 0）

1. **鎖涵蓋整個更新**：DEVPLAN 的 `Store.Commit` 只在寫入時取鎖，`Load → Apply → Commit` 之間會被並行的 hook 互相蓋掉。
   改為 `store.Update(e)`：取鎖 → 讀 state → 指派 Seq → `engine.Apply` → 追加 events.jsonl → 原子寫 state.json → 放鎖。
   取鎖逾時 `store.DefaultLockTimeout`（50 ms）回傳 `ErrBusy`，由呼叫端決定是否靜默放棄。
2. **Event 新增欄位**：`V`（事件格式版本）、`Seq`（store 在鎖內指派，連續遞增）、`Args map[string]string`（hatch 物種、warp 樓層、debug 數值等參數）。
3. **亂數種子**（迭代 3 實作，見第 28 條）：`hash(最後一次 commit hash, 樓層, Seq)`，讓沒有 commit hash 的 AI 回合也有確定的種子。
4. **tick 不落地，衰減必須路徑無關**：status／statusline 只在記憶體套用 tick。衰減以「累積未滿一週期的時間」（`Pet.*Carry`）實作，
   一次推進 Δ1+Δ2 與分兩次結果完全相同；`engine.TestTimeEffectsArePathIndependent` 守住這條。之後新增的任何時間效果都要加進這個測試。
5. **時間倒流**：`e.At < UpdatedAt` 時 Δ 視為 0，`UpdatedAt` 只前進。`debug time` 的偏移存在 `DND_HOME`，由 adapter 的 `now` 統一加上（尚未實作，見待決）。
6. **數值一律整數**：心情等以千分位表示；XP 曲線 `18 × N^1.3` 在 content 載入時預先算成整數表（迭代 1）。
7. **content 版本**：`State.ContentHash` 記錄套用時的 balance 雜湊。Rebuild 的保證是「同一份 content 下相同」；調數值後重播得到不同結果是預期行為（`dnd sim` 的用途）。
8. **state.json 帶 `events_bytes`**：與 events.jsonl 大小不符代表上次寫到一半，下次 Update 先重播補上；結尾的半行會被截掉，中間行損毀則回報錯誤。
9. **YAML 套件**：`go.yaml.in/yaml/v3`（`gopkg.in/yaml.v3` 已封存）；`content/` 放在 repo 根目錄以符合 DEVPLAN 的 `content/*.yaml` 路徑。
10. **Go 版本**：依賴（`golang.org/x/sys`）要求 Go 1.25，因此最低版本由 DEVPLAN 的 1.24 提高為 1.25。
11. **XP 曲線是逐級成本**（2026-10-30 決定，迭代 1 實作）：`18 × N^1.3` 四捨五入為整數，是「從 Lv N 升到 N+1」需要的 XP，不是累積值。
    - Lv1→2 為 18、Lv4→5 為 109、Lv14→15 為 556、Lv29→30 為 1433；到 Lv30 共 18,795 XP。
    - 迭代 5 的目標「1 到 2 個月到 Lv 30」因此換算成**每天約 313–626 XP**，這是調 commit 給分與每日補給時的基準。
    - 不採累積值：那樣到 Lv30 只要 1,498 XP，且 Lv29→30 只差 65 XP，後期每級成本幾乎持平，失去越升越難的感覺。
    - 迭代 1 的驗收「第一次 commit 升到 Lv 2」：第一筆 commit（含每日補給）至少要給 18 XP。

## 已採用（迭代 1）

12. **事件保留時區**：迭代 0 的 store 把 `At` 轉成 UTC；迭代 1 改為保留 adapter 給的時區，只去掉單調時鐘，
    因為每日補給要依 commit 的當地日期計算。`StateVersion` 升為 2，舊版 state.json 會自動從 events.jsonl 重建。
13. **commit 計分**（數值在 `content/balance.yaml`）：
    - XP = 類型基本值（feat 30、fix 25、test／refactor／perf 20，其餘 10）＋ 行數加成（2 × 行數的二進位位數，上限 20）。
    - 每日補給：當地每天前 3 筆 commit 的 XP ×2。飽足低於 200 時 XP 減半（先判斷再餵食）。
    - 每筆 commit 飽足 +150、HP +5；升級時 HP 回滿。
    - 以平均每天 8 筆、每筆約 35 XP 估算，每天約 385 XP，落在每天 313–626 XP 的目標內。
    - 最低基本值 10 × 補給 2 ≥ Lv1→2 的 18 XP，保證第一筆 commit 升到 Lv 2（`content.TestFirstCommitAlwaysReachesLevel2`）。
14. **post-commit 只認一般 commit**：hook 讀 HEAD 最新一筆 reflog，只有 `commit:` 與 `commit (initial):` 計分；
    amend、cherry-pick、rebase、merge 都會跑 post-commit 但不重複給 XP。另外同一個 hash 只計分一次（hook 重跑、重複安裝）。
15. **行數灌水**：略過二進位檔與常見 lockfile（`go.sum`、`package-lock.json` 等），行數加成本身也有上限。
16. **hook 一律成功**：`dnd hook git` 不輸出、永遠 exit 0，錯誤與 panic 寫進 `$DND_HOME/errors.log`。
    hook 區塊插在既有 post-commit 的 shebang 之後（原腳本中途 `exit` 也不影響），非 shell 腳本則拒絕安裝；寫入 dnd 的絕對路徑。
17. **日誌不另存**：`dnd log` 每次從 events.jsonl 重播產生；改日誌模板後，舊紀錄也會用新文字顯示。
18. **物種**：史萊姆（slime）、幼龍（drake）、貓頭鷹（owl）、人類騎士（knight，2026-10-30 依使用者要求新增），定義在 `content/species.yaml`。
    > ⚠ 已被第 20 條推翻：物種改為八個種族，slime／drake／owl 移除，knight 併入人族（2026-10-30）
19. **status 只顯示文字**（2026-10-30 使用者決定）：不畫物種圖像，也不畫數值條，只列出名字、物種、職業、進化、等級、HP、飽足、心情與補給。
    這推翻了 DEVPLAN 迭代 1「3 物種 × 第一階段 ASCII 圖」與迭代 5「物種 × 3 階段外型與職業配件疊加」：迭代 5 只需要實作職業與進化的**數值與名稱**，不需要畫外型。
    `Pet` 新增 `Class`（空字串＝未轉職）與 `Stage`（孵出時為 1），`StateVersion` 升為 3，舊 state.json 自動從 events.jsonl 重建。
20. **物種改為八個種族**（2026-10-30 使用者決定）：人族（human）、精靈族（elf）、獸人族（orc）、鼠人族（ratkin）、
    矮人族（dwarf）、不死族（undead）、惡魔族（demon）、龍人族（dragonkin）。
    - 物種 id 寫在 events.jsonl 裡，不能直接刪。`species.yaml` 新增 `aliases`：舊 id 重播時對應到新物種，孵出時一律存正式 id。
    - 舊的 `knight`（人類騎士）設為人族的 alias，正式遊玩中的 GR騎士因此成為人族；「騎士」留給之後的職業系統。
    - slime、drake、owl 沒有正式存檔用過，直接移除。

## 已採用（迭代 2）

21. **AI 活動只計數**：Claude Code 的 Stop → `turn`、PostToolUse → `tool_use`，記在 `State.AI`（累計回合、工具呼叫、當天回合），
    不改變寵物，也不產生冒險日誌（工具呼叫太頻繁）。地下城在迭代 3 才接上。`StateVersion` 升為 4。
22. **hook 用 async、永遠 exit 0**：官方文件（2026-11-04 查證）指出 Stop hook 若 exit 2 會讓 Claude 停不下來，
    所以 `dnd hook claude` 對任何輸入都安靜成功（`TestHookClaude` 守住）；安裝時設 `"async": true`，不阻塞 Claude。
23. **不保存 AI 的內容**：事件只留 hook 名稱、工具名稱、repo 資料夾名；`tool_input`、`tool_response`、`session_id`、
    `transcript_path` 都不寫進 events.jsonl。測試樣本也去識別化後才放進 repo。
24. **statusline 串接而不取代**：使用者原本的 statusLine 存到 `$DND_HOME/claude/original-statusline.json`，
    `dnd statusline` 先執行它（同一份 stdin、1 秒上限）再接上寵物資訊；uninstall 時還原。statusline 永遠 exit 0，
    因為 Claude Code 只顯示 exit 0 的輸出。
25. **AI 事件量**：每次工具呼叫都寫一行事件，一天幾百行。`dnd status`／`statusline` 只讀 state.json 不受影響；
    `dnd log` 每次全量重播，若一年後變慢，再考慮定期快照。

## 已採用（迭代 3）

26. **地下城的核心流程**（規則在 `internal/engine/dungeon.go`）：
    - AI 回合（Stop）：眼前沒怪物時遇到下一隻，這層打完 `encounters_per_floor` 隻（或打倒 Boss）才往下探；有怪物就被攻擊。
    - 攻擊工具（Edit、Write、MultiEdit、NotebookEdit）呼叫一次＝寵物攻擊一次。真實使用時一個回合的工具呼叫先發生、Stop 最後，
      所以寵物先出手，回合結束時才輪到怪物。
    - 每 10 層一隻 Boss，打倒後解鎖該層傳送點；`warp` 只能去入口或已解鎖、不深於最深傳送點的 Boss 層。
    - 屬性：火剋草、草剋水、水剋火；相剋 ×1.5、被剋 ×0.7，武器影響造成的傷害、防具影響受到的傷害。
    - commit：先救起倒地的寵物（同一層、同一隻怪物，HP 50%），最後鑑定背包裡最舊的一件，比身上同種類的強才換上。
    - 倒地中 AI 不能推進、攻擊，HP 也不會自然回復或紮營；只有 commit 能救（DEVPLAN 的鐵律：AI 不能單獨回血或免除倒地）。
27. **使用者的三個決定**（2026-11-04）：打倒怪物不給經驗、只掉裝備（經驗只來自 commit）；倒地時失去所有未鑑定的裝備；
    最後一次 commit／AI 活動後閒置滿 8 小時就紮營，HP 回滿。
28. **亂數**：第 3 條的種子設計在此實作。用 splitmix64（自己實作，不依賴標準庫演算法版本），種子是
    「最後一次 commit hash、樓層、Seq、用途」的 FNV 雜湊；掉落物把種子存在背包裡，鑑定時才展開，重播結果不變。
29. **State 仍保持可比較**：未鑑定背包用固定大小陣列（`BagSize` = 10），滿了之後新的掉落物留在原地。
30. **數值以模擬調整**（2026-11-04 快照，正式內容、每回合 2 次編輯）：每層 6 隻、怪物每層成長 10%、Boss ×2。

    | 玩法 | 第 1 天 | 第 8 天 | 第 30 天 | 每天倒地 |
    | --- | --- | --- | --- | --- |
    | 每天 8 commit、40 回合 | B7 | B44 | B116、Lv 26 | 前 10 天 0 次，之後 1–3 次 |
    | 每天 3 commit、80 回合 | B13 | B51 | B105、Lv 21 | 2–4 次 |
    | 只用 AI、不 commit | 第 1 天在 B10 倒地 | — | 一直倒地 | — |

    第一版每層 1 隻、成長 8% 時，第 1 天就到 B26、兩週 B169，樓層數字失去意義，因此改成上面的數值。

## 待決

- **statusline 唯讀**（迭代 2）：不取鎖、不寫檔；使用者已有 statusLine 時，`install claude` 用 chain 包裝。
- **未定義的規則**：紮營、`commit_mix`、迭代 6 的「待決事項」都沒有設計文件，邊做邊定，定了就記在上面「已採用」。
- **既有寵物的 `commit_mix`**（迭代 5）：迭代 1 就孵蛋，迭代 5 才建 `commit_mix`，需要對已孵的寵物補算一次。
- **拿不到鎖時事件遺失**：hook 等鎖超過 50 ms 會放棄並寫 errors.log，那一筆 commit 不計分。實際玩一陣子看 errors.log 是否出現，再決定要不要加暫存佇列。
- **`dnd debug` 指令**：`debug time`、`debug xp` 等（DEVPLAN「自我測試」一節）尚未實作，目前只有 `debug rebuild`。
