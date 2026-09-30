# File Index System V1.0 規格書

文件版本：1.1（依目前 README 同步）\
整理日期：2026-09-30  
文件語言：繁體中文  
用途：功能、資料模型、介面、建置與發行規格；區分目前行為及待整合項目。

## 1. 文件依據與決策優先序

本文件保留 V1.0 功能與 API 設計基線，並依目前 [README.md](README.md) 同步已公開的操作、建置、跨平台圖示封裝與 Portable 行為。README 明確描述的目前行為優先於舊版規格；其未涵蓋的詳細契約仍保留為設計基線，不代表全部已實作或完成平台驗證。文件版本更新不改變 SQLite Schema 版本。

整合時已套用下列修訂：

- Storage 資料欄位 `scan_root` 拆為 `last_scan_root` 與 `current_root`。
- Open Folder 與即時可用性判斷使用 `current_root`。
- Relocate 僅修改 `current_root`，保留上次成功掃描的所有歷史資料。
- API 統一掛在 `/api/v1`；新增 Storage 透過首次 Scan 成功後建立，不另設 `POST /storages`。
- CSV 編碼依最終技術基線採 UTF-8 + BOM。

本次同步包含四段式檔名／路徑排序、大小與日期顯示、頂部掃描／結束控制、跨平台選取檔案、圖示資源、發行腳本與 macOS bundle 外的資料目錄。原生目錄／資料庫選擇器及 Filesystem identity 比對仍待整合，目前使用經驗證的路徑欄位。UI Wireframe 是功能示意；目前畫面以 README 的截圖與操作說明為準。

## 2. 系統定位與範圍

File Index System 是個人使用、離線、跨平台、Portable 的檔案中繼資料索引工具。使用者主動掃描磁碟或目錄，將檔案 Metadata 保存至 SQLite；來源磁碟拔除後仍可搜尋，確認檔案位於哪個 Storage 與目錄。

```text
使用者選擇磁碟／目錄 → 掃描 Metadata → SQLite
                                         ↓
                            來源離線後仍可搜尋與查看位置
```

系統按需啟動，不常駐、不監控插拔、不自動重新掃描。非同步 Scan Job 是使用者啟動後在本次程序內執行的工作，不代表常駐背景掃描服務。

### 2.1 功能範圍

| 功能 | V1 行為 |
|---|---|
| Search | 檔名／相對目錄部分比對、多關鍵字 AND、篩選、排序、分頁 |
| File Details | 顯示索引資訊及來源可用狀態 |
| Open Folder | 來源可用時開啟所在目錄，在支援的平台選取索引檔案 |
| Storage | 新增、掃描、重掃、定位、檢視、編輯、匯出、刪除 |
| Scan／Rescan | 遞迴讀取 Metadata，使用 staging 保護正式索引 |
| CSV | 匯出全部搜尋結果或單一 Storage 索引 |
| Backup／Restore | 安全備份 SQLite、完整還原資料庫 |
| Settings | 讀寫 Portable Root 下的 JSON 設定 |

### 2.2 明確排除

- `.fileindex-id` 標記檔、自動 Storage 配對。
- directories 資料表、檔案內容索引／搜尋、Hash、重複檔案偵測。
- 常駐服務、背景自動掃描、排程掃描、File Watcher、插入偵測、自動增量掃描。
- 雲端、帳號、登入、多使用者服務。
- Regex、OR／NOT 語法、複雜布林搜尋。
- 直接開啟檔案內容。
- V1 預設導入 FTS5／trigram；是否採用留待實測與後續版本評估。

## 3. 技術 Stack 與平台

### 3.1 已選定技術

| 層次 | 技術基線 | 用途 |
|---|---|---|
| Backend | Go | Scanner、Search、Storage、Job Manager、Backup、OS Adapter |
| HTTP Server | Go `net/http` | 本機 HTTP API 與 Web UI 靜態資源 |
| Database | SQLite | 單一 `fileindex.db` 保存索引 |
| Frontend | HTML／CSS／JavaScript 輕量前端 | `web/index.html` 提供表格、表單、對話框、狀態與導覽 |
| 通訊 | HTTP + JSON | Browser 與 Backend 交換資料 |
| 長時間工作 | Go Job Manager + HTTP polling | Scan、CSV Export |
| UI 封裝 | Go embed | 將建置後 HTML／CSS／JavaScript 嵌入執行檔 |
| 設定 | JSON `config.json` | Portable 設定 |
| CSV | UTF-8 + BOM | 改善 Excel 中文相容性 |
| 時間 | UTC ISO 8601 | DB／API 統一時間格式 |
| 發布 | Windows EXE、macOS APP、Linux 執行檔與 Desktop Entry | 各平台圖示與可攜式發行包；執行程式不需 Go |

`//go:embed web` 在 Go 編譯時嵌入 `web/index.html` 等介面資源，發布後由系統瀏覽器顯示，不需要另外攜帶 `web/`。修改網頁後須重新編譯；目前不需要 TypeScript 建置步驟、Electron 或額外 Application Server。

目前 SQLite Driver 為 `modernc.org/sqlite`，版本依 `go.mod`；發行腳本採 `CGO_ENABLED=0` 交叉編譯。

### 3.2 平台矩陣

| 作業系統 | 架構 |
|---|---|
| Windows 10／11 | x64 |
| Linux | x64 |
| macOS | ARM64 |
| macOS | x64 |

各平台共用原始碼、Web UI、Application Logic、SQLite Schema、`fileindex.db` 與 `config.json`；發布各自的原生執行檔。跨 OS 後來源掛載位置可能不同，透過 Relocate 更新。

此矩陣為發行目標，實際最低 OS 需求亦受所用 Go 工具鏈限制，不表示所有系統版本均已完成實機驗證。建置與圖示封裝規則見第 4.4～4.6 節。

## 4. Portable 與系統架構

### 4.1 發布目錄

```text
FileIndex/
├── FileIndex.exe、FileIndex 或 FileIndex.app
├── data/
│   └── fileindex.db
├── config/
│   └── config.json
├── backup/
├── export/
└── LICENSE
```

所有 Application Path 以 Portable Root 為基準保存相對路徑，例如 `data/fileindex.db`；不得把安裝磁碟機代號寫死在設定。啟動時先解析 Portable Root，再解析設定路徑。

各平台分開發行，並非將四個執行檔放入不同子目錄後自動共用上一層資料。Windows／Linux 與 macOS 單獨執行檔以執行檔所在目錄為 Portable Root；macOS `FileIndex.app/Contents/MacOS/FileIndex` 則以 `.app` 所在目錄為 Root，資料不寫入 bundle。`go run .` 使用含 `go.mod` 的專案目錄。

可由捷徑、BAT、Shell Script、Finder 或應用程式選單啟動，Root 不依啟動時的工作目錄改變。資料目錄需可寫入；搬移既有資料時，將程式（macOS 為完整 `.app`）與 `config/`、`data/`、所需備份及匯出目錄一起攜帶。

此規則適用於程式資料、備份、匯出等 Application Path；來源 Storage 的 `last_scan_root`／`current_root` 則是所屬 OS 的來源根目錄路徑，兩者不可混淆。

### 4.2 元件關係

```text
系統瀏覽器：Embedded HTML／CSS／JavaScript
                       │ HTTP + JSON
                       ▼
             Go net/http（Loopback）
                       │
       ┌───────────────┼────────────────┐
       │               │                │
    Search       Scanner／Jobs       Storage
       │               │                │
       └───────────────┼────────────────┘
                       │
                  SQLite DB
                       │
             Export／Backup／Restore

Backend OS Adapter → 路徑驗證、檔案管理員
                   （原生選擇對話框、Filesystem identity 比對待整合）
```

Browser 不直接存取 SQLite、不自行組合 Open Folder 的本機絕對路徑、不執行 OS Command。上述責任均屬 Backend。

### 4.3 啟動與結束

1. 解析 Portable Root。
2. 讀取 `config/config.json`。
3. 開啟 SQLite，檢查 `PRAGMA user_version`。
4. 啟動 Loopback HTTP Server，建立隨機 Session Token。
5. 依設定開啟預設瀏覽器，顯示 Search 首頁。

`http_port = 0` 表示由 OS 自動配置可用 Port。預設服務位址為 `127.0.0.1`；如提供 IPv6，只能使用 Loopback `::1`，不得預設監聽 `0.0.0.0` 或公開網路介面。

UI 提供 Exit File Index。Scan 執行中一般結束要求回 `409 SCAN_IN_PROGRESS`；使用者確認後送 `force: true`，Backend 取消 Scan、回滾／丟棄 staging、關閉 SQLite 與 HTTP Server，再結束程序。

停用自動開啟瀏覽器時，使用終端機顯示的一次性 Session URL。掃描進度、Cancel scan 與 Exit File Index 位於頂部導覽列 Settings 右側。

### 4.4 開發與建置

開發需 Go 1.22+，在含 `go.mod` 的專案根目錄執行：

```powershell
go mod tidy
go run .
```

Windows x64 可直接建置並啟動含圖示的執行檔：

```powershell
go build -o FileIndex.exe .
.\FileIndex.exe
```

成功的 `go build` 通常不輸出訊息。使用者執行編譯成品不需要 Go；開發用 `go run .` 仍需要 Go。BAT 可使用 `cd /d "%~dp0"` 與 `start "" "%~dp0FileIndex.exe"` 啟動同目錄成品。

### 4.5 跨平台發行與圖示

發行腳本需 Go 1.22+ 與 Python 3.9+；macOS／Linux 通常使用 `python3`。在專案根目錄執行：

```powershell
python scripts/build.py --target all
```

`--target` 亦可指定下表任一單一目標。同一命令可在 Windows、macOS、Linux 執行；目標平台環境變數只套用於子程序。輸出位於 `dist/`，不打包既有索引、設定或備份。

| 目標 | 發行檔案 | 圖示與啟動方式 |
|---|---|---|
| `windows-amd64` | `FileIndex-windows-amd64.zip` | EXE 內嵌 ICO，解壓縮後雙擊 `FileIndex.exe` |
| `darwin-arm64` | `FileIndex-darwin-arm64.tar.gz` | Apple Silicon 的 `FileIndex.app`，Finder 使用 bundle 內 ICNS |
| `darwin-amd64` | `FileIndex-darwin-amd64.tar.gz` | Intel Mac 的 `FileIndex.app`，Finder 使用 bundle 內 ICNS |
| `linux-amd64` | `FileIndex-linux-amd64.tar.gz` | `FileIndex`、`FileIndex.png` 與 `install-desktop.py` |

macOS／Linux TAR.GZ 保留執行權限。macOS 須在 Mac 解壓縮並保留完整 `.app`；目前未經 Developer ID 簽章或公證，首次啟動可能需在「系統設定 → 隱私權與安全性」允許執行。單獨 `go build -o FileIndex .` 產生的是命令列執行檔，不提供 Finder 應用程式圖示。

Linux 可直接執行 `./FileIndex`。如需圖示與應用程式選單入口，在解壓縮目錄執行 `python3 install-desktop.py`；僅設定啟動器需要 Python 3，不需管理員權限。腳本在程式旁建立 `FileIndex.desktop` 並安裝使用者選單入口。因圖示與程式使用絕對路徑，移動資料夾、變更隨身碟掛載路徑或換電腦後須重新執行；部分桌面環境需標記為「允許啟動」。

Linux 圖示適用於支援 Desktop Entry 的啟動器／選單，不保證原始 ELF 在檔案管理員有自訂圖示。macOS／Linux 不直接使用 Windows ICO。介面仍在瀏覽器中執行，不替換瀏覽器的 Dock／工作列圖示。

### 4.6 圖示來源與更新

- Windows 來源為 `image/FileIndex.ico`，已產生並納入版本控制的 `icon_windows_amd64.syso` 由一般 `go build` 自動連結；執行時無須另外攜帶 ICO 或 SYSO。
- macOS／Linux 使用相同圖案的高解析度 `image/FileIndex-logo.png`；macOS 的 `image/FileIndex.icns` 亦納入版本控制。
- 更換圖案時同時替換 ICO 與 PNG，再執行：

```powershell
python -m pip install Pillow
python scripts/build.py --refresh-icons --target all
```

一般建置不需 Pillow 或下載圖示工具；只有 `--refresh-icons` 用 Pillow 轉換 ICNS，並透過 Go 下載／執行固定版本 `github.com/akavel/rsrc@v0.10.2` 產生 Windows 資源。更新後須一併提交 SYSO 與 ICNS；替換來源圖片不會更新既有執行檔。檔案總管若快取舊圖示，可重新整理或將新版複製至其他資料夾確認。

## 5. Storage 模型與 Relocate

### 5.1 Storage 定義與辨識

Storage 可代表 HDD、SSD、USB 磁碟、隨身碟、已掛載的網路儲存空間或指定目錄。資料庫中的 `storages.id` 才是正式 Identity。

Volume Label、Filesystem Type、Filesystem ID／UUID／Serial、Capacity 均是 OS 提供的輔助辨識資訊，不保證跨 OS 一致，也不作為自動配對的唯一依據。是否視為相同 Storage 由使用者決定。不得在來源建立 `.fileindex-id`。

### 5.2 兩個 Root 的語意

| 欄位 | 定義 | 更新時機 |
|---|---|---|
| `last_scan_root` | 最後成功建立索引所使用的 Root | Scan／Rescan 成功提交 |
| `current_root` | 目前使用者指定的來源位置 | Scan／Rescan 成功提交或 Relocate 成功 |

```text
Windows 掃描成功：
last_scan_root = E:\
current_root   = E:\

移至 macOS 並 Relocate：
last_scan_root = E:\
current_root   = /Volumes/BACKUP_A
```

### 5.3 Relocate 流程

目前流程使用經驗證的路徑欄位；以下原生選擇器與 identity 差異確認為待整合設計，不應視為現有功能：

1. 選擇既有 Storage，指定目錄（原生目錄選擇對話框待整合）。
2. Backend 驗證路徑存在並取得 Filesystem 資訊。
3. 與上次掃描資料差異明顯時顯示警告，回 `409 STORAGE_INFORMATION_CHANGED`。
4. 使用者確認後可用 `confirm_mismatch: true` 繼續。
5. 只更新 `current_root`，重新計算可用性。

Relocate 不掃描、不更動 files、不更新 `last_scan_root`、`last_scan_at`、`file_count`、`total_size_bytes`，也不覆寫上次掃描的 Volume／Filesystem／Capacity 資訊。

### 5.4 管理操作

- Edit 僅允許修改 `name` 與 `description`。
- View Files 導向已套用該 Storage 篩選條件的 Search。
- Delete 在同一交易刪除 Storage 與其 files，僅刪除索引，不刪除來源檔案。
- Details 顯示名稱、備註、Filesystem 資訊、容量、兩個 Root、索引筆數、索引大小及最後掃描時間。
- `available` 是 Backend 即時計算的 Runtime State，不保存至 SQLite。V1 基本判斷為 `current_root` 是否存在；存在不代表已自動驗證實體媒體身分。

## 6. SQLite Schema V1

Schema 版本固定為 `PRAGMA user_version = 1`。正式持久化資料表為 `storages`、`files`、`settings`，不建立 `directories` Table。staging 是暫存工作資料，不是第四個正式業務資料模型。

### 6.1 storages

資料庫開啟並通過 Schema 驗證後，啟用 `PRAGMA auto_vacuum=FULL`。既有 NONE 模式資料庫先設定 FULL，再執行一次 `VACUUM` 完成轉換；INCREMENTAL 模式可直接切換，已是 FULL 則不重建。此規則亦適用還原後開啟的 DB，Schema 版本仍為 1。轉換失敗須回報錯誤，不可假裝已啟用。轉換需額外磁碟空間及時間，於開始接受請求前完成。

刪除 Storage 的交易連動刪除 files，提交時自動回收完整空白頁面，不影響其他索引或來源檔案。未釋出完整頁面時不保證檔案大小減少；FULL 回收不等同全面消除碎片。發行驗證須涵蓋舊 DB 轉換、刪除後實際檔案縮小、其餘索引保留、完整性及重新開啟後設定持續有效。

| Column | Type／Constraint | 說明 |
|---|---|---|
| `id` | INTEGER PRIMARY KEY | Storage Identity |
| `name` | TEXT NOT NULL | 使用者指定名稱 |
| `description` | TEXT NULL | 備註 |
| `volume_label` | TEXT NULL | 上次成功 Scan 的 Volume Label |
| `filesystem_type` | TEXT NULL | NTFS／exFAT／APFS／ext4 等 |
| `filesystem_id` | TEXT NULL | UUID／Serial 等輔助資訊 |
| `capacity_bytes` | INTEGER NULL | 上次 Scan 的媒體容量，單位 Byte |
| `last_scan_root` | TEXT NOT NULL | 最後成功 Scan 的 Root |
| `current_root` | TEXT NULL | 目前定位 Root |
| `file_count` | INTEGER NOT NULL DEFAULT 0 | 正式索引檔案筆數 |
| `total_size_bytes` | INTEGER NOT NULL DEFAULT 0 | 正式索引檔案大小總和 |
| `last_scan_at` | TEXT NULL | 最後成功 Scan，UTC ISO 8601 |
| `created_at` | TEXT NOT NULL | Storage 建立時間，UTC ISO 8601 |

`capacity_bytes` 是媒體容量，`total_size_bytes` 是被索引檔案大小總和，不可互換。

### 6.2 files

| Column | Type／Constraint | 說明 |
|---|---|---|
| `id` | INTEGER PRIMARY KEY | File ID |
| `storage_id` | INTEGER NOT NULL，FK → storages.id | 所屬 Storage |
| `name` | TEXT NOT NULL | 檔名，包含原始副檔名 |
| `extension` | TEXT NULL | 副檔名，不包含 `.` |
| `relative_path` | TEXT NOT NULL | 相對目錄，不包含檔名 |
| `size_bytes` | INTEGER NOT NULL | 檔案大小，Byte |
| `modified_at` | TEXT NULL | 修改時間，UTC ISO 8601 |

路徑契約：

```text
relative_path = Project/ABB/REF615
name          = REF615_Manual.pdf
```

- DB 的邏輯目錄分隔符統一 `/`。
- Root 直屬檔案的 `relative_path` 為空字串 `""`。
- 不另建 Directory Record，因此不以獨立資料列索引空目錄。
- files 不保存 `scan_time`；透過 Storage 的 `last_scan_at` 取得，避免每筆檔案重複保存相同時間。
- 刪除 Storage 必須同步刪除其 files，可用交易明確刪除或 FK Cascade；不得留下孤兒資料。

### 6.3 settings

基線規格確認保留 `settings` Table，但可取得內容未列出欄位、主鍵與用途。故本文件不擅自指定 key/value 結構；正式欄位及其與 `config.json` 的分工列為實作前待釐清項目。已明確列出的應用程式設定仍由 `config.json` 保存。

### 6.4 索引與查詢策略

| 索引 | 欄位 | 狀態 |
|---|---|---|
| `idx_files_storage` | `files(storage_id)` | V1 至少建立 |
| `idx_files_extension` | `files(extension)` | V1 至少建立 |
| `idx_files_name` | `files(name)` | 可選 |

V1 採一般 substring search。不得假設一般 B-tree 能有效加速前置萬用字元的 `LIKE '%keyword%'`。是否導入 FTS5／trigram 需實測後再決定。

### 6.5 時間與顯示

DB／API 使用 UTC ISO 8601，例如 `2026-09-30T03:15:27Z`。Frontend 依 Browser Local Time 顯示；API 保留原始 Byte 數值，UI 再格式化人類可讀大小。未知時間以 NULL 表示，不捏造時間。

## 7. Search

### 7.1 比對規則

搜尋範圍是 `name` 或 `relative_path`，支援 Partial Match。Backend 負責拆解關鍵字；每一個關鍵字必須出現在檔名或目錄任一欄位。

```text
輸入：ABB REF615 manual

(name 或 relative_path 包含 ABB)
AND (name 或 relative_path 包含 REF615)
AND (name 或 relative_path 包含 manual)
```

V1 不提供 OR、NOT、Regex 或複雜布林語法。

### 7.2 篩選、排序與分頁

| 項目 | 行為 |
|---|---|
| Storage Filter | 指定 Storage |
| Extension Filter | 指定副檔名 |
| 排序 | Filename／Relative Path 四段切換；Storage、Size、Modified 升降冪，由 Backend 執行 |
| 預設排序 | Filename ASC |
| 分頁 | Backend 分頁；預設每頁 100 筆 |
| Extension 選項 | 由獨立 API 統計，可限制 Storage，不從當頁結果推導 |

### 7.3 結果與 Details

結果列顯示 Filename／Relative Path、Storage、Size、Modified。點擊整筆結果開啟 File Details，顯示 Filename、Storage、Relative Path、Size、Modified、Storage Last Scan 及 Source 狀態。

Filename／Relative Path 表頭依序切換「檔名升冪 → 路徑加檔名升冪 → 檔名降冪 → 路徑加檔名降冪」，以 `F`／`P+F` 與箭頭表示模式。Storage、Size、Modified 首次點擊升冪，再次點擊切換降冪。大小自動使用 B、KB、MB、GB、TB、PB；窄畫面可將數字與單位分行。日期時間依 Settings → Date and time format 顯示，必要時日期與時間分行；不改變 DB／API 的 UTC 格式。

來源離線不妨礙搜尋與 Details 的索引資料顯示。這些資料代表最後成功 Scan 時的快照，不保證來源目前仍相同。

## 8. File Details 與 Open Folder

File Details 開啟時，Backend 依 `current_root` 計算來源可用性。可用時提供 Open Folder；未定位／離線時提供狀態說明與 Relocate Storage 入口。

Open Folder 開啟所在目錄，支援時選取索引檔案，但不啟動檔案內容：

```text
file_id → DB 查得 Storage 與 relative_path
        → current_root + relative_path
        → 驗證目錄
        → 系統 File Manager
```

| 平台 | 開啟工具 |
|---|---|
| Windows | File Explorer，顯示並選取檔案 |
| macOS | Finder，顯示並選取檔案 |
| Linux | 支援 `org.freedesktop.FileManager1` 的桌面可顯示並選取檔案 |

Browser 只傳 File ID；Backend 負責路徑解析與 OS 操作。來源不可用回 `STORAGE_OFFLINE`；Root 存在但索引目錄已不存在回 `DIRECTORY_NOT_FOUND`。即使 Details 先前顯示 Available，Open Folder 執行時仍須重新檢查。

## 9. Scan／Rescan 與 staging 安全機制

### 9.1 工作模型

使用者手動指定 Root 後遞迴掃描 Metadata。同一時間只允許一個 Scan Job；長時間工作由 Go Job Manager 執行，HTTP 不等待整個 Scan 完成。

UI 以 500 ms～1 秒間隔 polling，顯示 Current Directory、Files Scanned、Directories Scanned、Indexed Size、Errors、Elapsed Time。未知總量時不顯示虛假的完成百分比。

### 9.2 首次 Scan

1. 選目錄並 Inspect Storage。
2. 輸入 Storage 名稱與備註。
3. 建立 `mode: new` Job，將掃描結果寫入 staging。
4. 成功完成後，在交易中建立正式 Storage 與 files，保存統計、Filesystem 資訊、成功時間與兩個 Root。
5. 失敗或取消時丟棄 staging；首次成功前不建立正式 Storage Record。

### 9.3 Rescan

Rescan 直接使用 `current_root`；僅當此欄位未設定時才使用 `last_scan_root`。不顯示 Root path 表單，也不採用請求另傳的路徑；變更位置須先使用 Relocate。來源不可用時回 `422 STORAGE_OFFLINE`，提示重新連接或 Relocate，不建立掃描工作，保留原索引。介面名稱改為 Relocate，既有 `/storages/{id}/locate` API 路徑維持相容。

1. 點選既有 Storage 的 Rescan，由 Backend 取得已保存的來源目錄。
2. 驗證輸入路徑；Filesystem identity 比對及差異確認仍待平台整合。
3. 保留舊索引，將新結果寫入獨立 staging。
4. 掃描成功後，在單一交易中替換該 Storage 的 files，更新統計、Filesystem 資訊、`last_scan_at`、`last_scan_root`、`current_root`。
5. 交易失敗則回滾；取消或掃描失敗則丟棄 staging，保留舊索引及其掃描資訊。

```text
舊正式索引 ────────────────────────────┐
                                      │
新 Scan → staging → 失敗／取消 → 丟棄 → 保留舊索引
                  └→ 成功 → 交易替換 → 新正式索引
```

不得先刪除舊索引再開始 Scan，也不得逐批將未完成的新結果混入正式索引。

### 9.4 Job 狀態與取消

| 狀態 | 意義 |
|---|---|
| `running` | 正在掃描 |
| `cancelling` | 已接受取消，仍在停止與清理 |
| `cancelled` | 已取消且完成清理 |
| `completed` | 掃描成功並完成正式提交 |
| `failed` | 掃描或提交失敗 |

取消 API 回 `cancelling` 不代表 Scanner 已停止；UI 繼續 polling，直到終止狀態。取消不能破壞正式索引。

### 9.5 Warnings 與成功條件的界線

詳細 API 原文允許 `completed` 同時有非零 `error_count`，並舉 `PERMISSION_DENIED`、`FILE_DISAPPEARED` 作為 Warning；基線則要求「完整成功後才取代舊索引」。兩者尚未明確界定哪些錯誤可跳過、哪些必須阻止提交。

因此已確認的安全底線是：失敗／取消不可覆蓋舊索引，只有成功終態才能提交；不可將嚴重或未分類錯誤悄悄視為成功。可容許 Warning 的精確條件需在實作前釐清，見第 18 節。

Warning API 回傳類型與路徑；記錄可保存在 Job Memory／Temporary State，程式關閉後不必保留。

## 10. CSV 匯出

支援 Search Result Export 與 Storage Index Export。Search Export 套用目前搜尋、篩選與排序，匯出全部符合條件的資料，不受目前分頁限制。

| CSV 欄位 | 資料來源／語意 |
|---|---|
| Storage Name | `storages.name` |
| Filename | `files.name` |
| Extension | `files.extension` |
| Relative Path | `files.relative_path`，只有目錄 |
| File Size | `files.size_bytes`，原始 Byte |
| Modified Time | `files.modified_at` |
| Scan Time | JOIN `storages.last_scan_at` |

編碼為 UTF-8 + BOM。時間沿用 UTC ISO 8601 基線。輸出保存至設定的 `export_path`；範例為 `export/FileIndex_Search_20260930_123500.csv`。

Export 使用 Job Model，開始後取得 `job_id`；完成結果包含 `filename`、Portable 相對 `relative_path`、`record_count`。CSV 欄位中的逗號、引號與換行必須按 CSV 規則保留；試算表公式型文字的輸出政策尚待定義。

## 11. Backup／Restore

### 11.1 Backup

- 預設 DB：`data/fileindex.db`。
- 預設備份：`backup/FileIndex_YYYYMMDD_HHMMSS.db`。
- 使用 SQLite 安全 Backup 機制產生一致快照；不得以任意複製開啟中的主 DB 檔取代一致性備份。
- UI 提供列表、建立、刪除與還原。
- Backup ID 由 Backend 安全識別，不讓 Browser 用任意路徑要求刪檔。
- 目前定案備份對象是 SQLite DB；未定義將 `config.json` 打包成同一備份。

### 11.2 Restore

Restore 是完整取代目前 DB，不是 Merge。UI 用語固定為 **Restore from File**，避免稱作 Import Database。

```text
選取備份或 .db 檔
  → 驗證 SQLite／Schema／PRAGMA user_version
  → 自動備份目前 DB
  → 進入維護狀態並關閉目前 DB
  → 還原
  → 重新開啟
  → 再次驗證
  → 恢復服務
```

維護期間其他 DB API 回 `503 DATABASE_MAINTENANCE`。目前從外部檔案還原使用經驗證的路徑欄位；原生 `.db` File Picker 待整合。副檔名不能取代 SQLite 與 Schema 驗證。V1 對應版本為 1，不應將未知版本直接當作相容。

自動備份失敗、替換失敗及重新開啟失敗的詳細復原流程，原對話未完整指定，列為實作前必要細節。

## 12. config.json

設定檔位置：`config/config.json`。

```json
{
  "database_path": "data/fileindex.db",
  "backup_path": "backup",
  "export_path": "export",
  "search_page_size": 100,
  "http_port": 0,
  "auto_open_browser": true,
  "time_format": "locale"
}
```

| Key | 定義 |
|---|---|
| `database_path` | SQLite 檔案位置，Portable Root 相對路徑 |
| `backup_path` | 備份目錄，Portable Root 相對路徑 |
| `export_path` | CSV 匯出目錄，Portable Root 相對路徑 |
| `search_page_size` | 搜尋每頁筆數，預設 100 |
| `http_port` | HTTP Port，0 表示自動配置 |
| `auto_open_browser` | 啟動時是否開啟預設瀏覽器 |
| `time_format` | UI 日期時間格式，預設 `locale`；亦支援 `yyyy-mm-dd-24h`、`yyyy-slash-24h`、`us-12h`、`eu-24h` |

Settings API 讀取與保存上述設定。設定變更哪些即時生效、哪些需重新啟動，以及切換 DB 路徑是否包含資料搬移，未在原對話定案，不可自行等同為自動搬移 DB。

## 13. UI Wireframe 與狀態

主要導覽為 Search、Storage、Backup、Settings；Search 為首頁。掃描進度、Cancel scan 與 Exit File Index 放在頂部 Settings 右側；以下框線僅為功能示意。

### 13.1 Search 首頁

```text
┌────────────────────────────────────────────────────────────┐
│ File Index   Search | Storage | Backup | Settings             │
│                       掃描進度 [Cancel scan] [Exit File Index] │
├────────────────────────────────────────────────────────────┤
│ 搜尋 [ ABB REF615 manual                         ] [Search] │
│ Storage [全部 ▼]   Extension [全部 ▼]         [Export CSV] │
│                                                            │
│ Filename / Relative Path        Storage       Size Modified│
│ REF615_Manual.pdf               工程備份 A    ...  ...     │
│ Project/ABB/REF615                                         │
│                                                            │
│ 共 126 筆                  [上一頁] 1 / 2 [下一頁]          │
├────────────────────────────────────────────────────────────┤
│ ● Ready                                                    │
└────────────────────────────────────────────────────────────┘
```

結果表頭支援 Backend 排序。無符合結果、載入中、API 錯誤與 DB 維護中必須能區分，不可把查詢失敗顯示成零筆結果。

### 13.2 File Details

```text
┌ File Details ──────────────────────────────────────── × ┐
│ Filename          REF615_Manual.pdf                    │
│ Storage           工程備份硬碟 A                       │
│ Relative Path     Project/ABB/REF615                    │
│ Size              18.3 MB                              │
│ Modified          2026/08/12 15:32                      │
│ Storage Last Scan 2026/09/30 10:30                      │
│ Source            ● Available                         │
│                                [Open Folder] [Close]   │
└────────────────────────────────────────────────────────┘
```

離線／未定位時顯示 `Offline / Not located` 與 Relocate Storage 入口，保留全部索引資訊；不可提供看似可執行的 Open Folder。

### 13.3 Storage 與工作流程

```text
Storage                                              [Add]
──────────────────────────────────────────────────────────
工程備份硬碟 A        ● Available / ○ Offline
Filesystem / Capacity / Files / Indexed Size / Last Scan
[View Files] [Details] [Edit] [Rescan] [Relocate]
[Export CSV] [Delete]

Add → Select Directory → Inspect → Name / Description → Scan
Rescan → 讀取已保存 Root → 驗證路徑 → Scan
Relocate → 指定 Current Root → 驗證路徑 → 更新位置
```

原生路徑選擇器及 Filesystem identity 差異警告為後續整合項目。

### 13.4 Scan Progress

```text
Scanning：工程備份硬碟 A
Current Directory：Project/ABB/Protection
Files Scanned：328,512       Directories Scanned：18,732
Indexed Size：...            Errors：3
Elapsed Time：00:08:42
                                       [View Warnings] [Cancel]
```

`cancelling` 顯示取消處理中；`cancelled` 與 `failed` 說明原索引保留；`completed` 顯示完成統計並刷新 Storage／Search。不以未知總量產生百分比。

### 13.5 Backup／Settings

```text
Backup                           [Create Backup] [Restore from File]
Filename                         Size   Created   Schema   Actions
FileIndex_20260930_123500.db      ...    ...       1        Restore / Delete

Settings
Database Path       [data/fileindex.db]
Backup Path         [backup]
Export Path         [export]
Search Page Size    [100]
HTTP Port           [0]
Auto Open Browser   [✓]
Date and time format [locale ▼]
                                                    [Save]
```

### 13.6 關鍵狀態對照

| 狀態 | UI 行為 |
|---|---|
| Ready | 可正常操作 |
| Offline／Not located | 可查索引；提供 Relocate |
| Root 存在但目錄不存在 | 顯示 Directory Not Found |
| Filesystem 資訊不一致（待整合） | 設計為顯示 previous／current 比較，讓使用者決定 |
| Scan 進行中 | 顯示統計；第二個 Scan 回衝突 |
| Cancelling | 持續 polling，等待清理完成 |
| Database Maintenance | 說明還原中，DB 功能暫不可用 |
| Failed | 顯示錯誤，不冒稱完成或空結果 |

## 14. Backend API 契約

Base URL：`http://127.0.0.1:<port>/api/v1`。UI 位於 `/`。以下路徑皆省略 `/api/v1` 前綴。

### 14.1 回應格式

單筆與集合使用 `data`；分頁查詢另有 `meta`：

```json
{
  "data": [],
  "meta": {
    "total": 126,
    "page": 1,
    "page_size": 100,
    "total_pages": 2,
    "sort": "name",
    "order": "asc"
  }
}
```

錯誤使用穩定 `error.code`；UI 不以人類可讀 `message` 字串作邏輯判斷：

```json
{
  "error": {
    "code": "STORAGE_INFORMATION_CHANGED",
    "message": "Filesystem information differs from the previous scan.",
    "details": {
      "previous": {},
      "current": {}
    }
  }
}
```

詳細流程採 `STORAGE_INFORMATION_CHANGED`，取代早期通用範例的 `STORAGE_MISMATCH`，避免同一情境兩種名稱。

| HTTP Status | 用途 |
|---|---|
| 200 | 成功 |
| 201 | 已建立，例如 Scan Job |
| 400 | 請求格式或參數錯誤 |
| 404 | 資源不存在 |
| 409 | 狀態衝突、需要明確確認 |
| 422 | 請求有效但內容無法執行 |
| 500 | 內部錯誤 |
| 503 | DB／服務不可用或維護中 |

### 14.2 完整端點表

| Method | Path | 功能／契約 |
|---|---|---|
| GET | `/system/status` | version、database_ready、schema_version、active_scan |
| POST | `/system/select-directory` | 原生目錄選擇為待整合契約；目前使用路徑欄位 |
| POST | `/system/select-database-file` | 原生 `.db` 選擇為待整合契約；目前使用路徑欄位 |
| POST | `/system/inspect-storage` | 收 path；回 Filesystem 輔助資訊 |
| POST | `/system/shutdown` | 結束程序；可傳 force |
| GET | `/files/search` | 搜尋、篩選、排序、分頁 |
| GET | `/files/extensions` | extension／count 集合；可傳 storage_id |
| GET | `/files/{id}` | File Details 與 storage.available |
| POST | `/files/{id}/open-folder` | 依 File ID 開目錄；回 opened |
| GET | `/storages` | Storage 列表與即時 available |
| GET | `/storages/{id}` | Storage 完整資訊與兩個 Root |
| PATCH | `/storages/{id}` | 僅修改 name、description |
| DELETE | `/storages/{id}` | 交易刪除 Storage 與 files 索引 |
| POST | `/storages/{id}/locate` | 收 path、可選 confirm_mismatch；只改 current_root |
| POST | `/scans` | mode=new／rescan，建立 Scan Job |
| GET | `/scans/{job_id}` | Scan 進度與終態 |
| POST | `/scans/{job_id}/cancel` | 提出取消要求 |
| GET | `/scans/{job_id}/errors` | Warning type／path 集合 |
| POST | `/exports` | 搜尋結果或 Storage CSV Export Job |
| GET | `/exports/{job_id}` | 匯出狀態與完成檔案資訊 |
| GET | `/backups` | 備份列表 |
| POST | `/backups` | 建立安全備份 |
| DELETE | `/backups/{id}` | 刪除指定備份 |
| POST | `/backups/{id}/restore` | 還原已知備份 |
| POST | `/database/restore` | 從經 Backend 驗證的 path 還原；原生 Picker 待整合 |
| GET | `/settings` | 讀設定 |
| PUT | `/settings` | 保存設定至 config.json |

### 14.3 Search

查詢參數為 `q`、`storage_id`、`extension`、`page`、`page_size`、`sort`、`order`。

```text
GET /api/v1/files/search?q=ABB%20REF615&storage_id=3&extension=pdf&page=1&page_size=100&sort=name&order=asc
```

每筆結果包含 `id`、`storage_id`、`storage_name`、`name`、`extension`、`relative_path`、`size_bytes`、`modified_at`，加上 14.1 的分頁 meta。`sort` 支援 `name`、`path_name`、`storage`、`size`、`modified`；`order` 為 `asc`／`desc`。四段式 Filename／Relative Path 分別對應 `name/asc`、`path_name/asc`、`name/desc`、`path_name/desc`。

### 14.4 File Details

```json
{
  "data": {
    "id": 98321,
    "name": "REF615_Application_Manual.pdf",
    "extension": "pdf",
    "relative_path": "Project/ABB/Protection/REF615",
    "size_bytes": 19188941,
    "modified_at": "2026-08-12T07:32:15Z",
    "storage": {
      "id": 3,
      "name": "工程備份硬碟 A",
      "last_scan_at": "2026-09-30T02:30:00Z",
      "available": true
    }
  }
}
```

File Details 無須接收完整本機絕對路徑；Storage Details 因顯示需求可回兩個 Root。Open Folder 成功回 `{"data":{"opened":true}}`。

### 14.5 Scan 請求

首次新增：

```json
{
  "mode": "new",
  "storage": {
    "name": "工程備份硬碟 A",
    "description": "2024–2026 工程資料"
  },
  "scan_root": "E:\\"
}
```

Rescan：

```json
{
  "mode": "rescan",
  "storage_id": 3
}
```

新增 Storage 請求中的 `scan_root` 表示「這次要掃描的輸入路徑」，可保留此名稱；它不是被移除的 storages 資料欄位。成功提交後才寫入兩個正式 Root 欄位。

建立 Job 回 `201`，data 包含 `job_id`、`status: running`。進度回傳：

```json
{
  "data": {
    "job_id": "scan-20260930-001",
    "status": "running",
    "storage_name": "工程備份硬碟 A",
    "current_path": "Project/ABB/Protection",
    "files_scanned": 328512,
    "directories_scanned": 18732,
    "size_bytes": 2873152738816,
    "error_count": 3,
    "elapsed_seconds": 522
  }
}
```

完成後回 `status: completed` 與 `storage_id`、最終統計；取消要求回 `cancelling`。

### 14.6 Relocate

```json
{
  "path": "/Volumes/BACKUP_A",
  "confirm_mismatch": false
}
```

成功回應：

```json
{
  "data": {
    "storage_id": 3,
    "current_root": "/Volumes/BACKUP_A",
    "available": true
  }
}
```

此處已修正早期範例的 `scan_root` 回應欄位。

### 14.7 Export

搜尋結果：

```json
{
  "type": "search",
  "query": {
    "q": "ABB REF615",
    "storage_id": 3,
    "extension": "pdf",
    "sort": "name",
    "order": "asc"
  }
}
```

單一 Storage：

```json
{
  "type": "storage",
  "storage_id": 3
}
```

開始回 `job_id` 與 `running`；完成回 `completed`、`filename`、`relative_path`、`record_count`。搜尋匯出不傳分頁限制。

### 14.8 Backup、Restore、Settings、Shutdown

- 建立備份回 `filename`、`relative_path`、`size_bytes`、`created_at`、`schema_version`。
- 從外部 DB 還原請求為 `{"path":"/Users/user/Desktop/fileindex.db"}`；Backend 必須先驗證。
- GET Settings 的 data 與 PUT Settings 的 request body 皆採第 12 節設定物件。
- 確認中止 Scan 並離開時，Shutdown request body 為 `{"force":true}`。

## 15. 安全設計

### 15.1 已定案的本機 API 邊界

- 只監聽 Loopback，不對外提供網路服務。
- 每次啟動產生隨機 Session Token；API 透過 `X-FileIndex-Token` 等明確 Header 驗證。
- 限制允許的 Origin；不得使用 `Access-Control-Allow-Origin: *`。
- 初次瀏覽器啟動可透過帶 Token 的本機 URL 建立 Session；這是本機程序授權，不是帳號登入系統。
- 前端不得直接操作 DB 或 OS Command。
- Open Folder 只接受 File ID，由 Backend 解析路徑。
- 刪除備份只接受安全 Backup ID，不接受任意刪除路徑。
- Inspect、Scan、Relocate、Restore 因用途需要可提交來源 path，但均由 Backend 驗證，不能因此泛化為任意 OS 執行介面。

### 15.2 安全實作補充（非原對話逐項定案）

為使上述邊界可落實，下列事項需納入實作審查；不增加 V1 功能範圍：

- SQL 使用參數綁定，排序欄位使用白名單。
- 檔名、目錄、備註與 Warning 視為不可信文字，UI 輸出須避免 HTML 注入。
- Open Folder 驗證解析結果位於指定 Root 範圍內，處理 `..`、絕對路徑與符號連結，避免目錄逃逸。
- OS 呼叫以固定程式及獨立參數傳遞，不將來源文字拼接成 Shell 指令。
- 驗證應用程式相對路徑不逃出 Portable Root；驗證輸入大小、頁碼、Port 等範圍。
- Token 不寫入一般日誌；初始 URL Token 的移除、保存與 Host／Origin 驗證需形成明確契約。
- Restore 選入的 DB 不因來自本機就視為可信，須驗證 Schema 與資料完整性。

## 16. 效能與發行驗證範圍

原對話以百萬筆以上 Metadata 作為需評估的情境，但未承諾搜尋延遲、掃描速度或記憶體上限。應持續以目前 SQLite Driver 評估批次寫入、substring search、分頁、排序、CSV Export 與 Rescan 原子替換。

Go 的 goroutine／channel 用於維持 HTTP UI 與長時間工作的回應能力；不代表對 HDD 啟用大量平行 I/O。平行度與批次大小待量測決定。

各平台發布需驗證檔案管理員開啟與選取、Portable 相對路徑、共用 DB、圖示與封裝啟動行為。Windows 檢查 EXE 圖示；macOS 檢查 Finder 圖示、bundle 啟動及資料寫至 bundle 旁；Linux 檢查 Desktop Entry、特殊字元路徑與搬移後重新安裝啟動器。原生 Picker 與 Filesystem identity 比對列為後續整合驗證。成功交叉編譯或檢查封裝結構不等於已在目標 OS 完成實機驗證；實際最低 OS 需求依 Go 工具鏈與桌面環境確認。

## 17. 驗收檢核

以下是從已定案行為整理的驗收項目，不表示本次已實作或執行測試。

| 編號 | 情境 | 預期結果 |
|---|---|---|
| A01 | 首次 Scan 成功 | 才建立正式 Storage、files 與統計 |
| A02 | 首次 Scan 取消／失敗 | 不留下正式 Storage 與部分索引 |
| A03 | Rescan 取消／失敗 | 舊 files、統計、Scan 時間及歷史 Root 保留 |
| A04 | Rescan 成功 | 單一交易替換索引與掃描資訊 |
| A05 | Scan 進行中再要求 Scan | 回狀態衝突，不建立第二個工作 |
| A06 | 拔除 Storage 後搜尋 | 仍可取得索引與 File Details |
| A07 | Windows 掃描後移至 macOS Relocate | 只改 current_root，不重新掃描 |
| A08 | Relocate 前後比對資料 | files、last_scan_at、統計與 Filesystem 歷史資訊不變 |
| A09 | 多關鍵字分散在檔名及目錄 | 每個詞都符合時才返回 |
| A10 | 結果有多頁後匯出 | CSV 含全部符合結果 |
| A11 | Root 可用且目錄存在 | Open Folder 開目錄，不開檔案 |
| A12 | Root 離線／子目錄消失 | 分別顯示 STORAGE_OFFLINE／DIRECTORY_NOT_FOUND |
| A13 | Restore 期間查 DB | 回 DATABASE_MAINTENANCE |
| A14 | Portable 目錄換位置 | Application 相對路徑仍可解析 |
| A15 | API Token 無效或 Origin 不允許 | 不執行受保護操作 |
| A16 | Delete Storage | 僅刪除 DB 索引，來源檔案保留 |
| A17 | CSV 含中文 | 以 UTF-8 + BOM 輸出，文字可正常讀取 |
| A18 | Schema 檢查 | 正式 V1 DB 的 user_version 為 1 |
| A19 | 重複點擊 Filename／Relative Path | 四段排序依序切換，F／P+F 與箭頭正確 |
| A20 | 修改日期格式或縮窄畫面 | 日期格式依設定；大小及日期時間可適當分行 |
| A21 | Windows x64 一般建置 | EXE 內嵌圖示，執行時不依賴外部 ICO／SYSO |
| A22 | 建置全部發行包 | 產生四個目標，ZIP／TAR.GZ 格式正確，不含既有使用者資料 |
| A23 | macOS APP 啟動及搬移 | 保留完整 bundle；圖示可見，資料保存在 APP 旁 |
| A24 | Linux 桌面啟動器 | 使用者選單有圖示；位置改變後重建入口可啟動 |
| A25 | 更新圖案並重新封裝 | SYSO／ICNS 與發行包更新；普通建置不需 Pillow |
| A26 | 從不同工作目錄啟動 | Portable Root 維持執行檔或 APP 所在目錄 |

## 18. 待整合項目與仍需確認的細節

README 明確列出的待整合項目為原生目錄／資料庫檔案選擇器及 Filesystem identity 比對，目前以經驗證的路徑欄位提供工作流程。以下保留舊版規格中 README 尚未涵蓋的詳細契約待核對項目；不表示程式完全沒有相關實作，亦不新增需求：

1. **settings Table**：欄位、主鍵、資料用途及與 JSON 設定的分工。
2. **Scan 成功與 Warning**：可跳過的錯誤、不可提交的錯誤、部分目錄無權限時的處理；解決「完整成功」與 completed 含 Warning 的界線。
3. **掃描規則**：符號連結、junction、隱藏／系統檔、排除規則、特殊檔案、跨掛載點、非法或無法轉換的檔名。
4. **Search 細節**：大小寫與 Unicode 比對、空白詞拆分、空查詢、`%`／`_` literal 語意、穩定次排序、NULL 排序、頁數上限；排序模式與 enum 已同步至第 7、14 節。
5. **檔案欄位正規化**：副檔名大小寫、無副檔名的 NULL／空字串政策、同一路徑唯一性與 Rescan 後 File ID 穩定性。
6. **staging 實作與互斥**：暫存 DB 或其他形式、崩潰後清理、Scan 與 Delete／Relocate／Restore／Export 等同時操作的規則。
7. **Restore 失敗復原**：自動備份失敗時的阻擋、替換方式、重新開啟失敗時回復、版本不相容處理；不得自行假設已有 Migration。
8. **CSV 完整契約**：換行格式、NULL 表示、公式型文字處理、同名檔案處理與下載／開啟匯出檔的 UI 交付方式。
9. **Settings 生效時機**：即時／重啟項目、驗證範圍、設定損壞處理、DB 路徑變更語意。
10. **Job／API 邊界**：Job 留存期限、Export 失敗狀態、原生 Picker 取消回應、Token 拒絕狀態碼、各端點未列明的錯誤對應。
11. **平台整合及驗證**：Native Dialog 方案、Filesystem identity 比對、最低 OS／Linux 桌面環境與實測效能門檻。SQLite Driver 與目前圖示工具、發行格式已記錄於第 3、4 節。

本文件依目前 README 同步使用者可見行為與發行規格；未涵蓋的詳細設計保留供後續核對。本次修訂僅更新規格文件，不代表新增功能或新增平台實機驗證結果。
