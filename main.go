package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed web
var webFS embed.FS

type Config struct {
	DatabasePath    string `json:"database_path"`
	BackupPath      string `json:"backup_path"`
	ExportPath      string `json:"export_path"`
	SearchPageSize  int    `json:"search_page_size"`
	HTTPPort        int    `json:"http_port"`
	AutoOpenBrowser bool   `json:"auto_open_browser"`
	TimeFormat      string `json:"time_format"`
}

type App struct {
	root        string
	dbPath      string
	cfg         Config
	db          *sql.DB
	token       string
	jobs        map[string]*Job
	exportJobs  map[string]*ExportJob
	mu          sync.RWMutex
	dbMu        sync.RWMutex
	maintenance bool
	srv         *http.Server
}
type ExportJob struct {
	ID           string `json:"job_id"`
	Status       string `json:"status"`
	Filename     string `json:"filename,omitempty"`
	RelativePath string `json:"relative_path,omitempty"`
	RecordCount  int64  `json:"record_count"`
	Err          string `json:"error,omitempty"`
}
type scanRequest struct {
	Mode      string `json:"mode"`
	StorageID int64  `json:"storage_id"`
	ScanRoot  string `json:"scan_root"`
	Storage   struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"storage"`
}
type Job struct {
	ID                 string             `json:"job_id"`
	Status             string             `json:"status"`
	Mode               string             `json:"mode"`
	StorageID          int64              `json:"storage_id,omitempty"`
	StorageName        string             `json:"storage_name"`
	Root               string             `json:"-"`
	CurrentPath        string             `json:"current_path"`
	FilesScanned       int64              `json:"files_scanned"`
	DirectoriesScanned int64              `json:"directories_scanned"`
	SizeBytes          int64              `json:"size_bytes"`
	ErrorCount         int                `json:"error_count"`
	Started            time.Time          `json:"-"`
	Err                string             `json:"error,omitempty"`
	cancel             context.CancelFunc `json:"-"`
}
type FileRow struct {
	ID, StorageID                              int64
	StorageName, Name, Extension, RelativePath string
	SizeBytes                                  int64
	ModifiedAt                                 sql.NullString
}
type storage struct {
	ID             int64          `json:"id"`
	Name           string         `json:"name"`
	Description    sql.NullString `json:"description"`
	VolumeLabel    sql.NullString `json:"volume_label"`
	FilesystemType sql.NullString `json:"filesystem_type"`
	FilesystemID   sql.NullString `json:"filesystem_id"`
	CapacityBytes  sql.NullInt64  `json:"capacity_bytes"`
	LastScanRoot   string         `json:"last_scan_root"`
	CurrentRoot    sql.NullString `json:"current_root"`
	FileCount      int64          `json:"file_count"`
	TotalSize      int64          `json:"total_size_bytes"`
	LastScanAt     sql.NullString `json:"last_scan_at"`
	CreatedAt      string         `json:"created_at"`
	Available      bool           `json:"available"`
}

func main() {
	root, err := portableRoot()
	if err != nil {
		log.Fatal(err)
	}
	cfg := defaultConfig()
	if err := loadConfig(root, &cfg); err != nil {
		log.Fatal(err)
	}
	if err := validateConfig(root, cfg); err != nil {
		log.Fatal(err)
	}
	dbPath, _ := safeResolve(root, cfg.DatabasePath)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		log.Fatal(err)
	}
	db, err := openDatabase(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	token := randomToken()
	app := &App{root: root, dbPath: dbPath, cfg: cfg, db: db, token: token, jobs: map[string]*Job{}, exportJobs: map[string]*ExportJob{}}
	mux := http.NewServeMux()
	app.routes(mux)
	port := cfg.HTTPPort
	if port < 0 || port > 65535 {
		port = 0
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		log.Fatal(err)
	}
	app.srv = &http.Server{Handler: app.auth(mux)}
	url := "http://127.0.0.1:" + strconv.Itoa(ln.Addr().(*net.TCPAddr).Port) + "/?token=" + token
	log.Printf("File Index is listening on %s", ln.Addr())
	if cfg.AutoOpenBrowser {
		go openBrowser(url)
	} else {
		log.Printf("Open this session URL: %s", url)
	}
	if err := app.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func portableRoot() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}
	// `go run` uses a temporary executable. Keep development data in the source tree.
	if strings.Contains(filepath.ToSlash(exe), "/go-build") {
		if cwd, e := os.Getwd(); e == nil {
			if _, e = os.Stat(filepath.Join(cwd, "go.mod")); e == nil {
				return cwd, nil
			}
		}
	}
	return executableRoot(exe, runtime.GOOS), nil
}

// Keep portable data beside the macOS bundle, not inside its signed contents.
func executableRoot(exe, platform string) string {
	dir := filepath.Dir(exe)
	if platform == "darwin" && filepath.Base(dir) == "MacOS" {
		contents := filepath.Dir(dir)
		bundle := filepath.Dir(contents)
		if filepath.Base(contents) == "Contents" && strings.HasSuffix(filepath.Base(bundle), ".app") {
			return filepath.Dir(bundle)
		}
	}
	return dir
}

func defaultConfig() Config {
	return Config{"data/fileindex.db", "backup", "export", 100, 0, true, "locale"}
}
func safeResolve(root, p string) (string, error) {
	if p == "" || filepath.IsAbs(p) || filepath.VolumeName(p) != "" {
		return "", fmt.Errorf("path must be relative to the portable root: %q", p)
	}
	root = filepath.Clean(root)
	resolved := filepath.Clean(filepath.Join(root, filepath.FromSlash(p)))
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the portable root: %q", p)
	}
	return resolved, nil
}
func resolve(root, p string) string { resolved, _ := safeResolve(root, p); return resolved }
func loadConfig(root string, c *Config) error {
	b, err := os.ReadFile(filepath.Join(root, "config", "config.json"))
	if os.IsNotExist(err) {
		_ = os.MkdirAll(filepath.Join(root, "config"), 0755)
		return saveConfig(root, c)
	}
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, c); err != nil {
		return err
	}
	return nil
}
func saveConfig(root string, c *Config) error {
	p := filepath.Join(root, "config", "config.json")
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0644); err != nil {
		return err
	}
	old := p + ".old"
	_ = os.Remove(old)
	if _, err := os.Stat(p); err == nil {
		if err = os.Rename(p, old); err != nil {
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Rename(old, p)
		return err
	}
	_ = os.Remove(old)
	return nil
}
func validateConfig(root string, c Config) error {
	if c.SearchPageSize < 1 || c.SearchPageSize > 1000 || c.HTTPPort < 0 || c.HTTPPort > 65535 {
		return errors.New("search_page_size or http_port is out of range")
	}
	for _, p := range []string{c.DatabasePath, c.BackupPath, c.ExportPath} {
		if _, err := safeResolve(root, p); err != nil {
			return err
		}
	}
	validTimeFormats := map[string]bool{"locale": true, "yyyy-mm-dd-24h": true, "yyyy-slash-24h": true, "us-12h": true, "eu-24h": true}
	if !validTimeFormats[c.TimeFormat] {
		return errors.New("time_format is invalid")
	}
	return nil
}
func randomToken() string     { b := make([]byte, 24); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func now() string             { return time.Now().UTC().Format(time.RFC3339) }
func (a *App) config() Config { a.mu.RLock(); defer a.mu.RUnlock(); return a.cfg }
func openDatabase(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err = migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	if err = enableAutoVacuum(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable automatic database space reclamation: %w", err)
	}
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'staging_scan_%'")
	if err != nil {
		db.Close()
		return nil, err
	}
	var stale []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			db.Close()
			return nil, err
		}
		stale = append(stale, name)
	}
	rows.Close()
	for _, name := range stale {
		if _, err = db.Exec("DROP TABLE " + name); err != nil {
			db.Close()
			return nil, err
		}
	}
	return db, nil
}

// Configure before serving requests, including when opening a restored database.
// Existing NONE databases require a one-time rebuild to add pointer-map pages.
func enableAutoVacuum(db *sql.DB) error {
	var mode int
	if err := db.QueryRow("PRAGMA auto_vacuum").Scan(&mode); err != nil {
		return err
	}
	if mode == 1 {
		return nil
	}
	if _, err := db.Exec("PRAGMA auto_vacuum = FULL"); err != nil {
		return err
	}
	if mode == 0 {
		if _, err := db.Exec("VACUUM"); err != nil {
			return err
		}
	}
	if err := db.QueryRow("PRAGMA auto_vacuum").Scan(&mode); err != nil {
		return err
	}
	if mode != 1 {
		return errors.New("database did not enable FULL auto_vacuum")
	}
	return nil
}

func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > 1 {
		return fmt.Errorf("database schema version %d is newer than supported version 1", version)
	}
	if version == 1 {
		return validateSchema(db)
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statements := []string{`CREATE TABLE storages (id INTEGER PRIMARY KEY, name TEXT NOT NULL, description TEXT, volume_label TEXT, filesystem_type TEXT, filesystem_id TEXT, capacity_bytes INTEGER, last_scan_root TEXT NOT NULL, current_root TEXT, file_count INTEGER NOT NULL DEFAULT 0, total_size_bytes INTEGER NOT NULL DEFAULT 0, last_scan_at TEXT, created_at TEXT NOT NULL)`, `CREATE TABLE files (id INTEGER PRIMARY KEY, storage_id INTEGER NOT NULL REFERENCES storages(id) ON DELETE CASCADE, name TEXT NOT NULL, extension TEXT, relative_path TEXT NOT NULL, size_bytes INTEGER NOT NULL, modified_at TEXT)`, `CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`, `CREATE INDEX idx_files_storage ON files(storage_id)`, `CREATE INDEX idx_files_extension ON files(extension)`, `CREATE INDEX idx_files_name ON files(name)`, `PRAGMA user_version=1`}
	for _, s := range statements {
		if _, err := tx.Exec(s); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func validateSchema(db *sql.DB) error {
	for _, table := range []string{"storages", "files", "settings"} {
		var name string
		if err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name); err != nil {
			return fmt.Errorf("invalid version 1 database: missing %s table", table)
		}
	}
	return nil
}

func (a *App) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
			httpError(w, 403, "INVALID_HOST", "Only loopback hosts are allowed", nil)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
			httpError(w, 403, "ORIGIN_NOT_ALLOWED", "Origin is not allowed", nil)
			return
		}
		if r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/web/") {
			next.ServeHTTP(w, r)
			return
		}
		supplied := r.Header.Get("X-FileIndex-Token")
		if supplied == "" {
			if cookie, err := r.Cookie("fileindex_token"); err == nil {
				supplied = cookie.Value
			}
		}
		if supplied == "" {
			supplied = r.URL.Query().Get("token")
		}
		if supplied != a.token {
			httpError(w, 401, "UNAUTHORIZED", "Invalid session token", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (a *App) routes(m *http.ServeMux) {
	m.HandleFunc("/", a.index)
	m.HandleFunc("/api/v1/system/status", a.status)
	m.HandleFunc("/api/v1/system/shutdown", a.shutdown)
	m.Handle("/api/v1/files/search", a.dbAccess(http.HandlerFunc(a.search)))
	m.Handle("/api/v1/files/extensions", a.dbAccess(http.HandlerFunc(a.extensions)))
	m.Handle("/api/v1/files/", a.dbAccess(http.HandlerFunc(a.fileRoute)))
	m.Handle("/api/v1/storages", a.dbAccess(http.HandlerFunc(a.storages)))
	m.Handle("/api/v1/storages/", a.dbAccess(http.HandlerFunc(a.storageRoute)))
	m.Handle("/api/v1/scans", a.dbAccess(http.HandlerFunc(a.scans)))
	m.HandleFunc("/api/v1/scans/", a.scanRoute)
	m.HandleFunc("/api/v1/exports", a.exports)
	m.HandleFunc("/api/v1/exports/", a.exportRoute)
	m.Handle("/api/v1/backups", a.dbAccess(http.HandlerFunc(a.backups)))
	m.HandleFunc("/api/v1/backups/", a.backupRoute)
	m.HandleFunc("/api/v1/settings", a.settings)
}
func (a *App) dbAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.RLock()
		maintenance := a.maintenance
		a.mu.RUnlock()
		if maintenance {
			httpError(w, 503, "DATABASE_MAINTENANCE", "Database maintenance is in progress", nil)
			return
		}
		a.dbMu.RLock()
		defer a.dbMu.RUnlock()
		next.ServeHTTP(w, r)
	})
}
func (a *App) index(w http.ResponseWriter, r *http.Request) {
	if token := r.URL.Query().Get("token"); token != "" {
		if token != a.token {
			httpError(w, 401, "UNAUTHORIZED", "Invalid session token", nil)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "fileindex_token", Value: a.token, HttpOnly: true, SameSite: http.SameSiteStrictMode, Path: "/"})
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	b, err := fs.ReadFile(webFS, "web/index.html")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.ServeContent(w, r, "index.html", time.Time{}, strings.NewReader(string(b)))
}
func jsonWrite(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}
func httpError(w http.ResponseWriter, status int, code, msg string, details any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": msg, "details": details}})
}
func decode(r *http.Request, v any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(v)
}
func (a *App) status(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	var active any
	for _, j := range a.jobs {
		if j.Status == "running" || j.Status == "cancelling" {
			active = j.ID
		}
	}
	ready := !a.maintenance
	a.mu.RUnlock()
	jsonWrite(w, 200, map[string]any{"version": "1.0.0", "database_ready": ready, "schema_version": 1, "active_scan": active})
}
func (a *App) shutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		httpError(w, 405, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}
	var req struct {
		Force bool `json:"force"`
	}
	if err := decode(r, &req); err != nil && !errors.Is(err, io.EOF) {
		httpError(w, 400, "INVALID_REQUEST", "Invalid shutdown request", nil)
		return
	}
	a.mu.Lock()
	active := []*Job{}
	for _, j := range a.jobs {
		if j.Status == "running" || j.Status == "cancelling" {
			active = append(active, j)
		}
	}
	if len(active) > 0 && !req.Force {
		a.mu.Unlock()
		httpError(w, 409, "SCAN_IN_PROGRESS", "A scan is still running", nil)
		return
	}
	for _, j := range active {
		if j.cancel != nil {
			j.cancel()
		}
		j.Status = "cancelling"
	}
	a.mu.Unlock()
	jsonWrite(w, 200, map[string]bool{"stopping": true})
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			a.mu.RLock()
			running := false
			for _, j := range active {
				if j.Status == "running" || j.Status == "cancelling" {
					running = true
				}
			}
			a.mu.RUnlock()
			if !running {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.srv.Shutdown(ctx)
	}()
}

func (a *App) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := positive(q.Get("page"), 1)
	size := positive(q.Get("page_size"), a.config().SearchPageSize)
	if size > 1000 {
		size = 1000
	}
	sortKey, order, orderExpression := fileOrder(q.Get("sort"), q.Get("order"))
	where := []string{"1=1"}
	args := []any{}
	for _, term := range strings.Fields(q.Get("q")) {
		where = append(where, "(LOWER(f.name) LIKE LOWER(?) OR LOWER(f.relative_path) LIKE LOWER(?))")
		like := "%" + term + "%"
		args = append(args, like, like)
	}
	if v := q.Get("storage_id"); v != "" {
		id, e := strconv.ParseInt(v, 10, 64)
		if e != nil {
			httpError(w, 400, "INVALID_PARAMETER", "Invalid storage_id", nil)
			return
		}
		where = append(where, "f.storage_id=?")
		args = append(args, id)
	}
	if v := q.Get("extension"); v != "" {
		where = append(where, "LOWER(f.extension)=LOWER(?)")
		args = append(args, strings.TrimPrefix(v, "."))
	}
	base := " FROM files f JOIN storages s ON s.id=f.storage_id WHERE " + strings.Join(where, " AND ")
	var total int
	_ = a.db.QueryRow("SELECT COUNT(*)"+base, args...).Scan(&total)
	rows, err := a.db.Query("SELECT f.id,f.storage_id,s.name,f.name,COALESCE(f.extension,''),f.relative_path,f.size_bytes,f.modified_at"+base+" ORDER BY "+orderExpression+", f.id "+order+" LIMIT ? OFFSET ?", append(args, size, (page-1)*size)...)
	if err != nil {
		httpError(w, 500, "SEARCH_FAILED", err.Error(), nil)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var f FileRow
		if err := rows.Scan(&f.ID, &f.StorageID, &f.StorageName, &f.Name, &f.Extension, &f.RelativePath, &f.SizeBytes, &f.ModifiedAt); err == nil {
			out = append(out, fileJSON(f))
		}
	}
	pages := (total + size - 1) / size
	if pages == 0 {
		pages = 1
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": out, "meta": map[string]any{"total": total, "page": page, "page_size": size, "total_pages": pages, "sort": sortKey, "order": strings.ToLower(order)}})
}
func fileOrder(sortKey, requestedOrder string) (string, string, string) {
	order := "ASC"
	if strings.EqualFold(requestedOrder, "desc") {
		order = "DESC"
	}
	switch sortKey {
	case "path_name":
		return sortKey, order, "f.relative_path " + order + ", f.name " + order
	case "storage":
		return sortKey, order, "s.name " + order
	case "size":
		return sortKey, order, "f.size_bytes " + order
	case "modified":
		return sortKey, order, "f.modified_at " + order
	default:
		return "name", order, "f.name " + order
	}
}
func fileJSON(f FileRow) map[string]any {
	return map[string]any{"id": f.ID, "storage_id": f.StorageID, "storage_name": f.StorageName, "name": f.Name, "extension": f.Extension, "relative_path": f.RelativePath, "size_bytes": f.SizeBytes, "modified_at": nullString(f.ModifiedAt)}
}
func nullString(v sql.NullString) any {
	if v.Valid {
		return v.String
	}
	return nil
}
func positive(s string, d int) int {
	v, e := strconv.Atoi(s)
	if e != nil || v < 1 {
		return d
	}
	return v
}
func (a *App) extensions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	args := []any{}
	where := []string{"extension IS NOT NULL", "extension<>''"}
	if s := q.Get("storage_id"); s != "" {
		id, e := strconv.ParseInt(s, 10, 64)
		if e != nil {
			httpError(w, 400, "INVALID_PARAMETER", "Invalid storage_id", nil)
			return
		}
		where = append(where, "storage_id=?")
		args = []any{id}
	}
	rows, err := a.db.Query("SELECT extension,COUNT(*) FROM files WHERE "+strings.Join(where, " AND ")+" GROUP BY extension ORDER BY extension", args...)
	if err != nil {
		httpError(w, 500, "QUERY_FAILED", err.Error(), nil)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var e string
		var n int
		_ = rows.Scan(&e, &n)
		out = append(out, map[string]any{"extension": e, "count": n})
	}
	jsonWrite(w, 200, out)
}

func (a *App) storages(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		rows, err := a.db.Query("SELECT id,name,description,volume_label,filesystem_type,filesystem_id,capacity_bytes,last_scan_root,current_root,file_count,total_size_bytes,last_scan_at,created_at FROM storages ORDER BY name")
		if err != nil {
			httpError(w, 500, "QUERY_FAILED", err.Error(), nil)
			return
		}
		defer rows.Close()
		out := []any{}
		for rows.Next() {
			s, err := scanStorage(rows)
			if err == nil {
				s.Available = pathAvailable(s.CurrentRoot, s.LastScanRoot)
				out = append(out, storageJSON(s))
			}
		}
		jsonWrite(w, 200, out)
		return
	}
	httpError(w, 405, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
}
func scanStorage(rows interface{ Scan(...any) error }) (storage, error) {
	var s storage
	err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.VolumeLabel, &s.FilesystemType, &s.FilesystemID, &s.CapacityBytes, &s.LastScanRoot, &s.CurrentRoot, &s.FileCount, &s.TotalSize, &s.LastScanAt, &s.CreatedAt)
	return s, err
}
func storageJSON(s storage) map[string]any {
	return map[string]any{"id": s.ID, "name": s.Name, "description": nullString(s.Description), "volume_label": nullString(s.VolumeLabel), "filesystem_type": nullString(s.FilesystemType), "filesystem_id": nullString(s.FilesystemID), "capacity_bytes": nullInt(s.CapacityBytes), "last_scan_root": s.LastScanRoot, "current_root": nullString(s.CurrentRoot), "file_count": s.FileCount, "total_size_bytes": s.TotalSize, "last_scan_at": nullString(s.LastScanAt), "created_at": s.CreatedAt, "available": s.Available}
}
func nullInt(v sql.NullInt64) any {
	if v.Valid {
		return v.Int64
	}
	return nil
}
func pathAvailable(cur sql.NullString, last string) bool {
	if !cur.Valid || cur.String == "" {
		return false
	}
	st, err := os.Stat(cur.String)
	return err == nil && st.IsDir()
}

func (a *App) fileRoute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		httpError(w, 404, "NOT_FOUND", "File not found", nil)
		return
	}
	id, e := strconv.ParseInt(parts[3], 10, 64)
	if e != nil {
		httpError(w, 400, "INVALID_ID", "Invalid file id", nil)
		return
	}
	if len(parts) == 5 && parts[4] == "open-folder" {
		if r.Method != "POST" {
			httpError(w, 405, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
			return
		}
		a.openFolder(w, id)
		return
	}
	var f FileRow
	var s storage
	row := a.db.QueryRow("SELECT f.id,f.storage_id,s.name,f.name,COALESCE(f.extension,''),f.relative_path,f.size_bytes,f.modified_at,s.id,s.name,s.description,s.volume_label,s.filesystem_type,s.filesystem_id,s.capacity_bytes,s.last_scan_root,s.current_root,s.file_count,s.total_size_bytes,s.last_scan_at,s.created_at FROM files f JOIN storages s ON s.id=f.storage_id WHERE f.id=?", id)
	err := row.Scan(&f.ID, &f.StorageID, &f.StorageName, &f.Name, &f.Extension, &f.RelativePath, &f.SizeBytes, &f.ModifiedAt, &s.ID, &s.Name, &s.Description, &s.VolumeLabel, &s.FilesystemType, &s.FilesystemID, &s.CapacityBytes, &s.LastScanRoot, &s.CurrentRoot, &s.FileCount, &s.TotalSize, &s.LastScanAt, &s.CreatedAt)
	if err != nil {
		httpError(w, 404, "FILE_NOT_FOUND", "File not found", nil)
		return
	}
	s.Available = pathAvailable(s.CurrentRoot, s.LastScanRoot)
	jsonWrite(w, 200, map[string]any{"id": f.ID, "name": f.Name, "extension": f.Extension, "relative_path": f.RelativePath, "size_bytes": f.SizeBytes, "modified_at": nullString(f.ModifiedAt), "storage": map[string]any{"id": s.ID, "name": s.Name, "last_scan_at": nullString(s.LastScanAt), "available": s.Available}})
}
func (a *App) openFolder(w http.ResponseWriter, id int64) {
	var path, name string
	var cur sql.NullString
	err := a.db.QueryRow("SELECT s.current_root,f.relative_path,f.name FROM files f JOIN storages s ON s.id=f.storage_id WHERE f.id=?", id).Scan(&cur, &path, &name)
	if err != nil {
		httpError(w, 404, "FILE_NOT_FOUND", "File not found", nil)
		return
	}
	if !cur.Valid || cur.String == "" {
		httpError(w, 422, "STORAGE_OFFLINE", "Storage is not available", nil)
		return
	}
	root, err := filepath.EvalSymlinks(cur.String)
	if err != nil {
		httpError(w, 422, "STORAGE_OFFLINE", "Storage is not available", nil)
		return
	}
	target, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(path), name))
	if err != nil {
		httpError(w, 422, "FILE_NOT_FOUND_ON_SOURCE", "The indexed file no longer exists at the current location", nil)
		return
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		httpError(w, 422, "DIRECTORY_NOT_FOUND", "Directory is outside the storage root", nil)
		return
	}
	if st, e := os.Stat(target); e != nil || st.IsDir() {
		httpError(w, 422, "FILE_NOT_FOUND_ON_SOURCE", "The indexed file no longer exists at the current location", nil)
		return
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("explorer.exe", "/select,"+target)
	} else if runtime.GOOS == "darwin" {
		cmd = exec.Command("open", "-R", target)
	} else {
		uri := (&url.URL{Scheme: "file", Path: filepath.ToSlash(target)}).String()
		cmd = exec.Command("dbus-send", "--session", "--dest=org.freedesktop.FileManager1", "--type=method_call", "/org/freedesktop/FileManager1", "org.freedesktop.FileManager1.ShowItems", "array:string:"+uri, "string:")
	}
	if err = cmd.Start(); err != nil {
		httpError(w, 500, "OPEN_FOLDER_FAILED", err.Error(), nil)
		return
	}
	jsonWrite(w, 200, map[string]bool{"opened": true, "selected": true})
}

func (a *App) storageRoute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		httpError(w, 404, "NOT_FOUND", "Storage not found", nil)
		return
	}
	id, e := strconv.ParseInt(parts[3], 10, 64)
	if e != nil {
		httpError(w, 400, "INVALID_ID", "Invalid storage id", nil)
		return
	}
	if len(parts) == 5 && parts[4] == "locate" {
		if r.Method != "POST" {
			httpError(w, 405, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
			return
		}
		a.locate(w, r, id)
		return
	}
	if r.Method == "DELETE" {
		_, err := a.db.Exec("DELETE FROM storages WHERE id=?", id)
		if err != nil {
			httpError(w, 500, "DELETE_FAILED", err.Error(), nil)
			return
		}
		jsonWrite(w, 200, map[string]bool{"deleted": true})
		return
	}
	if r.Method == "GET" {
		row := a.db.QueryRow("SELECT id,name,description,volume_label,filesystem_type,filesystem_id,capacity_bytes,last_scan_root,current_root,file_count,total_size_bytes,last_scan_at,created_at FROM storages WHERE id=?", id)
		s, err := rowStorage(row)
		if err != nil {
			httpError(w, 404, "STORAGE_NOT_FOUND", "Storage not found", nil)
			return
		}
		s.Available = pathAvailable(s.CurrentRoot, s.LastScanRoot)
		jsonWrite(w, 200, storageJSON(s))
		return
	}
	if r.Method == "PATCH" {
		var v struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if decode(r, &v) != nil || strings.TrimSpace(v.Name) == "" {
			httpError(w, 400, "INVALID_REQUEST", "Name is required", nil)
			return
		}
		_, err := a.db.Exec("UPDATE storages SET name=?,description=? WHERE id=?", v.Name, v.Description, id)
		if err != nil {
			httpError(w, 500, "UPDATE_FAILED", err.Error(), nil)
			return
		}
		jsonWrite(w, 200, map[string]bool{"updated": true})
		return
	}
	httpError(w, 405, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
}
func rowStorage(row *sql.Row) (storage, error) {
	var s storage
	err := row.Scan(&s.ID, &s.Name, &s.Description, &s.VolumeLabel, &s.FilesystemType, &s.FilesystemID, &s.CapacityBytes, &s.LastScanRoot, &s.CurrentRoot, &s.FileCount, &s.TotalSize, &s.LastScanAt, &s.CreatedAt)
	return s, err
}
func (a *App) locate(w http.ResponseWriter, r *http.Request, id int64) {
	var v struct {
		Path string `json:"path"`
	}
	if decode(r, &v) != nil || v.Path == "" {
		httpError(w, 400, "INVALID_REQUEST", "Path is required", nil)
		return
	}
	if st, e := os.Stat(v.Path); e != nil || !st.IsDir() {
		httpError(w, 422, "DIRECTORY_NOT_FOUND", "Directory not found", nil)
		return
	}
	if _, e := a.db.Exec("UPDATE storages SET current_root=? WHERE id=?", filepath.Clean(v.Path), id); e != nil {
		httpError(w, 500, "LOCATE_FAILED", e.Error(), nil)
		return
	}
	jsonWrite(w, 200, map[string]any{"storage_id": id, "current_root": filepath.Clean(v.Path), "available": true})
}

func (a *App) scans(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		httpError(w, 405, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}
	var req scanRequest
	if decode(r, &req) != nil {
		httpError(w, 400, "INVALID_REQUEST", "Invalid scan request", nil)
		return
	}
	req.Mode = strings.ToLower(req.Mode)
	req.Storage.Name = strings.TrimSpace(req.Storage.Name)
	if (req.Mode != "new" && req.Mode != "rescan") || (req.Mode == "new" && (req.ScanRoot == "" || req.Storage.Name == "")) || (req.Mode == "rescan" && req.StorageID < 1) {
		httpError(w, 400, "INVALID_REQUEST", "Required scan fields are missing", nil)
		return
	}
	if req.Mode == "rescan" {
		err := a.db.QueryRow("SELECT COALESCE(NULLIF(current_root,''),last_scan_root), name FROM storages WHERE id=?", req.StorageID).Scan(&req.ScanRoot, &req.Storage.Name)
		if errors.Is(err, sql.ErrNoRows) {
			httpError(w, 404, "STORAGE_NOT_FOUND", "Storage not found", nil)
			return
		}
		if err != nil {
			httpError(w, 500, "QUERY_FAILED", err.Error(), nil)
			return
		}
		if st, err := os.Stat(req.ScanRoot); err != nil || !st.IsDir() {
			httpError(w, 422, "STORAGE_OFFLINE", "Source directory is unavailable. Reconnect the storage or use Relocate before rescanning.", nil)
			return
		}
	}
	root, e := filepath.Abs(req.ScanRoot)
	if e != nil {
		httpError(w, 422, "DIRECTORY_NOT_FOUND", "Scan root is not a directory", nil)
		return
	}
	req.ScanRoot = filepath.Clean(root)
	if st, e := os.Stat(req.ScanRoot); e != nil || !st.IsDir() {
		httpError(w, 422, "DIRECTORY_NOT_FOUND", "Scan root is not a directory", nil)
		return
	}
	a.mu.Lock()
	for _, j := range a.jobs {
		if j.Status == "running" || j.Status == "cancelling" {
			a.mu.Unlock()
			httpError(w, 409, "SCAN_IN_PROGRESS", "Another scan is already running", nil)
			return
		}
	}
	id := "scan-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	ctx, cancel := context.WithCancel(context.Background())
	j := &Job{ID: id, Status: "running", Mode: req.Mode, StorageID: req.StorageID, StorageName: req.Storage.Name, Root: req.ScanRoot, Started: time.Now(), cancel: cancel}
	a.jobs[id] = j
	a.mu.Unlock()
	go a.runScan(ctx, j, req)
	jsonWrite(w, 201, map[string]any{"job_id": id, "status": "running"})
}
func (a *App) updateJob(j *Job, fn func(*Job)) { a.mu.Lock(); fn(j); a.mu.Unlock() }
func (a *App) runScan(ctx context.Context, j *Job, req scanRequest) {
	a.dbMu.RLock()
	defer a.dbMu.RUnlock()
	table := "staging_" + strings.ReplaceAll(j.ID, "-", "_")
	defer func() { _, _ = a.db.Exec("DROP TABLE IF EXISTS " + table) }()
	if _, err := a.db.Exec("CREATE TABLE " + table + " (name TEXT,extension TEXT,relative_path TEXT,size_bytes INTEGER,modified_at TEXT)"); err != nil {
		a.finish(j, "failed", err)
		return
	}
	stageTx, err := a.db.Begin()
	if err != nil {
		a.finish(j, "failed", err)
		return
	}
	insert, err := stageTx.Prepare("INSERT INTO " + table + " VALUES(?,?,?,?,?)")
	if err != nil {
		_ = stageTx.Rollback()
		a.finish(j, "failed", err)
		return
	}
	err = filepath.WalkDir(j.Root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		select {
		case <-ctx.Done():
			return context.Canceled
		default:
		}
		if d.IsDir() {
			a.updateJob(j, func(j *Job) {
				if path != j.Root {
					j.DirectoriesScanned++
				}
				j.CurrentPath = path
			})
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(j.Root, filepath.Dir(path))
		if e != nil {
			return e
		}
		if rel == "." {
			rel = ""
		}
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(d.Name())), ".")
		_, e = insert.Exec(d.Name(), ext, filepath.ToSlash(rel), info.Size(), info.ModTime().UTC().Format(time.RFC3339))
		if e != nil {
			return e
		}
		a.updateJob(j, func(j *Job) { j.FilesScanned++; j.SizeBytes += info.Size() })
		return nil
	})
	_ = insert.Close()
	if err != nil {
		_ = stageTx.Rollback()
		if errors.Is(err, context.Canceled) {
			a.finish(j, "cancelled", nil)
		} else {
			a.finish(j, "failed", err)
		}
		return
	}
	if err = stageTx.Commit(); err != nil {
		a.finish(j, "failed", err)
		return
	}
	select {
	case <-ctx.Done():
		a.finish(j, "cancelled", nil)
		return
	default:
	}
	a.mu.RLock()
	files, size := j.FilesScanned, j.SizeBytes
	a.mu.RUnlock()
	tx, err := a.db.Begin()
	if err != nil {
		a.finish(j, "failed", err)
		return
	}
	defer tx.Rollback()
	var sid int64
	if req.Mode == "new" {
		res, e := tx.Exec("INSERT INTO storages(name,description,last_scan_root,current_root,file_count,total_size_bytes,last_scan_at,created_at) VALUES(?,?,?,?,?,?,?,?)", req.Storage.Name, req.Storage.Description, j.Root, j.Root, files, size, now(), now())
		if e != nil {
			a.finish(j, "failed", e)
			return
		}
		sid, e = res.LastInsertId()
		if e != nil {
			a.finish(j, "failed", e)
			return
		}
	} else {
		sid = req.StorageID
		result, e := tx.Exec("UPDATE storages SET last_scan_root=?,current_root=?,file_count=?,total_size_bytes=?,last_scan_at=? WHERE id=?", j.Root, j.Root, files, size, now(), sid)
		if e != nil {
			a.finish(j, "failed", e)
			return
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			a.finish(j, "failed", errors.New("storage no longer exists"))
			return
		}
		if _, err = tx.Exec("DELETE FROM files WHERE storage_id=?", sid); err != nil {
			a.finish(j, "failed", err)
			return
		}
	}
	select {
	case <-ctx.Done():
		a.finish(j, "cancelled", nil)
		return
	default:
	}
	_, err = tx.Exec("INSERT INTO files(storage_id,name,extension,relative_path,size_bytes,modified_at) SELECT ?,name,extension,relative_path,size_bytes,modified_at FROM "+table, sid)
	if err != nil {
		a.finish(j, "failed", err)
		return
	}
	if err = tx.Commit(); err != nil {
		a.finish(j, "failed", err)
		return
	}
	a.updateJob(j, func(j *Job) { j.StorageID = sid })
	a.finish(j, "completed", nil)
}
func (a *App) finish(j *Job, status string, err error) {
	a.mu.Lock()
	j.Status = status
	if err != nil {
		j.Err = err.Error()
	}
	a.mu.Unlock()
}
func (a *App) scanRoute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		httpError(w, 404, "NOT_FOUND", "Scan not found", nil)
		return
	}
	id := parts[3]
	a.mu.RLock()
	j, ok := a.jobs[id]
	a.mu.RUnlock()
	if !ok {
		httpError(w, 404, "SCAN_NOT_FOUND", "Scan not found", nil)
		return
	}
	if len(parts) == 5 && parts[4] == "cancel" {
		if r.Method != "POST" {
			httpError(w, 405, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
			return
		}
		a.mu.Lock()
		if j.cancel != nil && j.Status == "running" {
			j.Status = "cancelling"
			j.cancel()
		}
		a.mu.Unlock()
		jsonWrite(w, 200, map[string]string{"status": "cancelling"})
		return
	}
	if r.Method != "GET" {
		httpError(w, 405, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}
	a.mu.RLock()
	snapshot := *j
	elapsed := time.Since(j.Started).Seconds()
	a.mu.RUnlock()
	jsonWrite(w, 200, map[string]any{"job_id": snapshot.ID, "status": snapshot.Status, "mode": snapshot.Mode, "storage_id": snapshot.StorageID, "storage_name": snapshot.StorageName, "current_path": snapshot.CurrentPath, "files_scanned": snapshot.FilesScanned, "directories_scanned": snapshot.DirectoriesScanned, "size_bytes": snapshot.SizeBytes, "error_count": snapshot.ErrorCount, "elapsed_seconds": int64(elapsed), "error": snapshot.Err})
}

func (a *App) exports(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		httpError(w, 405, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}
	var req struct {
		Type  string `json:"type"`
		Query struct {
			Q         string `json:"q"`
			StorageID string `json:"storage_id"`
			Extension string `json:"extension"`
			Sort      string `json:"sort"`
			Order     string `json:"order"`
		} `json:"query"`
		StorageID2 int64 `json:"storage_id"`
	}
	if decode(r, &req) != nil {
		httpError(w, 400, "INVALID_REQUEST", "Invalid export request", nil)
		return
	}
	if req.Type != "search" && req.Type != "storage" {
		httpError(w, 400, "INVALID_REQUEST", "type must be search or storage", nil)
		return
	}
	if req.Type == "storage" && req.StorageID2 < 1 {
		httpError(w, 400, "INVALID_REQUEST", "storage_id is required", nil)
		return
	}
	if req.Query.StorageID != "" {
		if id, err := strconv.ParseInt(req.Query.StorageID, 10, 64); err != nil || id < 1 {
			httpError(w, 400, "INVALID_REQUEST", "Invalid storage_id", nil)
			return
		}
	}
	id := "export-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	job := &ExportJob{ID: id, Status: "running"}
	a.mu.Lock()
	a.exportJobs[id] = job
	a.mu.Unlock()
	go a.exportCSV(job, req.Type, req.Query.Q, req.Query.StorageID, req.Query.Extension, req.StorageID2, req.Query.Sort, req.Query.Order)
	jsonWrite(w, 201, map[string]string{"job_id": id, "status": "running"})
}
func (a *App) failExport(job *ExportJob, err error) {
	a.mu.Lock()
	job.Status = "failed"
	job.Err = err.Error()
	a.mu.Unlock()
}
func (a *App) exportCSV(job *ExportJob, typ, q, storageID, ext string, sid int64, sortKey, order string) {
	a.dbMu.RLock()
	defer a.dbMu.RUnlock()
	cfg := a.config()
	dir := resolve(a.root, cfg.ExportPath)
	if e := os.MkdirAll(dir, 0755); e != nil {
		a.failExport(job, e)
		return
	}
	prefix := "Search"
	if typ == "storage" {
		prefix = "Storage"
	}
	name := "FileIndex_" + prefix + "_" + time.Now().UTC().Format("20060102_150405.000000000") + ".csv"
	p := filepath.Join(dir, name)
	f, e := os.Create(p)
	if e != nil {
		a.failExport(job, e)
		return
	}
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(p)
		}
	}()
	if _, e = f.Write([]byte{0xEF, 0xBB, 0xBF}); e != nil {
		a.failExport(job, e)
		return
	}
	writer := csv.NewWriter(f)
	writer.UseCRLF = true
	if e = writer.Write([]string{"Storage Name", "Filename", "Extension", "Relative Path", "File Size", "Modified Time", "Scan Time"}); e != nil {
		a.failExport(job, e)
		return
	}
	where := []string{"1=1"}
	args := []any{}
	for _, t := range strings.Fields(q) {
		where = append(where, "(LOWER(f.name) LIKE LOWER(?) OR LOWER(f.relative_path) LIKE LOWER(?))")
		like := "%" + t + "%"
		args = append(args, like, like)
	}
	if storageID != "" {
		if v, e := strconv.ParseInt(storageID, 10, 64); e == nil {
			where = append(where, "f.storage_id=?")
			args = append(args, v)
		}
	} else if sid > 0 {
		where = append(where, "f.storage_id=?")
		args = append(args, sid)
	}
	if ext != "" {
		where = append(where, "LOWER(f.extension)=LOWER(?)")
		args = append(args, strings.TrimPrefix(ext, "."))
	}
	_, ord, orderExpression := fileOrder(sortKey, order)
	rows, e := a.db.Query("SELECT s.name,f.name,COALESCE(f.extension,''),f.relative_path,f.size_bytes,COALESCE(f.modified_at,''),COALESCE(s.last_scan_at,'') FROM files f JOIN storages s ON s.id=f.storage_id WHERE "+strings.Join(where, " AND ")+" ORDER BY "+orderExpression+", f.id "+ord, args...)
	if e != nil {
		a.failExport(job, e)
		return
	}
	defer rows.Close()
	var count int64
	for rows.Next() {
		var vals [7]string
		var size int64
		if e = rows.Scan(&vals[0], &vals[1], &vals[2], &vals[3], &size, &vals[5], &vals[6]); e != nil {
			a.failExport(job, e)
			return
		}
		vals[4] = strconv.FormatInt(size, 10)
		if e = writer.Write(vals[:]); e != nil {
			a.failExport(job, e)
			return
		}
		count++
	}
	if e = rows.Err(); e != nil {
		a.failExport(job, e)
		return
	}
	writer.Flush()
	if e = writer.Error(); e != nil {
		a.failExport(job, e)
		return
	}
	if e = f.Sync(); e != nil {
		a.failExport(job, e)
		return
	}
	ok = true
	a.mu.Lock()
	job.Status = "completed"
	job.Filename = name
	job.RelativePath = filepath.ToSlash(filepath.Join(cfg.ExportPath, name))
	job.RecordCount = count
	a.mu.Unlock()
}
func (a *App) exportRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		httpError(w, 405, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		httpError(w, 404, "EXPORT_NOT_FOUND", "Export not found", nil)
		return
	}
	a.mu.RLock()
	job, ok := a.exportJobs[parts[3]]
	if ok {
		snapshot := *job
		a.mu.RUnlock()
		jsonWrite(w, 200, snapshot)
		return
	}
	a.mu.RUnlock()
	httpError(w, 404, "EXPORT_NOT_FOUND", "Export not found", nil)
}

func (a *App) backups(w http.ResponseWriter, r *http.Request) {
	cfg := a.config()
	dir := resolve(a.root, cfg.BackupPath)
	_ = os.MkdirAll(dir, 0755)
	if r.Method == "POST" {
		name := "FileIndex_" + time.Now().UTC().Format("20060102_150405.000000000") + ".db"
		dest := filepath.Join(dir, name)
		q := strings.ReplaceAll(dest, "'", "''")
		if _, e := a.db.Exec("VACUUM INTO '" + q + "'"); e != nil {
			httpError(w, 500, "BACKUP_FAILED", e.Error(), nil)
			return
		}
		jsonWrite(w, 201, map[string]any{"filename": name, "relative_path": filepath.ToSlash(filepath.Join(cfg.BackupPath, name)), "schema_version": 1})
		return
	}
	entries, _ := os.ReadDir(dir)
	out := []any{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".db" {
			continue
		}
		info, _ := e.Info()
		out = append(out, map[string]any{"id": e.Name(), "filename": e.Name(), "size_bytes": info.Size(), "created_at": info.ModTime().UTC().Format(time.RFC3339), "schema_version": 1})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].(map[string]any)["filename"].(string) > out[j].(map[string]any)["filename"].(string)
	})
	jsonWrite(w, 200, out)
}
func (a *App) backupRoute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		httpError(w, 404, "NOT_FOUND", "Backup not found", nil)
		return
	}
	name := filepath.Base(parts[3])
	if name != parts[3] || filepath.Ext(name) != ".db" {
		httpError(w, 400, "INVALID_BACKUP_ID", "Invalid backup id", nil)
		return
	}
	p := filepath.Join(resolve(a.root, a.config().BackupPath), name)
	if len(parts) == 5 && parts[4] == "restore" {
		if r.Method != "POST" {
			httpError(w, 405, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
			return
		}
		if _, err := os.Stat(p); err != nil {
			httpError(w, 404, "BACKUP_NOT_FOUND", "Backup not found", nil)
			return
		}
		if err := a.restoreDatabase(p); err != nil {
			if err.Error() == "scan in progress" {
				httpError(w, 409, "SCAN_IN_PROGRESS", err.Error(), nil)
			} else {
				httpError(w, 422, "RESTORE_FAILED", err.Error(), nil)
			}
			return
		}
		jsonWrite(w, 200, map[string]bool{"restored": true})
		return
	}
	if r.Method == "DELETE" {
		if err := os.Remove(p); err != nil {
			httpError(w, 404, "BACKUP_NOT_FOUND", "Backup not found", nil)
			return
		}
		jsonWrite(w, 200, map[string]bool{"deleted": true})
		return
	}
	httpError(w, 405, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
}
func validateRestore(path string) error {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	var integrity string
	if err = db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return errors.New("database integrity check failed")
	}
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version != 1 {
		return fmt.Errorf("unsupported schema version %d", version)
	}
	return validateSchema(db)
}
func copyDatabaseFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = out.Close()
		if !ok {
			_ = os.Remove(dst)
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	if err = out.Sync(); err != nil {
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}
func (a *App) restoreDatabase(source string) error {
	if err := validateRestore(source); err != nil {
		return err
	}
	a.mu.Lock()
	for _, j := range a.jobs {
		if j.Status == "running" || j.Status == "cancelling" {
			a.mu.Unlock()
			return errors.New("scan in progress")
		}
	}
	a.maintenance = true
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.maintenance = false; a.mu.Unlock() }()
	a.dbMu.Lock()
	defer a.dbMu.Unlock()
	cfg := a.config()
	dbPath := a.dbPath
	backupDir := resolve(a.root, cfg.BackupPath)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return err
	}
	auto := filepath.Join(backupDir, "FileIndex_PreRestore_"+time.Now().UTC().Format("20060102_150405.000000000")+".db")
	quoted := strings.ReplaceAll(filepath.ToSlash(auto), "'", "''")
	if _, err := a.db.Exec("VACUUM INTO '" + quoted + "'"); err != nil {
		return fmt.Errorf("automatic backup failed: %w", err)
	}
	tmp := dbPath + ".restore-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := copyDatabaseFile(source, tmp); err != nil {
		return err
	}
	if err := validateRestore(tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := a.db.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	rollback := dbPath + ".rollback-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.Rename(dbPath, rollback); err != nil {
		_ = os.Remove(tmp)
		a.db, _ = openDatabase(dbPath)
		return err
	}
	if err := os.Rename(tmp, dbPath); err != nil {
		_ = os.Rename(rollback, dbPath)
		a.db, _ = openDatabase(dbPath)
		return err
	}
	newDB, err := openDatabase(dbPath)
	if err != nil {
		_ = os.Remove(dbPath)
		_ = os.Rename(rollback, dbPath)
		a.db, _ = openDatabase(dbPath)
		return err
	}
	a.db = newDB
	_ = os.Remove(rollback)
	return nil
}
func (a *App) settings(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		jsonWrite(w, 200, a.config())
		return
	}
	if r.Method == "PUT" {
		var c Config
		if decode(r, &c) != nil || validateConfig(a.root, c) != nil {
			httpError(w, 400, "INVALID_SETTINGS", "Invalid settings", nil)
			return
		}
		if err := saveConfig(a.root, &c); err != nil {
			httpError(w, 500, "SETTINGS_SAVE_FAILED", err.Error(), nil)
			return
		}
		a.mu.Lock()
		a.cfg = c
		a.mu.Unlock()
		jsonWrite(w, 200, c)
		return
	}
	httpError(w, 405, "METHOD_NOT_ALLOWED", "Method not allowed", nil)
}
func openBrowser(url string) {
	time.Sleep(200 * time.Millisecond)
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	} else if runtime.GOOS == "darwin" {
		cmd = exec.Command("open", url)
	} else {
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
