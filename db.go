package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

type FileRecord struct {
	RelPath   string `json:"relpath"`
	Size      int64  `json:"size"`
	Mtime     int64  `json:"mtime"`
	SHA256    string `json:"sha256"`
	ScannedAt int64  `json:"scanned_at"`
	Source    string `json:"source,omitempty"` // "local" | "replica" (set by handlers)
}

type BrowseResult struct {
	Files     []FileRecord `json:"files"`
	NextCursor string      `json:"next_cursor"`
	Total     int          `json:"total"`
}

type DupeGroup struct {
	SHA256  string       `json:"sha256"`
	Count   int          `json:"count"`
	Size    int64        `json:"size"`
	Files   []FileRecord `json:"files"`
}

type Stats struct {
	TotalFiles   int   `json:"total_files"`
	TotalSize    int64 `json:"total_size"`
	DupeGroups   int   `json:"dupe_groups"`
	DupeFiles    int   `json:"dupe_files"`
	DupeSize     int64 `json:"dupe_size"`
	MisfiledCount int  `json:"misfiled_count"`
	MisfiledSize int64 `json:"misfiled_size"`
	HiddenFiles  int   `json:"hidden_files"`
	Categories   []CategoryStat `json:"categories"`
}

type CategoryStat struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
	Size     int64  `json:"size"`
}

func OpenDB(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	conn.SetMaxOpenConns(4)
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return &DB{conn: conn}, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

// MediaFilter excludes photos and/or videos from a listing. The zero value
// shows everything, including files that are neither.
type MediaFilter struct {
	HidePhotos bool
	HideVideos bool
	JunkOnly   bool // list only what the junk filter would hide
}

var (
	photoExts = []string{"jpg", "jpeg", "png", "gif", "bmp", "tif", "tiff", "webp", "heic"}
	videoExts = []string{"mp4", "mov", "avi", "mkv", "wmv", "3gp"}
)

func extCondition(exts []string) string {
	parts := make([]string, len(exts))
	for i, e := range exts {
		parts[i] = "lower(relpath) LIKE '%." + e + "'"
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

func (m MediaFilter) condition() string {
	switch {
	case m.HidePhotos && m.HideVideos:
		return "1=0"
	case m.HidePhotos:
		return extCondition(videoExts)
	case m.HideVideos:
		return extCondition(photoExts)
	}
	return ""
}

func (d *DB) Browse(category, yearMonth, cursor string, limit int, hideJunk bool, hasFace bool, faceDB *FaceDB, media ...MediaFilter) (*BrowseResult, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var conditions []string
	var args []interface{}
	conditions = append(conditions, "visible(relpath)")

	if category != "" {
		conditions = append(conditions, "relpath LIKE ? || '/%'")
		args = append(args, category)
	}
	if yearMonth != "" {
		conditions = append(conditions, "relpath LIKE '%/' || ? || '/%'")
		args = append(args, yearMonth)
	}
	for _, m := range media {
		if c := m.condition(); c != "" {
			conditions = append(conditions, c)
		}
	}
	// Junk filter: hide images < 50KB, or 50-1000KB images without a detected face.
	// Videos and images >= 1000KB are never junk.
	faceSHAs := ""
	if faceDB != nil && (hideJunk || hasFace || (len(media) > 0 && media[0].JunkOnly)) {
		faceSHAs = faceDB.faceSHASet()
	}

	junkOnly := len(media) > 0 && media[0].JunkOnly
	if hideJunk || junkOnly {
		keep := `(
			NOT (lower(relpath) LIKE '%.jpg' OR lower(relpath) LIKE '%.jpeg' OR lower(relpath) LIKE '%.png'
				OR lower(relpath) LIKE '%.gif' OR lower(relpath) LIKE '%.bmp' OR lower(relpath) LIKE '%.tif' OR lower(relpath) LIKE '%.tiff')
			OR size >= 51200
		)`
		if faceSHAs != "" {
			keep = `(
			NOT (lower(relpath) LIKE '%.jpg' OR lower(relpath) LIKE '%.jpeg' OR lower(relpath) LIKE '%.png'
				OR lower(relpath) LIKE '%.gif' OR lower(relpath) LIKE '%.bmp' OR lower(relpath) LIKE '%.tif' OR lower(relpath) LIKE '%.tiff')
			OR size >= 1024000
			OR (size >= 51200 AND sha256 IN (` + faceSHAs + `))
		)`
		}
		if junkOnly {
			keep = "NOT COALESCE(" + keep + ", 0)"
		}
		conditions = append(conditions, keep)
	}

	// Has-face filter: only show files whose sha256 has at least one detected face
	if hasFace {
		if faceSHAs != "" {
			conditions = append(conditions, "sha256 IN ("+faceSHAs+")")
		} else {
			// No faces DB — filter matches nothing
			conditions = append(conditions, "1=0")
		}
	}

	// total count: every filter except the cursor
	var total int
	d.conn.QueryRow("SELECT COUNT(*) FROM files WHERE "+strings.Join(conditions, " AND "), args...).Scan(&total)

	if cursor != "" {
		conditions = append(conditions, "relpath > ?")
		args = append(args, cursor)
	}
	where := " WHERE " + strings.Join(conditions, " AND ")

	query := "SELECT relpath, size, mtime, COALESCE(sha256,''), COALESCE(scanned_at,0) FROM files" +
		where + " ORDER BY relpath LIMIT ?"
	args = append(args, limit+1)

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("browse query: %w", err)
	}
	defer rows.Close()

	var files []FileRecord
	for rows.Next() {
		var f FileRecord
		rows.Scan(&f.RelPath, &f.Size, &f.Mtime, &f.SHA256, &f.ScannedAt)
		files = append(files, f)
	}

	result := &BrowseResult{Total: total}
	if len(files) > limit {
		result.NextCursor = files[limit-1].RelPath
		files = files[:limit]
	}
	result.Files = files
	return result, nil
}

func maybeCursor(cursor string) int {
	if cursor != "" {
		return 1
	}
	return 0
}

func (d *DB) Search(query string, limit int) ([]FileRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := d.conn.Query(
		"SELECT relpath, size, mtime, COALESCE(sha256,''), COALESCE(scanned_at,0) FROM files WHERE visible(relpath) AND relpath LIKE ? ORDER BY relpath LIMIT ?",
		"%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []FileRecord
	for rows.Next() {
		var f FileRecord
		rows.Scan(&f.RelPath, &f.Size, &f.Mtime, &f.SHA256, &f.ScannedAt)
		files = append(files, f)
	}
	return files, nil
}

func (d *DB) GetBySHA(sha string) (*FileRecord, error) {
	var f FileRecord
	err := d.conn.QueryRow(
		"SELECT relpath, size, mtime, COALESCE(sha256,''), COALESCE(scanned_at,0) FROM files WHERE sha256 = ? LIMIT 1",
		sha).Scan(&f.RelPath, &f.Size, &f.Mtime, &f.SHA256, &f.ScannedAt)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (d *DB) GetByRelPath(relpath string) (*FileRecord, error) {
	var f FileRecord
	err := d.conn.QueryRow(
		"SELECT relpath, size, mtime, COALESCE(sha256,''), COALESCE(scanned_at,0) FROM files WHERE relpath = ?",
		relpath).Scan(&f.RelPath, &f.Size, &f.Mtime, &f.SHA256, &f.ScannedAt)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (d *DB) DupeGroups(limit, offset int) ([]DupeGroup, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := d.conn.Query(
		`SELECT sha256, COUNT(*) as cnt, SUM(size) as total_size
		 FROM files WHERE visible(relpath) AND sha256 IS NOT NULL AND sha256 != ''
		 GROUP BY sha256 HAVING cnt > 1
		 ORDER BY total_size DESC LIMIT ? OFFSET ?`,
		limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []DupeGroup
	for rows.Next() {
		var g DupeGroup
		rows.Scan(&g.SHA256, &g.Count, &g.Size)
		groups = append(groups, g)
	}
	return groups, nil
}

func (d *DB) DupeFiles(sha string) ([]FileRecord, error) {
	rows, err := d.conn.Query(
		"SELECT relpath, size, mtime, COALESCE(sha256,''), COALESCE(scanned_at,0) FROM files WHERE visible(relpath) AND sha256 = ? ORDER BY relpath",
		sha)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []FileRecord
	for rows.Next() {
		var f FileRecord
		rows.Scan(&f.RelPath, &f.Size, &f.Mtime, &f.SHA256, &f.ScannedAt)
		files = append(files, f)
	}
	return files, nil
}

func (d *DB) GetStats() (*Stats, error) {
	s := &Stats{}

	d.conn.QueryRow("SELECT COUNT(*), COALESCE(SUM(size),0) FROM files WHERE visible(relpath)").Scan(&s.TotalFiles, &s.TotalSize)
	d.conn.QueryRow("SELECT COUNT(*), COALESCE(SUM(s),0) FROM (SELECT sha256, COUNT(*) c, SUM(size) s FROM files WHERE visible(relpath) AND sha256 != '' GROUP BY sha256 HAVING c > 1)").Scan(&s.DupeGroups, &s.DupeSize)
	d.conn.QueryRow("SELECT COALESCE(SUM(c),0) FROM (SELECT COUNT(*) c FROM files WHERE visible(relpath) AND sha256 != '' GROUP BY sha256 HAVING c > 1)").Scan(&s.DupeFiles)

	// misfiled from flagged_dupes (if table exists)
	d.conn.QueryRow("SELECT COUNT(*), COALESCE(SUM(size),0) FROM flagged_dupes WHERE visible(relpath) AND reason='misfiled'").Scan(&s.MisfiledCount, &s.MisfiledSize)

	// categories
	rows, err := d.conn.Query(
		`SELECT substr(relpath, 1, instr(relpath, '/') - 1) AS cat, COUNT(*), SUM(size)
		 FROM files WHERE visible(relpath) AND instr(relpath, '/') > 0 AND substr(relpath, 1, instr(relpath, '/') - 1) != ''
		 GROUP BY cat ORDER BY SUM(size) DESC`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var c CategoryStat
			rows.Scan(&c.Category, &c.Count, &c.Size)
			s.Categories = append(s.Categories, c)
		}
	}

	return s, nil
}

func (d *DB) Categories() ([]string, error) {
	rows, err := d.conn.Query(
		"SELECT DISTINCT substr(relpath, 1, instr(relpath, '/') - 1) FROM files WHERE visible(relpath) AND instr(relpath, '/') > 0 AND substr(relpath, 1, instr(relpath, '/') - 1) != '' ORDER BY 1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []string
	for rows.Next() {
		var c string
		rows.Scan(&c)
		cats = append(cats, c)
	}
	return cats, nil
}

func (d *DB) YearMonths(category string) ([]string, error) {
	// Extract the segment after the first '/', filter to valid YYYY-MM format only
	query := `SELECT DISTINCT substr(relpath, instr(relpath, '/') + 1, 7) AS ym
		FROM files
		WHERE visible(relpath) AND substr(relpath, instr(relpath, '/') + 1, 7) GLOB '20[0-9][0-9]-[0-1][0-9]'`
	var args []interface{}
	if category != "" {
		query += ` AND relpath LIKE ? || '/%'`
		args = append(args, category)
	}
	query += " ORDER BY 1 DESC"

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var yms []string
	for rows.Next() {
		var ym string
		rows.Scan(&ym)
		yms = append(yms, ym)
	}
	return yms, nil
}

func (d *DB) UniqueSHA256Count() (int, error) {
	var count int
	err := d.conn.QueryRow("SELECT COUNT(DISTINCT sha256) FROM files WHERE sha256 != ''").Scan(&count)
	return count, err
}

// AllRelPaths returns every indexed relpath, hidden ones included.
func (d *DB) AllRelPaths() ([]string, error) {
	rows, err := d.conn.Query("SELECT relpath FROM files")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d *DB) ScanProgress() (int, int) {
	var done int
	var total int
	d.conn.QueryRow("SELECT COUNT(*) FROM files WHERE sha256 IS NOT NULL AND sha256 != ''").Scan(&done)
	d.conn.QueryRow("SELECT COUNT(*) FROM files").Scan(&total)
	return done, total
}

func ts() string {
	return time.Now().UTC().Format(time.RFC3339)
}
