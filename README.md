# File Index System

[English](README.Eng.md) | **繁體中文**

<p align="center"><img src="image/FileIndex-logo.png" alt="File Index System FI 標誌" width="128"></p>

## 專案起源

本專案起源是 **CD Index 光碟索引大師**，這套由蔡明修所撰寫的免費軟體。我使用多年，覺得它是一個很實用的工具程式；但在目前的系統上，不知為何開啟後若閒置太久便會出現錯誤。加上現在 AI 工具盛行，於是請 AI 協助訂定規格並進行開發。

**CD Index 光碟索引大師**的說明可參考[布丁布丁吃什麼？— CD Index 光碟索引大師](https://blog.pulipuli.info/2016/05/cd-index-cd-index-download.html)。

File Index System 是依照 V1 規格開發的可攜式離線檔案中繼資料索引工具。它會將指定目錄中的檔名、相對路徑、大小及修改時間保存至 SQLite；來源磁碟離線後，仍可搜尋索引並確認檔案原本存放的位置。

## 功能

- 檔名與相對路徑搜尋、多關鍵字 AND、Storage／副檔名篩選、四段式檔名／路徑排序及分頁
- Storage 新增、編輯、定位、重新掃描、匯出與刪除
- 使用 staging 與交易保護正式索引的掃描流程，以及取消掃描
- UTF-8 BOM CSV 非同步匯出
- SQLite 安全備份、驗證及完整還原；還原前會自動備份現有資料庫
- Portable JSON 設定，包括搜尋頁面筆數、HTTP Port、自動開啟瀏覽器及日期時間格式
- 在 Windows、macOS，以及支援 `org.freedesktop.FileManager1` 的 Linux 桌面中顯示並選取索引檔案

## 畫面預覽

![File Index System 搜尋畫面](image/System-01.png)

![File Index System Storage 畫面](image/System-02.png)

## 介面操作

- 重複點選 **Filename / Relative Path**，會依序切換「只對檔名升冪」、「路徑加檔名升冪」、「只對檔名降冪」及「路徑加檔名降冪」。欄位上的 `F` 或 `P+F` 和箭頭會顯示目前模式。
- 點選 **Storage**、**Size** 或 **Modified** 會先以升冪排序；再次點選同一欄位可切換升冪與降冪。
- 檔案大小會自動使用 B、KB、MB、GB、TB 或 PB；畫面較窄時，數字與單位可能分行顯示。
- 日期時間依 **Settings → Date and time format** 的設定顯示；必要時日期與時間會分行顯示。
- 點選搜尋結果可查看詳細資料。按下 **Open Folder** 時，支援的平台會開啟所在資料夾並選取該檔案。
- 掃描進度、**Cancel scan** 與 **Exit File Index** 位於頂部導覽列的 **Settings** 右側。

**Rescan** 直接重新掃描 Storage 目前的來源目錄並更新索引，不需輸入 Root path。若已使用 **Relocate** 指定新位置，會掃描新位置；來源不可用時，請先重新連接或重新定位。**Relocate** 只更新來源位置，不掃描或修改既有檔案索引。

按下 **Exit File Index** 並成功送出結束要求後，狀態會顯示 `Stopping server…`，服務停止連線後改為 `Server stopped`，並停用操作控制。若正在掃描，會先詢問是否取消掃描並結束；取消結束操作時仍可繼續使用。只關閉瀏覽器分頁不會結束背景程式。

### 刪除 Storage 與資料庫空間

刪除 Storage 時，會連動刪除該筆檔案索引，並以 SQLite `auto_vacuum=FULL` 在交易提交時回收空白頁面，縮小 DB；不會刪除來源檔案。少量刪除若未釋出完整頁面，檔案大小可能不變。舊資料庫（含還原的資料庫）首次開啟時會自動轉換，可能需要較長時間及額外磁碟空間；之後不需每次重整整個 DB。

刪除只影響選取的 Storage 及其索引，其他 Storage 保留。既有備份不會同步刪除其中的索引；還原較早的備份可能讓已刪除的 Storage 再次出現。

## 開發模式

開發模式需要 Go 1.22 或更新版本，而且必須在包含 `go.mod` 的專案目錄執行：

```powershell
go mod tidy
go run .
```

`go run .` 會以專案目錄作為 Portable Root。一般使用者建議使用下方說明建置原生執行檔；建置後不需要安裝 Go，也不需要從專案目錄啟動。

程式只監聽 `127.0.0.1`。啟動後會依設定開啟預設瀏覽器；若停用自動開啟，請使用終端機顯示的一次性 Session URL。

## Windows

### 建置

在 PowerShell 中執行：

```powershell
go build -o FileIndex.exe .
```

`go build` 成功時通常不會顯示訊息；請確認專案目錄中已產生 `FileIndex.exe`。

建置後可直接雙擊 `FileIndex.exe`，或從 PowerShell 執行：

```powershell
.\FileIndex.exe
```

也可以在執行檔旁建立 `Start-FileIndex.bat`：

```bat
@echo off
cd /d "%~dp0"
start "" "%~dp0FileIndex.exe"
```

之後雙擊 BAT 即可啟動。

若要透過 BAT 執行開發版本，可使用：

```bat
@echo off
cd /d "%~dp0"
go run .
pause
```

這種方式仍需要安裝 Go，而且 BAT 必須放在專案根目錄。

### 執行檔圖示

已納入由 `image/FileIndex.ico` 產生的 `icon_windows_amd64.syso`，因此一般 `go build -o FileIndex.exe .` 就會把圖示嵌入 Windows x64 執行檔。不需要另外攜帶 ICO 或 SYSO。若檔案總管仍顯示舊圖示，請重新整理或將新版複製到另一個資料夾確認。

## 含圖示的跨平台發行包

需要 Go 1.22+ 與 Python 3.9+（macOS/Linux 通常使用 `python3`）。在專案根目錄執行：

```powershell
python scripts/build.py --target all
```

也可只建置指定平台：

```powershell
python scripts/build.py --target windows-amd64
python scripts/build.py --target darwin-arm64
python scripts/build.py --target darwin-amd64
python scripts/build.py --target linux-amd64
```

同一組命令可在 Windows、macOS、Linux 執行；目標設定只套用於子程序。輸出放在 `dist/`，Windows 為 ZIP，macOS/Linux 為保留執行權限的 TAR.GZ。封裝不包含既有索引、設定或備份；使用者不需要安裝 Go。作業系統最低需求亦取決於建置所用的 Go 版本。

| 平台 | 圖示與啟動方式 |
| --- | --- |
| Windows x64 | 解壓縮後雙擊 `FileIndex.exe`，圖示嵌入 EXE。 |
| macOS Apple Silicon / Intel | 在 Mac 解壓縮對應架構版本，雙擊 `FileIndex.app`；Finder 圖示由包內 ICNS 提供。 |
| Linux x64 | 解壓縮後執行 `./FileIndex`；要有圖示與選單入口，使用下方桌面啟動器。 |

### macOS

請保留完整的 `.app`。設定、資料庫、備份與匯出目錄會建立在 `.app` **旁邊**，請解壓縮到可寫入的資料夾，移動時一起帶走資料目錄。

發行包未經 Developer ID 簽章或公證，首次啟動可能需要在「系統設定 → 隱私權與安全性」允許執行。單獨 `go build -o FileIndex .` 仍可建立命令列執行檔，但要有 Finder 圖示請使用 `.app` 發行包。

### Linux

在解壓縮後的發行包目錄執行（僅設定啟動器需要 Python 3）：

```bash
python3 install-desktop.py
```

腳本會在程式旁建立 `FileIndex.desktop`，並安裝使用者的應用程式選單入口，不需要管理員權限。啟動器使用 `FileIndex.png` 的絕對路徑；移動資料夾、隨身碟掛載路徑改變或換電腦後，請重新執行。部分桌面環境需將啟動器標記為「允許啟動」。

Linux 圖示顯示於支援 Desktop Entry 的啟動器／選單；原始 ELF 執行檔不保證在檔案管理員顯示自訂圖示。macOS/Linux 不直接使用 Windows ICO。介面仍在瀏覽器中開啟，不會替換瀏覽器本身的 Dock／工作列圖示。

### 更新圖案

Windows 使用 `image/FileIndex.ico`，macOS/Linux 使用同圖案的高解析度 `image/FileIndex-logo.png`。請同時替換兩個來源，再重新產生資源與發行包：

```powershell
python -m pip install Pillow
python scripts/build.py --refresh-icons --target all
```

一般建置使用版本控制中的 SYSO 與 `image/FileIndex.icns`，不需要 Pillow 或下載圖示工具。只有 `--refresh-icons` 會用 Pillow 轉換 ICNS，並透過 Go 下載／執行固定版本 `github.com/akavel/rsrc@v0.10.2` 產生 Windows 資源；請將更新後的產物一起提交。替換圖片不會更新既有執行檔。

## Portable 目錄

建置後，程式以執行檔所在目錄作為 Portable Root（macOS `.app` 使用 bundle 所在目錄）。建議結構如下：

```text
FileIndex/
├── FileIndex.exe、FileIndex 或 FileIndex.app
├── config/
│   └── config.json
├── data/
│   └── fileindex.db
├── backup/
└── export/
```

`database_path`、`backup_path` 與 `export_path` 必須是 Portable Root 下的相對路徑。因此可以從捷徑、BAT、Shell Script、Finder 或應用程式選單啟動，資料仍會放在執行檔旁的相同 Portable Root 中。

## 尚待整合

原生目錄／資料庫檔案選擇器及 Filesystem identity 比對仍是後續的平台整合項目；目前以經過驗證的路徑欄位提供相同工作流程。
