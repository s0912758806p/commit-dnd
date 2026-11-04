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

## 已採用（迭代 5）

40. **commit 組成（commit_mix）**：`Pet.Mix` 是固定七格的次數陣列（feat、fix、test、refactor、perf、docs、other；
    style、chore、ci、build、revert 與不符合 Conventional Commits 的標題都算 other），State 因此仍可用 `==` 比較。
    - `dnd hatch` 讀 `--repo`（預設目前資料夾）過去 90 天、`git config user.email` 這個作者的 commit（不含 merge），
      寫進 hatch 事件的 `mix` 參數（`feat=12,fix=3,…`）；引擎只解析參數、不碰 git。不是 repo 或沒有 commit 時從 0 開始。
    - 之後每筆**計分的** commit 依類型累加（同 hash 重跑、孵蛋前的 commit 不算）。
    - **既有寵物補算**：舊的 hatch 事件沒有 `mix`，重播已記錄的 commit 就會得到組成，不另外讀 git。
      `StateVersion` 升為 6，舊 state.json 自動從 events.jsonl 重建。
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
43. **轉職卷軸**：打倒 Boss 時 50% 機率掉落（種子同第 28 條，用途 `scroll`，重播結果不變），最多帶 3 張。
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
45. **加成的算法**：職業＋特化＋進化的百分比相加，乘在**不含裝備**的基礎值上（小數捨去），裝備強度另外加：
    攻擊 = (5 + 2 × 等級) × (100 + 攻擊加成)% + 武器、防禦 = 等級 × (100 + 防禦加成)% + 防具、
    HP 上限 = (20 + 5 × (等級 − 1)) × (100 + HP 加成)%。HP 上限改由 `engine.MaxHP(pet)` 計算（原本的 `content.MaxHP(level)`
    變成基礎值）。升級時 HP 照舊回滿到新上限；選特化、用卷軸讓上限改變時 HP 不回復，只夾在新上限內（只有 commit 能回血）。
46. **`dnd sim [repo]`**：重播 repo 的 git log（不含 merge、依作者時間排序），用正式的 `engine.Apply` 在記憶體跑，
    不讀寫 DND_HOME、不寫 repo。孵蛋在第一筆 commit 前一分鐘；`--from YYYY-MM-DD` 改成在那天孵蛋，前 90 天的 commit 當孵蛋時的組成。
    `--ai-turns-per-commit N` 在每筆 commit 前插入 N 個假的 AI 回合（每回合 `--edits-per-turn` 次 Edit，預設 2），
    平均分布在與上一筆 commit 之間、最多 5 分鐘一個。沒到 Lv 30 時，用「累積 XP ÷ 日曆天數」推估第幾天會到。

    **驗收（到 Lv 30 落在 1 到 2 個月）**：合成的一般玩法（每天固定筆數、9 點起每小時一筆、類型依上面的平均組成、每筆 40 行），
    由 `internal/sim.TestTypicalPlayReachesLv30InOneToTwoMonths` 綁住下面三個數字：
    - 每天 5 筆：第 68 天到 Lv 30（Lv 5／10／20 在第 1／5／27 天）
    - 每天 8 筆：第 49 天到 Lv 30（Lv 5／10／20 在第 1／4／19 天）——第 13 條的設計基準，落在 1–2 個月內
    - 每天 12 筆：第 36 天到 Lv 30（Lv 5／10／20 在第 1／3／14 天）

    使用者的真實 repo（2026-11-04 快照，`--author s0912758806p --ai-turns-per-commit 5`，種族人族）：

    | repo | commit | 期間 | Lv 5 | Lv 10 | Lv 20 | Lv 30 | Lv 5 職業 | 地下城最深 |
    | --- | --- | --- | --- | --- | --- | --- | --- | --- |
    | Therapi-backstage | 1218 | 115 天 | 第 1 天 | 第 1 天 | 第 8 天 | **第 28 天** | 法師 | B384（倒地 755） |
    | commit-dnd | 22 | 6 天 | 第 1 天 | — | — | 推估第 123 天 | 法師 | B19 |
    | agentic-sop-to-work | 126 | 144 天 | 第 2 天 | 第 3 天 | — | 推估第 489 天 | 法師 | B74 |

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

## 待決

- **statusline 唯讀**（迭代 2）：不取鎖、不寫檔；使用者已有 statusLine 時，`install claude` 用 chain 包裝。
- **未定義的規則**：迭代 6 的「待決事項」沒有設計文件，邊做邊定，定了就記在上面「已採用」（紮營見第 27 條、`commit_mix` 見第 40 條）。
- **Lv 5 判定的樣本數**（迭代 5）：新 repo、沒有 commit_mix 的舊寵物在 Lv 5 時只有 6–8 筆 commit，判定容易被開頭的 docs 左右（第 46 條）。要不要改成「至少 N 筆才判定」或延後判定等級，等實際玩一陣子再決定。
- **拿不到鎖時事件遺失**：hook 等鎖超過 50 ms 會放棄並寫 errors.log，那一筆 commit 不計分。實際玩一陣子看 errors.log 是否出現，再決定要不要加暫存佇列。
- **`dnd debug` 指令**：`debug time`、`debug xp` 等（DEVPLAN「自我測試」一節）尚未實作，目前只有 `debug rebuild`。
