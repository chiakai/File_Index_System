package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecutableRoot(t *testing.T) {
	base := t.TempDir()
	for _, tc := range []struct{ path, platform, want string }{
		{filepath.Join(base, "FileIndex.app", "Contents", "MacOS", "FileIndex"), "darwin", base},
		{filepath.Join(base, "FileIndex"), "darwin", base},
		{filepath.Join(base, "other", "Contents", "MacOS", "FileIndex"), "darwin", filepath.Join(base, "other", "Contents", "MacOS")},
		{filepath.Join(base, "FileIndex.app", "Contents", "MacOS", "FileIndex"), "linux", filepath.Join(base, "FileIndex.app", "Contents", "MacOS")},
	} {
		if got := executableRoot(tc.path, tc.platform); got != tc.want {
			t.Errorf("executableRoot(%q, %q) = %q; want %q", tc.path, tc.platform, got, tc.want)
		}
	}
}

func testApp(t *testing.T) *App {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"data", "backup", "export", "source"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	db, err := openDatabase(filepath.Join(root, "data", "fileindex.db"))
	if err != nil {
		t.Fatal(err)
	}
	app := &App{root: root, dbPath: filepath.Join(root, "data", "fileindex.db"), cfg: defaultConfig(), db: db, jobs: map[string]*Job{}, exportJobs: map[string]*ExportJob{}}
	t.Cleanup(func() { _ = app.db.Close() })
	return app
}

func TestSafeResolveRejectsEscape(t *testing.T) {
	root := t.TempDir()
	if _, err := safeResolve(root, "../outside"); err == nil {
		t.Fatal("expected escaping path to be rejected")
	}
	if _, err := safeResolve(root, "data/fileindex.db"); err != nil {
		t.Fatalf("valid relative path rejected: %v", err)
	}
}

func TestTimeFormatConfiguration(t *testing.T) {
	root := t.TempDir()
	cfg := defaultConfig()
	cfg.TimeFormat = "yyyy-slash-24h"
	if err := validateConfig(root, cfg); err != nil {
		t.Fatalf("valid time format rejected: %v", err)
	}
	cfg.TimeFormat = "unknown"
	if err := validateConfig(root, cfg); err == nil {
		t.Fatal("invalid time format was accepted")
	}
}

func TestFileOrderModes(t *testing.T) {
	tests := []struct{ key, order, wantKey, wantOrder, wantExpression string }{
		{"name", "asc", "name", "ASC", "f.name ASC"},
		{"path_name", "asc", "path_name", "ASC", "f.relative_path ASC, f.name ASC"},
		{"name", "desc", "name", "DESC", "f.name DESC"},
		{"path_name", "desc", "path_name", "DESC", "f.relative_path DESC, f.name DESC"},
	}
	for _, tt := range tests {
		key, order, expression := fileOrder(tt.key, tt.order)
		if key != tt.wantKey || order != tt.wantOrder || expression != tt.wantExpression {
			t.Errorf("fileOrder(%q,%q)=(%q,%q,%q)", tt.key, tt.order, key, order, expression)
		}
	}
}

func TestWindowsOpenFolderArgumentsAreExtensionIndependent(t *testing.T) {
	root := `D:\Indexed files, archive`
	for _, name := range []string{
		`manual.pdf`,
		`notes.txt`,
		`document.docx`,
		`image.jpg`,
		`no-extension`,
		`中文 檔案,版本 2.pdf`,
	} {
		t.Run(name, func(t *testing.T) {
			target := filepath.Join(root, name)
			cmd := fileManagerCommand("windows", target)
			if cmd.Path != "explorer.exe" && filepath.Base(cmd.Path) != "explorer.exe" {
				t.Fatalf("unexpected command path: %q", cmd.Path)
			}
			want := []string{"explorer.exe", "/select,", target}
			if len(cmd.Args) != len(want) {
				t.Fatalf("args = %#v; want %#v", cmd.Args, want)
			}
			for i := range want {
				if cmd.Args[i] != want[i] {
					t.Fatalf("args = %#v; want %#v", cmd.Args, want)
				}
			}
		})
	}
}

func TestMigrateRejectsNewerSchema(t *testing.T) {
	db, err := openDatabase(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("PRAGMA user_version=2"); err != nil {
		t.Fatal(err)
	}
	if err := migrate(db); err == nil {
		t.Fatal("expected newer schema to be rejected")
	}
}

func TestRescanUsesStoredRoot(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(fmt.Sprintf("fallback=%v", fallback), func(t *testing.T) {
			app := testApp(t)
			root := filepath.Join(app.root, "source")
			if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("new"), 0600); err != nil {
				t.Fatal(err)
			}
			current, last := root, "missing-old-root"
			if fallback {
				current, last = "", root
			}
			result, err := app.db.Exec("INSERT INTO storages(name,last_scan_root,current_root,created_at) VALUES(?,?,?,?)", "archive", last, current, now())
			if err != nil {
				t.Fatal(err)
			}
			id, _ := result.LastInsertId()
			w := httptest.NewRecorder()
			app.scans(w, httptest.NewRequest("POST", "/scans", strings.NewReader(fmt.Sprintf(`{"mode":"rescan","storage_id":%d}`, id))))
			if w.Code != 201 {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				app.mu.RLock()
				var status string
				for _, job := range app.jobs {
					status = job.Status
				}
				app.mu.RUnlock()
				if status == "completed" {
					break
				}
				if status == "failed" || time.Now().After(deadline) {
					t.Fatalf("scan status: %s", status)
				}
				time.Sleep(10 * time.Millisecond)
			}
			var name, savedRoot string
			if err := app.db.QueryRow("SELECT name FROM files WHERE storage_id=?", id).Scan(&name); err != nil {
				t.Fatal(err)
			}
			if err := app.db.QueryRow("SELECT last_scan_root FROM storages WHERE id=?", id).Scan(&savedRoot); err != nil {
				t.Fatal(err)
			}
			if name != "new.txt" || savedRoot != root {
				t.Fatalf("indexed %q at %q", name, savedRoot)
			}
		})
	}
}

func TestRescanOfflineDoesNotUseSuppliedOrHistoricalRoot(t *testing.T) {
	app := testApp(t)
	root := filepath.Join(app.root, "source")
	result, err := app.db.Exec("INSERT INTO storages(name,last_scan_root,current_root,created_at) VALUES(?,?,?,?)", "archive", root, filepath.Join(root, "missing"), now())
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	if _, err := app.db.Exec("INSERT INTO files(storage_id,name,relative_path,size_bytes) VALUES(?,?,?,?)", id, "old.txt", "", 10); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	app.scans(w, httptest.NewRequest("POST", "/scans", strings.NewReader(fmt.Sprintf(`{"mode":"rescan","storage_id":%d,"scan_root":"."}`, id))))
	if w.Code != 422 || !strings.Contains(w.Body.String(), "STORAGE_OFFLINE") || len(app.jobs) != 0 {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	var name string
	if err := app.db.QueryRow("SELECT name FROM files WHERE storage_id=?", id).Scan(&name); err != nil || name != "old.txt" {
		t.Fatalf("index changed: %q %v", name, err)
	}
}

func TestDeleteStorageCascadesOnlyItsIndex(t *testing.T) {
	app := testApp(t)
	source := filepath.Join(app.root, "source", "keep.txt")
	if err := os.WriteFile(source, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, name := range []string{"remove", "keep"} {
		result, err := app.db.Exec("INSERT INTO storages(name,last_scan_root,created_at) VALUES(?,?,?)", name, filepath.Dir(source), now())
		if err != nil {
			t.Fatal(err)
		}
		id, _ := result.LastInsertId()
		ids = append(ids, id)
		if _, err := app.db.Exec("INSERT INTO files(storage_id,name,relative_path,size_bytes) VALUES(?,?,?,?)", id, "keep.txt", "", 4); err != nil {
			t.Fatal(err)
		}
	}
	w := httptest.NewRecorder()
	app.storageRoute(w, httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/storages/%d", ids[0]), nil))
	if w.Code != 200 {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	for _, table := range []string{"storages", "files"} {
		column := "storage_id"
		if table == "storages" {
			column = "id"
		}
		for i, id := range ids {
			var count int
			if err := app.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE "+column+"=?", id).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != i {
				t.Fatalf("%s for %d: got %d, want %d", table, id, count, i)
			}
		}
	}
	if content, err := os.ReadFile(source); err != nil || string(content) != "keep" {
		t.Fatalf("source changed: %q %v", content, err)
	}
}

func TestStorageDeleteShrinksExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	if _, err := legacy.Exec("PRAGMA auto_vacuum=NONE"); err != nil {
		t.Fatal(err)
	}
	if err := migrate(legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec("INSERT INTO storages(id,name,last_scan_root,created_at) VALUES(1,'remove','root','today'),(2,'keep','root','today')"); err != nil {
		t.Fatal(err)
	}
	tx, err := legacy.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1000; i++ {
		if _, err := tx.Exec("INSERT INTO files(storage_id,name,relative_path,size_bytes) VALUES(1,?,?,1)", fmt.Sprintf("file-%d", i), strings.Repeat("x", 1024)); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec("INSERT INTO files(storage_id,name,relative_path,size_bytes) VALUES(2,'keep.txt','',4)"); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := openDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{db: db}
	w := httptest.NewRecorder()
	app.storageRoute(w, httptest.NewRequest("DELETE", "/api/v1/storages/1", nil))
	if w.Code != 200 {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() >= before.Size() {
		t.Fatalf("database did not shrink: %d -> %d", before.Size(), after.Size())
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM files WHERE storage_id=2 AND name='keep.txt'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("remaining index: %d %v", count, err)
	}
	var check string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&check); err != nil || check != "ok" {
		t.Fatalf("integrity: %s %v", check, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = openDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var mode int
	if err := db.QueryRow("PRAGMA auto_vacuum").Scan(&mode); err != nil || mode != 1 {
		t.Fatalf("persistent auto_vacuum: %d %v", mode, err)
	}
	t.Logf("database shrank from %d to %d bytes", before.Size(), after.Size())
}

func TestCancelledRescanPreservesIndex(t *testing.T) {
	app := testApp(t)
	result, err := app.db.Exec("INSERT INTO storages(name,last_scan_root,current_root,file_count,total_size_bytes,created_at) VALUES(?,?,?,?,?,?)", "archive", "old", "old", 1, 10, now())
	if err != nil {
		t.Fatal(err)
	}
	storageID, _ := result.LastInsertId()
	if _, err := app.db.Exec("INSERT INTO files(storage_id,name,relative_path,size_bytes) VALUES(?,?,?,?)", storageID, "old.txt", "", 10); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(app.root, "source")
	if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := scanRequest{Mode: "rescan", StorageID: storageID, ScanRoot: root}
	job := &Job{ID: "scan-test", Status: "running", Mode: "rescan", StorageID: storageID, Root: root}
	app.jobs[job.ID] = job
	app.runScan(ctx, job, request)
	var name string
	if err := app.db.QueryRow("SELECT name FROM files WHERE storage_id=?", storageID).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "old.txt" || job.Status != "cancelled" {
		t.Fatalf("old index was not preserved: name=%q status=%q", name, job.Status)
	}
}

func TestExportJobWritesValidCSV(t *testing.T) {
	app := testApp(t)
	result, err := app.db.Exec("INSERT INTO storages(name,last_scan_root,current_root,file_count,total_size_bytes,last_scan_at,created_at) VALUES(?,?,?,?,?,?,?)", "store,one", "root", "root", 1, 5, now(), now())
	if err != nil {
		t.Fatal(err)
	}
	storageID, _ := result.LastInsertId()
	if _, err := app.db.Exec("INSERT INTO files(storage_id,name,extension,relative_path,size_bytes,modified_at) VALUES(?,?,?,?,?,?)", storageID, "a\"b.csv", "csv", "folder\nname", 5, now()); err != nil {
		t.Fatal(err)
	}
	job := &ExportJob{ID: "export-test", Status: "running"}
	app.exportCSV(job, "storage", "", "", "", storageID, "name", "asc")
	if job.Status != "completed" || job.RecordCount != 1 {
		t.Fatalf("unexpected export result: %+v", job)
	}
	b, err := os.ReadFile(filepath.Join(app.root, filepath.FromSlash(job.RelativePath)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("CSV is missing UTF-8 BOM")
	}
	records, err := csv.NewReader(bytes.NewReader(b[3:])).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[1][1] != "a\"b.csv" || records[1][3] != "folder\nname" {
		t.Fatalf("CSV fields were not preserved: %#v", records)
	}
}

func TestRestoreReplacesDatabaseAndCreatesSafetyBackup(t *testing.T) {
	app := testApp(t)
	if _, err := app.db.Exec("INSERT INTO storages(name,last_scan_root,created_at) VALUES(?,?,?)", "current", "root", now()); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(app.root, "backup", "source.db")
	if err := backupDatabaseForTest(app.db, source); err != nil {
		t.Fatal(err)
	}
	sourceDB, err := openDatabase(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sourceDB.Exec("UPDATE storages SET name='restored'"); err != nil {
		t.Fatal(err)
	}
	if err := sourceDB.Close(); err != nil {
		t.Fatal(err)
	}
	if err := app.restoreDatabase(source); err != nil {
		t.Fatal(err)
	}
	var name string
	if err := app.db.QueryRow("SELECT name FROM storages LIMIT 1").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "restored" {
		t.Fatalf("database was not restored: %q", name)
	}
	matches, err := filepath.Glob(filepath.Join(app.root, "backup", "FileIndex_PreRestore_*.db"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("expected safety backup, got %v (%v)", matches, err)
	}
}

func backupDatabaseForTest(db *sql.DB, destination string) error {
	quoted := strings.ReplaceAll(filepath.ToSlash(destination), "'", "''")
	_, err := db.Exec("VACUUM INTO '" + quoted + "'")
	return err
}
