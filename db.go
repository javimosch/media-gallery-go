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

func (d *DB) Browse(category, yearMonth, cursor string, limit int) (*BrowseResult, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var conditions []string
	var args []interface{}

	if category != "" {
		conditions = append(conditions, "relpath LIKE ? || '/%'")
		args = append(args, category)
	}
	if yearMonth != "" {
		conditions = append(conditions, "relpath LIKE '%/' || ? || '/%'")
		args = append(args, yearMonth)
	}
	if cursor != "" {
		conditions = append(conditions, "relpath > ?")
		args = append(args, cursor)
	}

	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	// total count
	totalQuery := "SELECT COUNT(*) FROM files" + wherePathOnly(where, category, yearMonth)
	var total int
	d.conn.QueryRow(totalQuery, args[:len(args)-maybeCursor(cursor)]...).Scan(&total)

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

func wherePathOnly(where, category, yearMonth string) string {
	// strip cursor condition for total count
	conds := []string{}
	if category != "" {
		conds = append(conds, "relpath LIKE '"+category+"/%'")
	}
	if yearMonth != "" {
		conds = append(conds, "relpath LIKE '%/"+yearMonth+"/%'")
	}
	if len(conds) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(conds, " AND ")
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
		"SELECT relpath, size, mtime, COALESCE(sha256,''), COALESCE(scanned_at,0) FROM files WHERE relpath LIKE ? ORDER BY relpath LIMIT ?",
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
		 FROM files WHERE sha256 IS NOT NULL AND sha256 != ''
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
		"SELECT relpath, size, mtime, COALESCE(sha256,''), COALESCE(scanned_at,0) FROM files WHERE sha256 = ? ORDER BY relpath",
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

	d.conn.QueryRow("SELECT COUNT(*), COALESCE(SUM(size),0) FROM files").Scan(&s.TotalFiles, &s.TotalSize)
	d.conn.QueryRow("SELECT COUNT(*), COALESCE(SUM(s),0) FROM (SELECT sha256, COUNT(*) c, SUM(size) s FROM files WHERE sha256 != '' GROUP BY sha256 HAVING c > 1)").Scan(&s.DupeGroups, &s.DupeSize)
	d.conn.QueryRow("SELECT COALESCE(SUM(c),0) FROM (SELECT COUNT(*) c FROM files WHERE sha256 != '' GROUP BY sha256 HAVING c > 1)").Scan(&s.DupeFiles)

	// misfiled from flagged_dupes (if table exists)
	d.conn.QueryRow("SELECT COUNT(*), COALESCE(SUM(size),0) FROM flagged_dupes WHERE reason='misfiled'").Scan(&s.MisfiledCount, &s.MisfiledSize)

	// categories
	rows, err := d.conn.Query(
		`SELECT substr(relpath, 1, instr(relpath, '/') - 1) AS cat, COUNT(*), SUM(size)
		 FROM files WHERE instr(relpath, '/') > 0 AND substr(relpath, 1, instr(relpath, '/') - 1) != ''
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
		"SELECT DISTINCT substr(relpath, 1, instr(relpath, '/') - 1) FROM files WHERE instr(relpath, '/') > 0 AND substr(relpath, 1, instr(relpath, '/') - 1) != '' ORDER BY 1")
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
		WHERE substr(relpath, instr(relpath, '/') + 1, 7) GLOB '20[0-9][0-9]-[0-1][0-9]'`
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
