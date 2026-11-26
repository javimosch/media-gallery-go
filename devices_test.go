package main

import (
	"database/sql"
	"testing"
)

func TestDeviceFilter(t *testing.T) {
	path := t.TempDir() + "/d.db"
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(`
		CREATE TABLE files (relpath TEXT PRIMARY KEY, size INTEGER, mtime INTEGER, sha256 TEXT, scanned_at INTEGER);
		CREATE TABLE media_info (sha256 TEXT PRIMARY KEY, make TEXT, model TEXT, status TEXT, scanned_at INTEGER);
		CREATE TABLE devices (model TEXT PRIMARY KEY, label TEXT, type TEXT);
		INSERT INTO files VALUES
			('a/2020-01/1.jpg', 10, 0, 's1', 0), ('a/2020-01/2.jpg', 2000000, 0, 's2', 0),
			('a/2020-02/3.jpg', 2000000, 0, 's3', 0), ('b/2020-01/4.mp4', 2000000, 0, 's4', 0),
			('b/2020-01/5.png', 2000000, 0, NULL, 0);
		INSERT INTO media_info VALUES ('s1','m','X1','ok',0), ('s2','m','X1','ok',0), ('s3','','','ok',0), ('s4','m','Y2','ok',0);
		INSERT INTO devices VALUES ('X1','Nice name','phone');`)
	conn.Close()
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	total := func(hideJunk bool, m MediaFilter) int {
		t.Helper()
		r, err := db.Browse("", "", "", 10, hideJunk, false, nil, m)
		if err != nil {
			t.Fatal(err)
		}
		return r.Total
	}
	unknown := deviceFromParam(unknownDeviceParam)
	for name, c := range map[string][2]int{
		"model":            {total(false, MediaFilter{Device: "X1"}), 2},
		"model, junk off":  {total(true, MediaFilter{Device: "X1"}), 1},
		"model, junk only": {total(false, MediaFilter{Device: "X1", JunkOnly: true}), 1},
		"unknown":          {total(false, MediaFilter{Device: unknown}), 2},
		"name and device":  {total(true, MediaFilter{Device: "X1", Name: "1.jpg"}), 1},
		"injection":        {total(false, MediaFilter{Device: "x' OR 1=1 --"}), 0},
	} {
		if c[0] != c[1] {
			t.Errorf("%s: got %d want %d", name, c[0], c[1])
		}
	}
	got, err := db.DeviceCounts("", "a", "")
	if err != nil {
		t.Fatal(err)
	}
	want := []DeviceCount{{"X1", "Nice name", "phone", 2}, {unknownDeviceParam, "Unknown device", "unknown", 1}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("counts %+v", got)
	}
	got, _ = db.DeviceCounts("", "b", "2020-01")
	if len(got) != 2 || got[0].Label != "Y2" || got[0].Type != "other" {
		t.Errorf("unlabelled model %+v", got)
	}
}
