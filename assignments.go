package main

// Assignments give files a virtual folder and month without moving them. They
// live in an optional table, keyed by content so every identical copy follows:
//
//	assignments(sha256 TEXT PRIMARY KEY, folder TEXT NOT NULL, month TEXT NOT NULL,
//	            rule TEXT, batch TEXT, assigned_at INTEGER)
//
// The gallery only reads it. Deleting rows restores the original grouping.
//
// Listings read from "items": relpath stays the real file, vpath is where it is
// shown, and shown is 0 for the extra copies of an assigned file.
const plainItems = `(SELECT relpath, size, mtime, sha256, scanned_at,
	relpath AS vpath, 0 AS assigned, 1 AS shown FROM files)`

const assignedItems = `(SELECT f.relpath AS relpath, f.size AS size, f.mtime AS mtime,
	f.sha256 AS sha256, f.scanned_at AS scanned_at,
	CASE WHEN a.sha256 IS NULL THEN f.relpath ELSE
		a.folder || '/' || a.month || '/' || replace(f.relpath, rtrim(f.relpath, replace(f.relpath, '/', '')), '')
	END AS vpath,
	a.sha256 IS NOT NULL AS assigned,
	CASE WHEN a.sha256 IS NULL THEN 1 ELSE
		f.relpath = (SELECT min(c.relpath) FROM files c WHERE c.sha256 = f.sha256 AND visible(c.relpath))
	END AS shown
	FROM files f LEFT JOIN assignments a ON a.sha256 = f.sha256)`

// detectAssignments picks the listing source. It runs when the DB is opened.
func (d *DB) detectAssignments() error {
	var n int
	if err := d.conn.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='assignments'").Scan(&n); err != nil {
		return err
	}
	var dates int
	if err := d.conn.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='media_dates'").Scan(&dates); err != nil {
		return err
	}
	inner := plainItems
	d.hasAssignments = n > 0
	if d.hasAssignments {
		inner = assignedItems
	}
	d.items = datedItems(inner, dates > 0)
	return nil
}

// GalleryMonth is the latest month any copy of the content is shown under.
func (d *DB) GalleryMonth(sha string) string {
	var m string
	d.conn.QueryRow("SELECT COALESCE(MAX(month), '') FROM "+d.items+" WHERE sha256 = ?", sha).Scan(&m)
	return m
}

func (d *DB) IsAssigned(sha string) bool {
	if !d.hasAssignments {
		return false
	}
	var n int
	d.conn.QueryRow("SELECT COUNT(*) FROM assignments WHERE sha256 = ?", sha).Scan(&n)
	return n > 0
}
