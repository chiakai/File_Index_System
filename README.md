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

`image` 目錄中的 [`FileIndex.ico`](image/FileIndex.ico) 可供 Windows 執行檔封裝使用，[`FileIndex-logo.png`](image/FileIndex-logo.png) 則是高解析度來源圖。使用者可在封裝發行版本前，以自己的圖案替換這兩個檔案；若建置或封裝腳本引用這些檔名，請維持檔名不變。替換圖片不會直接修改已建置的執行檔，變更圖示後需重新建置或封裝。

## macOS

### 建置目前 Mac 的版本

```bash
go build -o FileIndex .
chmod +x FileIndex
```

啟動：

```bash
./FileIndex
```

若要從 Finder 雙擊啟動，可在執行檔旁建立 `start-fileindex.command`：

```sh
#!/bin/sh
cd "$(dirname "$0")"
exec ./FileIndex
```

設定執行權限：

```bash
chmod +x start-fileindex.command
```

第一次執行未簽章的程式時，可能需要在「系統設定 → 隱私權與安全性」中允許執行。

### 指定架構

Apple Silicon：

```bash
GOOS=darwin GOARCH=arm64 go build -o FileIndex-macos-arm64 .
```

Intel Mac：

```bash
GOOS=darwin GOARCH=amd64 go build -o FileIndex-macos-x64 .
```

## Linux

### 建置

```bash
go build -o FileIndex .
chmod +x FileIndex
```

啟動：

```bash
./FileIndex
```

也可以建立 `start-fileindex.sh`：

```sh
#!/bin/sh
cd "$(dirname "$0")"
exec ./FileIndex
```

設定權限並啟動：

```bash
chmod +x start-fileindex.sh
./start-fileindex.sh
```

如需從桌面環境的應用程式選單啟動，可建立 `.desktop` 檔：

```ini
[Desktop Entry]
Type=Application
Name=File Index
Exec=/完整路徑/FileIndex/FileIndex
Path=/完整路徑/FileIndex
Terminal=false
Categories=Utility;
```

請將 `Exec` 和 `Path` 改成實際絕對路徑。

## 從 Windows 交叉編譯

在 PowerShell 中可建置所有支援平台：

```powershell
$env:GOOS="windows"; $env:GOARCH="amd64"; go build -o FileIndex-windows-x64.exe .
$env:GOOS="linux";   $env:GOARCH="amd64"; go build -o FileIndex-linux-x64 .
$env:GOOS="darwin";  $env:GOARCH="arm64"; go build -o FileIndex-macos-arm64 .
$env:GOOS="darwin";  $env:GOARCH="amd64"; go build -o FileIndex-macos-x64 .
```

## Portable 目錄

建置後，程式以執行檔所在目錄作為 Portable Root。建議結構如下：

```text
FileIndex/
├── FileIndex.exe 或 FileIndex
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
