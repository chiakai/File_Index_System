package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestVolumeInfoAndScanPersistence(t *testing.T) {
	app := testApp(t)
	root := filepath.Join(app.root, "source")
	info := readVolumeInfo(root)
	if info.Filesystem == "" || info.Capacity == nil || *info.Capacity <= 0 {
		t.Fatalf("missing local volume metadata: %+v", info)
	}
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	child := readVolumeInfo(nested)
	if child.Filesystem != info.Filesystem || child.Capacity == nil || *child.Capacity != *info.Capacity {
		t.Fatalf("nested directory uses different volume: %+v", child)
	}
	job := &Job{ID: "volume-test", Status: "running", Root: root}
	req := scanRequest{Mode: "new", ScanRoot: root}
	req.Storage.Name = "volume"
	app.runScan(context.Background(), job, req)
	if job.Status != "completed" {
		t.Fatalf("scan: %s %s", job.Status, job.Err)
	}
	var filesystem string
	var capacity int64
	if err := app.db.QueryRow("SELECT filesystem_type,capacity_bytes FROM storages WHERE id=?", job.StorageID).Scan(&filesystem, &capacity); err != nil {
		t.Fatal(err)
	}
	if filesystem != info.Filesystem || capacity != *info.Capacity {
		t.Fatalf("metadata not saved: %s %d", filesystem, capacity)
	}
}
