package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
