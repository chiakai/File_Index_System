# File Index System

**English** | [繁體中文](README.md)

<p align="center"><img src="image/FileIndex-logo.png" alt="File Index System FI logo" width="128"></p>

## Project origin

This project was inspired by **CD Index 光碟索引大師**, freeware written by Tsai Ming-Hsiu (蔡明修). After using it for many years, I found it to be a very practical utility. On my current system, however, it sometimes reports an error after being left idle for too long. With AI development tools becoming widely available, I asked AI to help define the specification and develop this project.

For an introduction to **CD Index 光碟索引大師**, see [布丁布丁吃什麼？— CD Index 光碟索引大師](https://blog.pulipuli.info/2016/05/cd-index-cd-index-download.html).

File Index System is a portable, offline file metadata indexer based on the V1 specification. It stores filenames, relative paths, sizes, and modification times in SQLite so indexed files remain searchable when their source storage is offline.

## Features

- Filename and relative-path search with multi-keyword AND, storage and extension filters, four-step filename/path sorting, and pagination
- Add, edit, locate, rescan, export, and delete storage indexes
- Atomic staging scans with cancellation support
- Asynchronous UTF-8 BOM CSV exports
- Validated SQLite backup and complete restore with an automatic safety backup
- Portable JSON settings for page size, HTTP port, browser launch, and date-time display format
- Reveal and select indexed files in Windows, macOS, and Linux desktops that support `org.freedesktop.FileManager1`

## Screenshots

![File Index System search screen](image/System-01.png)

![File Index System storage screen](image/System-02.png)

## Using the interface

- Click **Filename / Relative Path** repeatedly to cycle through filename ascending, path plus filename ascending, filename descending, and path plus filename descending. The `F` or `P+F` marker and arrow show the active mode.
- Click **Storage**, **Size**, or **Modified** to sort ascending; click the same heading again to switch between ascending and descending.
- File sizes automatically use B, KB, MB, GB, TB, or PB. The number and unit may wrap separately on narrow screens.
- Dates and times use the format selected under **Settings → Date and time format**. The date and time may wrap onto separate lines.
- Click a search result to view its details. **Open Folder** opens the containing folder and selects the file when the desktop platform supports it.
- Scan progress, **Cancel scan**, and **Exit File Index** appear in the top navigation area to the right of **Settings**.

## Development mode

Development requires Go 1.22 or newer. Run these commands from the project directory containing `go.mod`:

```powershell
go mod tidy
go run .
```

`go run .` uses the project directory as the portable root. For regular use, build a native executable as described below. A built application does not require Go and can be started outside the project directory.

The server listens only on `127.0.0.1`. It opens the default browser when configured to do so. If automatic browser launch is disabled, open the one-time session URL printed in the terminal.

## Windows

### Build

Run in PowerShell:

```powershell
go build -o FileIndex.exe .
```

A successful `go build` normally prints no message. Confirm that `FileIndex.exe` was created in the project directory.

Start the application by double-clicking `FileIndex.exe` or from PowerShell:

```powershell
.\FileIndex.exe
```

You can also place a `Start-FileIndex.bat` file next to the executable:

```bat
@echo off
cd /d "%~dp0"
start "" "%~dp0FileIndex.exe"
```

Double-click the BAT file to start the application.

To run the development version from a BAT file, place this file in the project root:

```bat
@echo off
cd /d "%~dp0"
go run .
pause
```

This development launcher still requires Go to be installed.

### Executable icon

The `image` directory contains [`FileIndex.ico`](image/FileIndex.ico) for Windows executable packaging and [`FileIndex-logo.png`](image/FileIndex-logo.png) as its high-resolution source. You can replace both files with your own artwork before packaging a release. Keep the same filenames if your build or packaging script refers to them. Replacing these files does not alter an already-built executable; rebuild or repackage the application after changing the icon.

## macOS

### Build for the current Mac

```bash
go build -o FileIndex .
chmod +x FileIndex
```

Start it with:

```bash
./FileIndex
```

To launch it by double-clicking in Finder, create `start-fileindex.command` next to the executable:

```sh
#!/bin/sh
cd "$(dirname "$0")"
exec ./FileIndex
```

Make the launcher executable:

```bash
chmod +x start-fileindex.command
```

The first launch of an unsigned build may require approval under **System Settings → Privacy & Security**.

### Build for a specific architecture

Apple Silicon:

```bash
GOOS=darwin GOARCH=arm64 go build -o FileIndex-macos-arm64 .
```

Intel Mac:

```bash
GOOS=darwin GOARCH=amd64 go build -o FileIndex-macos-x64 .
```

## Linux

### Build

```bash
go build -o FileIndex .
chmod +x FileIndex
```

Start it with:

```bash
./FileIndex
```

You can also create `start-fileindex.sh`:

```sh
#!/bin/sh
cd "$(dirname "$0")"
exec ./FileIndex
```

Make it executable and run it:

```bash
chmod +x start-fileindex.sh
./start-fileindex.sh
```

To launch the application from a desktop environment menu, create a `.desktop` file:

```ini
[Desktop Entry]
Type=Application
Name=File Index
Exec=/absolute/path/to/FileIndex/FileIndex
Path=/absolute/path/to/FileIndex
Terminal=false
Categories=Utility;
```

Replace `Exec` and `Path` with the actual absolute paths.

## Cross-compile from Windows

Use PowerShell to build all supported targets:

```powershell
$env:GOOS="windows"; $env:GOARCH="amd64"; go build -o FileIndex-windows-x64.exe .
$env:GOOS="linux";   $env:GOARCH="amd64"; go build -o FileIndex-linux-x64 .
$env:GOOS="darwin";  $env:GOARCH="arm64"; go build -o FileIndex-macos-arm64 .
$env:GOOS="darwin";  $env:GOARCH="amd64"; go build -o FileIndex-macos-x64 .
```

## Portable directory

A built executable uses its own directory as the portable root. The recommended layout is:

```text
FileIndex/
├── FileIndex.exe or FileIndex
├── config/
│   └── config.json
├── data/
│   └── fileindex.db
├── backup/
└── export/
```

`database_path`, `backup_path`, and `export_path` must be relative to the portable root. The application can therefore be launched from a shortcut, BAT file, shell script, Finder, or an application menu while continuing to use the same data beside the executable.

## Remaining platform work

Native directory and database-file pickers and filesystem identity comparison remain platform-integration work. Validated path fields currently provide the corresponding workflows.
