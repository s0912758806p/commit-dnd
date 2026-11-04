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
   > ⚠ 「由 adapter 的 `now` 統一加上」已被第 50 條改寫：偏移由 `internal/clock` 加在 CLI 傳給 adapter 與 tick 的「現在」，且只在 `DND_DEBUG=1` 時生效（2026-11-04）
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

## 已採用（迭代 4：Codex）

31. **Codex 一回合＝攻擊一次＋回合**：Codex 的 notify 只在回合結束時通知，沒有個別的工具呼叫；只記 `turn` 的話，
    寵物永遠不會出手、只會挨打。所以 `agent-turn-complete` 產生兩個事件：先 `tool_use`（`Tool: "Codex"`），再 `turn`，
    並把 `Codex` 加進 `balance.yaml` 的 `dungeon.attack_tools`。順序跟 Claude Code 一樣是「先出手、回合結束才輪到怪物」。
    Claude 一個回合通常有好幾次 Edit，Codex 固定一次，所以同樣回合數下 Codex 打得比較慢；第 30 條的模擬是以
    Claude「每回合 2 次編輯」做的，Codex 玩家的節奏還沒有模擬，實際玩過再調。`attack_tools` 不分來源，
    Claude 若有一個工具剛好叫 `Codex` 也會算攻擊（目前沒有）。
32. **notify 的介面**（2026-11-04 查證官方文件與原始碼 `codex-rs/hooks/src/legacy_notify.rs`）：設定在
    `$CODEX_HOME/config.toml`（預設 `~/.codex`）最上層的 `notify`，是「程式＋參數」的字串陣列；Codex 把 JSON 附在
    **最後一個參數**，stdin／stdout／stderr 都是空的，只啟動、不等結束也不看 exit code。payload 欄位是 kebab-case：
    `type`（目前只有 `agent-turn-complete`）、`thread-id`、`turn-id`、`cwd`、`client`（選填）、`input-messages`、
    `last-assistant-message`。adapter 只讀 `type` 與 `cwd`（取資料夾名），缺 `cwd` 不產生事件；對話內容一律不保存。
    原始碼把 notify 標成 legacy，見「待決」。
33. **串接原本的 notify**：已有別人的 notify 時，原本的陣列與 TOML 原文存到 `$DND_HOME/codex/original-notify.json`，
    `notify` 換成 dnd。`dnd hook codex` 先寫自己的事件，再用同樣的參數（含 payload）啟動原本的 notify，
    跟 Codex 一樣不等它結束；兩段各自用 `runHook` 吞錯並寫 errors.log，任一邊失敗都不影響另一邊。
    `hook codex` 關掉 Cobra 的旗標解析，參數原樣轉交。uninstall 先試著還原原文（含註解、多行），原文解析結果對不上時才用陣列重寫。
34. **保留原文的 TOML 編輯**：用 `github.com/BurntSushi/toml` v1.6.0（2025-12 發布、仍在維護、無其他依賴）完整解析，
    但它不能保留原文寫回，所以自己寫一個只掃最上層 `key = value` 位置的掃描器，只替換 `notify` 的值、插入或刪除那一行。
    寫入前再解析一次，確認 notify 以外的資料完全相同（`verifyCodexEdit`），掃描器出錯時寧可報錯也不寫檔；
    `FuzzCodexInstall` 守住「合法 TOML 一定改得動，install → uninstall 回到原檔」。新增那一行時不額外加空行，
    uninstall 才能逐 byte 還原。寫入流程（diff、確認、備份、原子寫入）沿用 `ClaudePlan`，以型別別名 `ConfigPlan` 共用。

## 已採用（迭代 4：收服與隊伍）

DEVPLAN 迭代 4 只寫了「`fix:` 收服、合體技、圖鑑與隊伍」四個名詞，以下規則是實作時定的預設（2026-11-04），數值都在 `content/balance.yaml` 的 `party` 區塊，待使用者試玩後調整。

35. **收服**（規則在 `internal/engine/capture.go`）：`fix:` 的 commit 計分、鑑定之後，眼前有一般怪物就依機率收服。
    - 機率 = `capture_base_pct` + `capture_missing_hp_pct` × 怪物已損失 HP 的比例（整數運算）。亂數用 `seedFor(…, "capture")`，重播結果不變。
    - Boss 不能收服（寫一句日誌）；這筆 commit 之前倒地的話只救援、不收服，也不發動合體技——倒地的代價。
    - 成功：怪物離開戰鬥、`Cleared++`（算這層打倒一隻），但**不掉裝備、不算擊敗數**，讓「收服或打倒」各有取捨。失敗：寫一句日誌，怪物留著。
    - 只認 `fix:`：DEVPLAN 指定；也呼應 `fix` 日誌「拔掉陷阱上的機關」。其他類型的 commit 不收服。
36. **圖鑑與隊伍的資料結構**：State 必須保持可比較，所以圖鑑是 `Dex.Entries [content.MaxMonsters]DexEntry`（64 格）＋ `N`，
    依**第一次遇到的順序**以 id 為鍵記錄「遇過次數、收服次數、最深收服樓層」，而不是用 `monsters.yaml` 的位置當索引——
    之後新增、重排怪物都不影響舊存檔；`content` 載入時驗證怪物不超過 64 種。隊伍是 `Party.Members [3]string` ＋ `N`，
    上限 `engine.PartySize` = 3 寫在引擎（陣列大小必須是常數）。`StateVersion` 升為 6，舊 state.json 自動從 events.jsonl 重建。
    - 同種只能一隻；收服時隊伍有空位且沒有同種才自動入隊，滿了就只記進圖鑑並提示 `dnd party`（不自動替換，避免蓋掉使用者的選擇）。
    - 隊員加成：該怪物在**最深收服樓層**的攻擊力（同怪物成長公式）× `member_atk_pct`，至少 +1，加進 `PetAtk`。往深處再收服同種，隊員就變強。
    - `dnd party add|remove <id>` 寫 `party` 事件，由引擎驗證（不存在、沒收服過、已在隊伍、隊伍滿、不在隊伍、未知操作），不合法時寫一句日誌、狀態不變。
37. **合體技**：commit 前 `combo_window_minutes` 分鐘內有 AI 活動，這筆 commit 對眼前的怪物（含 Boss）造成
    寵物攻擊 × (`combo_base_pct` + `combo_per_member_pct` × 隊員數) 的傷害（武器屬性照算），打倒照常掉落、Boss 照常解鎖傳送點。
    - AI 活動另存 `Party.LastAI`（只有 turn、tool_use 更新），不用 `Dungeon.LastActive`，因為後者也包含 commit 本身。
    - **收服優先**：同一筆 `fix:` 收服成功就不再發動；先合體再收服的話，前期怪物幾乎都會被合體技打死，收服不到。
    - 合體技是 commit 的效果，只造成傷害；AI 仍然不能單獨回血或救倒地（DEVPLAN 的鐵律）。每筆 commit 都能發動，不限每次 AI 活動一次。
38. **數值**（`balance.yaml` 的 `party`；`content.TestPartyMatchesDesignNotes` 綁住）：收服 40% + 40% × 已損失 HP 比例（滿血 40%、快死近 80%）；
    隊員攻擊 25%；合體技 30 分鐘內、200% + 每位隊員 50%。
    - 起始建議是收服 25%、隊員 50%。模擬後改掉：25% 時一般玩法一個月只收集到 4–5 種一般怪物（前期樓層一兩天就走過，fix 的機會太少）；
      隊員 50% 時後期隊伍加成可達寵物攻擊的一倍（深處收服的怪物攻擊力隨樓層成長，寵物攻擊只隨等級成長）。
39. **數值模擬**（2026-11-04 快照，正式內容、每個玩法 3 組 commit hash；每天 9:00 起 40 個 AI 回合、每回合 2 次 Edit；
    commit 平均穿插在回合之間，fix 佔 2/8；隊伍只靠自動入隊，沒有手動換人）：

    | 玩法 | 第 7 天收服（種） | 第 30 天收服（種） | 第 30 天樓層 | 30 天倒地 |
    | --- | --- | --- | --- | --- |
    | 每天 8 commit（2 fix）、40 回合 | 7–10 次（5–6） | 23–30 次（6–8） | B126–138 | 30–43 |
    | 同上，但 commit 都在下班後（沒有合體技） | 4–6 次（4–5） | 22–24 次（6–7） | B81–89 | 19–22 |
    | 同第一列，第 8、16 天各回入口玩 2 天 | 7–10 次（5–6） | 18–31 次（7–8） | B116–117 | 11–23 |
    | 每天 8 commit（1 fix）、40 回合 | 3–5 次（2–4） | 12–14 次（4–5） | B139–146 | 26–33 |
    | 每天 3 commit（1 fix）、80 回合 | 0–1 次 | 0–1 次 | B69–78 | 約 86 |

    - 同一個模擬關掉迭代 4（不收服、不合體）時，第一列第 30 天是 B111–116（與第 30 條迭代 3 的快照一致）、第二列是 B75–76：
      合體技與隊伍讓第一列多下探 10–20 層，只有隊伍加成的第二列多 6–13 層。
    - 會手動把最強的候補換進隊伍時，第 30 天隊伍加成約 +51～+62（寵物攻擊 137–149 的四成），樓層 B145–153。
    - 最後一列幾乎每筆 commit 都在救援，所以收服不到、合體技也很少發動；這是倒地的代價，見下方待決。
    - 一般怪物共 10 種，但第 30 層之後只會出現 4 種；要收集齊全得用 `dnd warp 0` 回頭走一趟（第三列）。

## 已採用（迭代 5）

40. **commit 組成（commit_mix）**：`Pet.Mix` 是固定七格的次數陣列（feat、fix、test、refactor、perf、docs、other；
    style、chore、ci、build、revert 與不符合 Conventional Commits 的標題都算 other），State 因此仍可用 `==` 比較。
    - `dnd hatch` 讀 `--repo`（預設目前資料夾）過去 90 天、`git config user.email` 這個作者的 commit（不含 merge），
      寫進 hatch 事件的 `mix` 參數（`feat=12,fix=3,…`）；引擎只解析參數、不碰 git。不是 repo 或沒有 commit 時從 0 開始。
    - 之後每筆**計分的** commit 依類型累加（同 hash 重跑、孵蛋前的 commit 不算）。
    - **既有寵物補算**：舊的 hatch 事件沒有 `mix`，重播已記錄的 commit 就會得到組成，不另外讀 git。
      `StateVersion` 升為 7（和迭代 4 收服的 6 合併後再加一），舊 state.json 自動從 events.jsonl 重建。
41. **職業判定**（`content/classes.yaml`，規則在 `internal/engine/class.go`）：升到 Lv 5 時判定一次，之後不會自己改變。
    每個職業的分數 = 它負責的分類次數 × `weight_pct`，最高者當選、同分取清單前面的。
    權重用來抵銷「feat 本來就最多」：使用者幾個 repo 的平均組成約 feat 45%、docs 20%、fix 17%、test 10%、refactor＋perf 3%，
    比平均明顯多的那一類才勝出。

    | id | 職業 | 類型 | 計分分類 | 權重 | 加成（攻擊／防禦／HP） |
    | --- | --- | --- | --- | --- | --- |
    | ranger | 遊俠 | 開拓型 | feat | 100 | +20% ／ 0 ／ +10% |
    | cleric | 神官 | 修補型 | fix | 150 | +5% ／ +5% ／ +20% |
    | knight | 騎士 | 守護型 | test | 250 | 0 ／ +20% ／ +10% |
    | artificer | 鍊金術士 | 工匠型 | refactor、perf | 300 | +10% ／ +10% ／ +10% |
    | mage | 法師 | 學者型 | docs | 150 | +25% ／ 0 ／ +5% |
    | adventurer | 冒險者 | 萬事通型 | other | 100 | +10% ／ +5% ／ +15% |

    以這些權重，使用者的 repo 長期組成判定為：Therapi-backstage、agentic-sop-to-work、commit-dnd → 遊俠；
    Omnik、claude-workspace（fix 多）→ 神官（2026-11-04 用 `git log` 統計）。
42. **特化**（Lv 15 起，`dnd class choose <id>`）：每個職業兩個方向，選一次；要換要用轉職卷軸。
    引擎驗證「已轉職、Lv ≥ 15、是自己職業的特化、還沒選過」，不合法時寫一句日誌、狀態不變。這一版特化只有數值差異。

    | 職業 | 特化 1 | 特化 2 |
    | --- | --- | --- |
    | 遊俠 | marksman 神射手（攻擊 +20%） | wayfarer 荒野行者（防禦 +5%、HP +15%） |
    | 神官 | exorcist 驅魔師（攻擊 +15%、HP +5%） | oracle 先知（防禦 +10%、HP +10%） |
    | 騎士 | paladin 聖騎士（攻擊 +10%、防禦 +5%、HP +5%） | bulwark 鐵壁騎士（防禦 +15%、HP +5%） |
    | 鍊金術士 | runesmith 符文鍛造師（攻擊 +10%、防禦 +10%） | apothecary 秘藥師（HP +20%） |
    | 法師 | elementalist 元素術士（攻擊 +20%） | sage 賢者（防禦 +10%、HP +10%） |
    | 冒險者 | mercenary 傭兵（攻擊 +15%、防禦 +5%） | scavenger 拾荒者（防禦 +5%、HP +15%） |
43. **轉職卷軸**：打倒 Boss 時（一般攻擊或合體技，都在 `defeat()` 裡）50% 機率掉落（種子同第 28 條，用途 `scroll`，重播結果不變），最多帶 3 張。
    `dnd class reroll` 用掉一張，依**當下**的 commit 組成重新判定並清除特化，等級與 XP 不變；
    判定結果和現在一樣、又沒有特化可重置時，不消耗卷軸（只寫日誌）。Boss 層打過之後只能靠傳送回去再往下打到下一隻 Boss，
    卷軸沒辦法無限刷。
44. **進化**：Lv 1 第 1 階段、Lv 10 第 2 階段、Lv 20 第 3 階段，升階時寫日誌；加成 +0% ／ +5% ／ +10%（攻防 HP 都加，不累加前一階段）。
    名稱只有文字（第 19 條）：

    | 種族 | 第 1 階段 | 第 2 階段 | 第 3 階段 |
    | --- | --- | --- | --- |
    | 人族 | 見習者 | 勇者 | 英雄 |
    | 精靈族 | 幼精靈 | 森林精靈 | 上古精靈 |
    | 獸人族 | 獸人幼崽 | 部落勇士 | 獸人大酋長 |
    | 鼠人族 | 小鼠人 | 溝渠潛行者 | 鼠王 |
    | 矮人族 | 矮人學徒 | 山丘矮人 | 山岳之王 |
    | 不死族 | 骷髏 | 幽魂 | 巫妖王 |
    | 惡魔族 | 小鬼 | 惡魔 | 魔王 |
    | 龍人族 | 幼龍人 | 龍裔 | 古龍王 |
45. **加成的算法**：職業＋特化＋進化的百分比相加，乘在**不含裝備**的基礎值上（小數捨去），裝備強度與隊伍加成最後另外加、不乘職業倍率：
    攻擊 = (5 + 2 × 等級) × (100 + 攻擊加成)% + 武器 + 隊伍加成（第 36 條的 `PartyAtk`）、防禦 = 等級 × (100 + 防禦加成)% + 防具、
    HP 上限 = (20 + 5 × (等級 − 1)) × (100 + HP 加成)%。HP 上限改由 `engine.MaxHP(pet)` 計算（原本的 `content.MaxHP(level)`
    變成基礎值），所有路徑（commit、`debug_xp` 升級、救援、紮營、回血）都用它。升級時 HP 照舊回滿到新上限，
    commit 與 `debug_xp` 共用同一個升級流程（`levelUp`），轉職與進化在兩者都會觸發；選特化、用卷軸讓上限改變時 HP 不回復，只夾在新上限內（只有 commit 能回血）。
46. **`dnd sim [repo]`**：重播 repo 的 git log（不含 merge、依作者時間排序），用正式的 `engine.Apply` 在記憶體跑，
    不讀寫 DND_HOME、不寫 repo。孵蛋在第一筆 commit 前一分鐘；`--from YYYY-MM-DD` 改成在那天孵蛋，前 90 天的 commit 當孵蛋時的組成。
    `--ai-turns-per-commit N` 在每筆 commit 前插入 N 個假的 AI 回合（每回合 `--edits-per-turn` 次 Edit，預設 2），
    平均分布在與上一筆 commit 之間、最多 5 分鐘一個。沒到 Lv 30 時，用「累積 XP ÷ 日曆天數」推估第幾天會到。

    **驗收（到 Lv 30 落在 1 到 2 個月）**：合成的一般玩法（每天固定筆數、9 點起每小時一筆、類型依上面的平均組成、每筆 40 行），
    由 `internal/sim.TestTypicalPlayReachesLv30InOneToTwoMonths` 綁住下面三個數字：
    - 每天 5 筆：第 68 天到 Lv 30（Lv 5／10／20 在第 1／5／27 天）
    - 每天 8 筆：第 49 天到 Lv 30（Lv 5／10／20 在第 1／4／19 天）——第 13 條的設計基準，落在 1–2 個月內
    - 每天 12 筆：第 36 天到 Lv 30（Lv 5／10／20 在第 1／3／14 天）

    使用者的真實 repo（2026-11-04 快照，已含迭代 4 的隊伍加成與合體技；`--author s0912758806p --ai-turns-per-commit 5`，種族人族；commit-dnd 是迭代 3 合併時的 main）：

    | repo | commit | 期間 | Lv 5 | Lv 10 | Lv 20 | Lv 30 | Lv 5 職業 | 地下城最深 |
    | --- | --- | --- | --- | --- | --- | --- | --- | --- |
    | Therapi-backstage | 1218 | 115 天 | 第 1 天 | 第 1 天 | 第 8 天 | **第 28 天** | 法師 | B479（倒地 798） |
    | commit-dnd | 22 | 6 天 | 第 1 天 | — | — | 推估第 123 天 | 法師 | B20 |
    | agentic-sop-to-work | 126 | 144 天 | 第 2 天 | 第 3 天 | — | 推估第 489 天 | 法師 | B86（倒地 4） |

    Therapi-backstage 前 28 天平均每天約 15 筆，略快於 1 個月；另外兩個 repo 是輕度使用（每天 1–4 筆），遠超過 2 個月。
    三個 repo 都在第 1–2 天、只有 6–8 筆 commit 時就到 Lv 5，而新專案開頭多半是 docs，所以全部判定為法師；
    結束時的組成則全部會是遊俠。用 `--from 2026-09-01`（帶著前 90 天的組成孵蛋）重跑時，兩個 repo 都在 Lv 5 判定為遊俠——
    真實孵蛋會讀 90 天的 git log，這個「開頭樣本太少」的問題只發生在新 repo 或沒有 commit_mix 的舊寵物。
47. **既有寵物（GR騎士）的影響**：`StateVersion` 升級後會從 events.jsonl 重播，所以 Lv 5 那一刻會依當時累積的 commit 組成
    補上職業，之後的戰鬥都帶著職業加成重算，地下城的樓層、倒地次數、裝備可能和升級前看到的不同（第 7 條：換內容後重播不同是預期行為）。
    等級與 XP 不受影響（經驗只來自 commit）。
48. **名稱不和種族重複**：職業、特化、進化名稱彼此不重複，職業名也不和種族名互相包含（`content.TestGrowthNamesDistinct`）。
    第 20 條保留的「騎士」用在守護型職業。新內容放在新檔 `content/classes.yaml`（含日誌模板），避免和其他迭代同時改 logs.yaml、balance.yaml。
49. **給之後調整的旋鈕**：職業與特化的名稱、權重、加成，進化門檻與名稱，卷軸掉率與上限，全部在 `content/classes.yaml`，
    改完重新編譯、用 `dnd sim` 比較即可。還沒做的：特化只有數值、沒有獨特能力；孵蛋時 90 天的組成和之後的 commit 同權重，
    累積越多越難靠卷軸改變職業。

## 已採用（除錯指令）

DEVPLAN「自我測試與迭代流程」一節的 `dnd debug` 指令（2026-11-04 實作）。全部只在 `DND_DEBUG=1` 時註冊，開發測試一律搭配 `DND_HOME=./tmp/dnd`。

50. **時間偏移**（`internal/clock`）：偏移存在 `$DND_HOME/debug-time-offset`（Go 的 duration 字串），`clock.Now()` = 真實時間 + 偏移。
    - `cmd/dnd` 裡所有產生事件（hook git／claude、event、hatch、warp、party、debug xp／floor）與在記憶體套用 tick（status、statusline、dungeon、dex／party）的地方都改用 `clock.Now()`；
      引擎仍然不讀時鐘。errors.log 的時間戳與 `install claude` 的備份檔名是給人看的紀錄，維持真實時間。
    - **只在 `DND_DEBUG=1` 時生效**：正式使用時就算檔案殘留也讀不到；`dnd doctor` 看到殘留檔會用 `!` 提醒。檔案壞掉時 `Now()` 退回真實時間，hook 不會因此失敗。
    - `debug time +8h`／`-1h` 是**累加**，`reset` 刪檔，不帶參數顯示目前偏移。關掉 Cobra 的旗標解析，`-1h` 才不會被當成旗標。
    - 往回調時，新事件的 `At` 會早於 `UpdatedAt`，依第 5 條 Δ 視為 0：時間效果不會倒退，只是暫停到追上為止。
51. **`debug_xp`／`debug_floor` 是真正的事件**（規則在 `internal/engine/debug.go`，`Apply` 的 switch 只加分派）：寫進 events.jsonl，所以重播、Rebuild 結果一致；
    引擎不看 `DND_DEBUG`，只有 CLI 決定提不提供指令。
    - `debug_xp`：XP 直接加上去，再走和 commit 共用的 `levelUp`（連續升級、每級 HP 回滿）；不吃補給倍率、飢餓減半，也不算 commit 次數。
    - `debug_floor`：直接設定樓層、清掉眼前的怪物、`Cleared` 歸零，`Deepest` 取較大值。倒地狀態、傳送點、背包不變，
      下一個 AI 回合在新樓層遇到第一隻怪物（Boss 層就是 Boss）；`0` 是入口，下一回合往下探到 B1。
    - 日誌文字寫死在 `debug.go`、開頭「【除錯】」，不放進 `content/logs.yaml`：它們只給開發者看，`dnd log` 裡也一眼認得出來。
52. **`debug event` 與 `dnd event` 是同一個指令**（`cmd/dnd/event.go`），不做兩套。`repo=`、`tool=` 填進 `Event.Repo`／`Event.Tool`
    （`claude tool_use tool=Edit` 才會攻擊）；type 為 `commit` 時一定帶 `CommitInfo`（`kind` 預設 `other`、`added`、`deleted`、`hash`），
    沒給 hash 時產生 `debug-<時間>-<序號>`，否則第 14 條會把它當成同一筆不計分。`--count N` 一次寫 N 筆（同一個時間、各自的 Seq 與 hash）。
    `dnd event` 本來就不需要 `DND_DEBUG` 也能用，因此現在也能寫出會計分的 commit；這是單機遊戲、events.jsonl 本來就能手改，防作弊不是目標。
53. **snapshot 只複製遊戲資料**：`events.jsonl`、`state.json`、`debug-time-offset`、`errors.log` 存到 `$DND_HOME/snapshots/<名稱>/`；
    `lock`、`capture/`、`claude/`（安裝 Claude Code 時存的原本 statusLine，跟著真實設定走）不在快照裡。
    - save 與 load 都在 store 的寫入鎖內（`store.FileStore.WithLock`，逾時放寬為 2 秒），不會和 hook 同時寫；每個檔案先寫暫存檔再 rename。
    - load 先把目前的資料另存成 `before-load-<真實時間>`，另存失敗就不載入；快照裡沒有的檔案會從 `DND_HOME` 刪掉（例如快照沒有偏移，載回後也沒有）。
    - 同名 save 需要 `--force`；名稱不能含路徑分隔符號，也不能是 `.`、`..`。
54. **stats 從狀態差值算**（`store.FileStore.Walk` 逐筆給「事件、套用前狀態、結果」）：不另外解析事件內容，和引擎的判斷永遠一致。
    - 日期用事件 `At` 自己的時區（和每日補給同一個「當地日期」），只列有事件的日子。
    - XP 是累積 XP（已升過的每級成本 + 本級 XP）的差，含 `debug_xp`；commit 是 `Pet.Commits` 的差（只算計分的）；AI 回合、倒地同理。
    - 最深是當天事件前後到過的最大樓層；下探只算 AI 回合造成的往下探，不含 `warp` 與 `debug_floor`。最後一行總計，平均下探 = 總下探 ÷ 有事件的天數。

## 已採用（迭代 6）

55. **使用者的四個決定**（2026-11-04）：終職每個特化一種（12 種）；轉生保留圖鑑與隊伍；Lv 30 且已取得終職才能轉生、
    由使用者執行 `dnd rebirth`；終職能力以玩法類為主（不是純戰鬥數值）。DEVPLAN 迭代 6 寫的「依待決事項的結論」
    指的待決事項並不存在，以這四個決定為準。
56. **終職的取得**（規則在 `internal/engine/rebirth.go`）：等級 ≥ `final_level`（30）且已選特化時，依特化取得終職，
    每一輪只取得一次；Lv 30 才選特化的話，選的那一刻取得。終職記在 `Pet.Legacy`（固定大小陣列，依第一次取得的順序），
    再次取得同一個終職時等級 +1。這一輪的終職在 `Pet.Final`，轉生時清空。`StateVersion` 升為 8。
57. **12 種能力**（`content/classes.yaml` 的 `finals`；效果 = `per_level` × 終職等級）：合體技倍率、飽足下降變慢（最多 −90%）、
    收服機率（總機率最多 100%）、救起時 HP（最多 100%）、某類 commit 的經驗、倒地保留未鑑定裝備（最舊的先留）、鑑定強度、
    commit 回血、屬性相剋傷害（只在寵物相剋時加）、每日補給次數、隊伍攻擊加成、轉職卷軸掉落率。
    每種能力都在規則裡生效（`TestAbilitiesInRules`、`TestAbilitiesWithRealContent`）。
    **AI 不能回血或免除倒地**：`TestAbilitiesNeverLetAIHeal` 讓寵物持有全部 12 種能力各 Lv 5，
    每個 AI 事件之後的 HP 都不可超過「同一時間只推進時間」的 HP（時間經過的自然回血與紮營不算 AI 回血），倒地中也不會被救起。
58. **轉生**：重置等級、XP、職業、特化、進化、卷軸、裝備、背包、樓層、最深樓層與傳送點；保留種族、名字、commit 組成、
    圖鑑、隊伍、`Legacy`，以及飽足、心情、補給與戰績（擊敗、倒地次數）。每次轉生累加基礎能力 `stats_pct`（10%）
    與 commit XP `xp_pct`（20%），數值照 DEVPLAN。基礎能力加成和職業、特化、進化一樣乘在不含裝備的值上（第 45 條）。
    能不能轉生由 `engine.CanRebirth` 判斷，引擎與 `dnd class`／`dnd rebirth` 共用。
59. **驗收**（`TestTwoRebirthsStackAndGoDeeper`、CLI 的 `TestRebirthFlow`）：用除錯 XP 快轉到 Lv 30、取得同一個終職三次、
    轉生兩次，能力疊加為 Lv 3、轉生加成 +20%／+40%。深度比較（2026-11-04 快照，同樣從 Lv 1 開始玩 20 天：
    每天 8 筆 feat commit、40 個 AI 回合、每回合 2 次 Edit）：

    | | 第 20 天 | 最深 |
    | --- | --- | --- |
    | 沒轉生 | Lv 22 | B98 |
    | 轉生兩次（只算終職能力與轉生加成） | Lv 25 | B108（+10%） |
    | 轉生兩次＋保留的隊伍（三隻在 B90 收服的隊員） | Lv 25 | B127（+30%） |

    DEVPLAN 的 +10%／+20% 本身只讓深度多一成；轉生後「明顯更深」主要來自保留的隊伍與終職能力。
    若實際玩起來覺得轉生回饋不夠，優先調 `rebirth.stats_pct`。

## 待決

- **未定義的規則**：迭代 6 的「待決事項」沒有設計文件，邊做邊定，定了就記在上面「已採用」（紮營見第 27 條、`commit_mix` 見第 40 條）。
- **Lv 5 判定的樣本數**（迭代 5）：新 repo、沒有 commit_mix 的舊寵物在 Lv 5 時只有 6–8 筆 commit，判定容易被開頭的 docs 左右（第 46 條）。要不要改成「至少 N 筆才判定」或延後判定等級，等實際玩一陣子再決定。
- **拿不到鎖時事件遺失**：hook 等鎖超過 50 ms 會放棄並寫 errors.log，那一筆 commit 不計分。實際玩一陣子看 errors.log 是否出現，再決定要不要加暫存佇列。
- **Codex notify 是 legacy**（迭代 4）：Codex 原始碼把 notify 標成為了相容而保留，並註記之後要移除；
  新的 hooks 機制（stdin、snake_case 欄位）文件上沒有提到 notify。notify 被移除時，要改成 hooks 版的 adapter 與 installer。
- **迭代 4 待使用者決定**（2026-11-04）：倒地被救起的那筆 commit 要不要也能收服／合體（目前不行）；
  隊伍滿時要不要自動換上更強的新收服（目前不換，只提示）；第 35–38 條的數值。
