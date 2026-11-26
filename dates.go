package main

import (
	"database/sql/driver"
	"regexp"
	"sync"
	"time"

	"modernc.org/sqlite"
)

// A file's date is, in order: the capture date recorded in media_dates (EXIF,
// filled by scripts/media_info.py), a date in its filename, then the video
// creation time. Its gallery month comes from that date, and only without one
// from the YYYY-MM folder. File modification times are not used: copying changes
// them.
//
//	media_dates(sha256 TEXT PRIMARY KEY, taken TEXT, source TEXT)
//	  taken is "YYYY-MM-DD HH:MM:SS" or ""; source is "exif", "video" or ""

var nameDate = regexp.MustCompile(`(?:^|[^0-9])((?:19|20)\d\d)([-_]?)(0[1-9]|1[0-2])([-_]?)(0[1-9]|[12]\d|3[01])(?:[-_ T.]?([01]\d|2[0-3])[-_.h]?([0-5]\d)[-_.m]?([0-5]\d))?`)

var nameDates sync.Map // filename -> date, parsing is repeated per query

// plausibleDate checks the leading "YYYY-MM-DD": a real calendar day, from 1998
// up to about now. It runs several times per row, so it avoids time.Parse.
func plausibleDate(s string) bool {
	if len(s) < 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	num := func(a, b int) int {
		n := 0
		for i := a; i < b; i++ {
			if s[i] < '0' || s[i] > '9' {
				return -1
			}
			n = n*10 + int(s[i]-'0')
		}
		return n
	}
	y, m, d := num(0, 4), num(5, 7), num(8, 10)
	if y < 1998 || m < 1 || m > 12 || d < 1 || s[:10] > latestDate {
		return false
	}
	days := [...]int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}[m-1]
	if m == 2 && y%4 == 0 && (y%100 != 0 || y%400 == 0) {
		days = 29
	}
	return d <= days
}

// latestDate is computed at startup; the margin outlasts long-running servers.
var latestDate = time.Now().AddDate(1, 0, 0).Format("2006-01-02")

func filenameDate(relpath string) string {
	name := relpath
	for i := len(relpath) - 1; i >= 0; i-- {
		if relpath[i] == '/' {
			name = relpath[i+1:]
			break
		}
	}
	if v, ok := nameDates.Load(name); ok {
		return v.(string)
	}
	out := ""
	if m := nameDate.FindStringSubmatch(name); m != nil {
		d := m[1] + "-" + m[3] + "-" + m[5]
		if plausibleDate(d) {
			clock := "00:00:00"
			if m[6] != "" {
				clock = m[6] + ":" + m[7] + ":" + m[8]
			}
			out = d + " " + clock
		}
	}
	nameDates.Store(name, out)
	return out
}

func mediaDate(relpath, taken, source string) string {
	if source == "exif" && plausibleDate(taken) {
		return taken
	}
	if d := filenameDate(relpath); d != "" {
		return d
	}
	if plausibleDate(taken) {
		return taken
	}
	return ""
}

// media_date(relpath, taken, source) returns "" when no date is known.
func init() {
	sqlite.MustRegisterDeterministicScalarFunction("media_date", 3,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			p, _ := args[0].(string)
			t, _ := args[1].(string)
			s, _ := args[2].(string)
			return mediaDate(p, t, s), nil
		})
}

// datedItems adds "taken" and "month" to a listing source.
func datedItems(inner string, hasTable bool) string {
	join, taken, source := "", "''", "''"
	if hasTable {
		join, taken, source = " LEFT JOIN media_dates md ON md.sha256 = i.sha256", "md.taken", "md.source"
	}
	return `(SELECT d.*, CASE
			WHEN d.taken <> '' THEN substr(d.taken, 1, 7)
			WHEN substr(d.vpath, instr(d.vpath, '/') + 1, 8) GLOB '[12][0-9][0-9][0-9]-[01][0-9]/'
				THEN substr(d.vpath, instr(d.vpath, '/') + 1, 7)
			ELSE '' END AS month
		FROM (SELECT i.relpath, i.size, i.mtime, i.sha256, i.scanned_at, i.vpath, i.assigned, i.shown,
			media_date(i.relpath, ` + taken + `, ` + source + `) AS taken
			FROM ` + inner + ` AS i` + join + `) AS d) AS items`
}
