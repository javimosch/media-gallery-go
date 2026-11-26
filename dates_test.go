package main

import (
	"database/sql"
	"strings"
	"testing"
)

func TestMediaDate(t *testing.T) {
	for _, c := range [][4]string{
		{"a/2023-08/IMG_20221130_165706.jpg", "", "", "2022-11-30 16:57:06"},
		{"a/IMG-20230425-WA0009.jpg", "", "", "2023-04-25 00:00:00"},
		{"a/signal-2024-12-23-19-19-27-196.jpg", "", "", "2024-12-23 19:19:27"},
		{"a/IMG_20221130.jpg", "2015-07-27 10:00:00", "exif", "2015-07-27 10:00:00"},
		{"a/VID_20221130_235900.mp4", "2022-12-01 00:59:00", "video", "2022-11-30 23:59:00"},
		{"a/MVI_1.mp4", "2014-01-02 03:04:05", "video", "2014-01-02 03:04:05"},
		{"a/IMG_1.jpg", "1979-12-31 00:00:00", "exif", ""},
		{"a/1234567820221130.jpg", "", "", ""},
		{"a/IMG_20230231_000000.jpg", "", "", ""},
		{"20221130/IMG_1.jpg", "", "", ""},
	} {
		if got := mediaDate(c[0], c[1], c[2]); got != c[3] {
			t.Errorf("%s: got %q want %q", c[0], got, c[3])
		}
	}
}

func TestNewestFirstAndJunkInclude(t *testing.T) {
	path := t.TempDir() + "/n.db"
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(`
		CREATE TABLE files (relpath TEXT PRIMARY KEY, size INTEGER, mtime INTEGER, sha256 TEXT, scanned_at INTEGER);
		CREATE TABLE media_dates (sha256 TEXT PRIMARY KEY, taken TEXT, source TEXT);
		INSERT INTO files VALUES
			('a/2023-08/IMG_20220105_100000.jpg', 2000000, 0, 's1', 0),
			('a/2020-01/x.jpg', 2000000, 0, 's2', 0),
			('b/2019-03/y.jpg', 2000000, 0, 's3', 0),
			('b/2022-01/IMG_20220120_090000.jpg', 10, 0, 's4', 0),
			('b/misc/z.mp4', 10, 0, 's5', 0),
			('c/2022-01/VID_20220105_100001.mp4', 10, 0, 's6', 0);
		INSERT INTO media_dates VALUES ('s2','2024-05-01 08:00:00','exif');`)
	conn.Close()
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	names := func(r *BrowseResult) string {
		var out []string
		for _, f := range r.Files {
			out = append(out, f.SHA256)
		}
		return strings.Join(out, " ")
	}
	browse := func(ym, cursor string, limit int, hideJunk bool, m MediaFilter) *BrowseResult {
		t.Helper()
		r, err := db.Browse("", ym, cursor, limit, hideJunk, false, nil, m)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	var all []string
	cursor := ""
	for page := 0; page < 10; page++ {
		r := browse("", cursor, 2, false, MediaFilter{})
		all = append(all, names(r))
		if cursor = r.NextCursor; cursor == "" {
			break
		}
	}
	if got := strings.Join(all, " "); got != "s2 s4 s6 s1 s3 s5" {
		t.Errorf("newest first across pages: %s", got)
	}
	if got := names(browse("2022-01", "", 10, false, MediaFilter{})); got != "s4 s6 s1" {
		t.Errorf("month follows the file date, not the folder: %s", got)
	}
	yms, _ := db.YearMonths("")
	if got := strings.Join(yms, " "); got != "2024-05 2022-01 2019-03" {
		t.Errorf("months: %s", got)
	}
	for name, c := range map[string]struct {
		hide bool
		m    MediaFilter
		want string
	}{
		"junk hidden":          {true, MediaFilter{}, "s2 s6 s1 s3 s5"},
		"junk added":           {false, MediaFilter{IncludeJunk: true}, "s2 s4 s6 s1 s3 s5"},
		"junk only":            {false, MediaFilter{IncludeJunk: true, HidePhotos: true, HideVideos: true}, "s4"},
		"junk and videos":      {false, MediaFilter{IncludeJunk: true, HidePhotos: true}, "s4 s6 s5"},
		"photos and junk":      {false, MediaFilter{IncludeJunk: true, HideVideos: true}, "s2 s4 s1 s3"},
		"nothing checked":      {true, MediaFilter{HidePhotos: true, HideVideos: true}, ""},
		"videos, junk hidden": {true, MediaFilter{HidePhotos: true}, "s6 s5"},
	} {
		if got := names(browse("", "", 10, c.hide, c.m)); got != c.want {
			t.Errorf("%s: got %q want %q", name, got, c.want)
		}
	}
}
