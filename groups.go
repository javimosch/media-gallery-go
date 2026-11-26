package main

import (
	"database/sql/driver"
	"fmt"
	"path"
	"sort"
	"strings"

	"modernc.org/sqlite"
)

// Folder groups are a view over top-level folders, set once at startup with
// -groups, e.g. "Cameras=cam-*;Other=*". The first matching rule wins. Nothing
// moves on disk, and this is not access control.
type groupRule struct {
	name     string
	patterns []string
}

var groupRules []groupRule

func configureGroups(spec string) error {
	var rules []groupRule
	for _, part := range strings.Split(spec, ";") {
		if part = strings.TrimSpace(part); part == "" {
			continue
		}
		name, list, ok := strings.Cut(part, "=")
		r := groupRule{name: strings.TrimSpace(name)}
		for _, p := range strings.Split(list, ",") {
			if p = strings.TrimSpace(p); p == "" {
				continue
			}
			if _, err := path.Match(p, ""); err != nil {
				return fmt.Errorf("-groups: bad pattern %q", p)
			}
			r.patterns = append(r.patterns, p)
		}
		if !ok || r.name == "" || len(r.patterns) == 0 {
			return fmt.Errorf("-groups: want Name=pattern[,pattern], got %q", part)
		}
		rules = append(rules, r)
	}
	groupRules = rules
	return nil
}

func groupOfFolder(folder string) string {
	for _, r := range groupRules {
		for _, p := range r.patterns {
			if ok, _ := path.Match(p, folder); ok {
				return r.name
			}
		}
	}
	return ""
}

// folder_group(relpath) is "" for files outside any folder or rule.
func init() {
	sqlite.MustRegisterDeterministicScalarFunction("folder_group", 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			p, _ := args[0].(string)
			if i := strings.Index(p, "/"); i > 0 {
				return groupOfFolder(p[:i]), nil
			}
			return "", nil
		})
}

type FolderCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type FolderGroup struct {
	Name    string        `json:"name"`
	Count   int           `json:"count"`
	Folders []FolderCount `json:"folders"`
}

// FolderCounts returns visible file counts per top-level folder ("" = files
// outside any folder).
func (d *DB) FolderCounts() ([]FolderCount, error) {
	rows, err := d.conn.Query(`SELECT substr(vpath, 1, max(instr(vpath, '/') - 1, 0)), COUNT(*)
		FROM ` + d.items + ` WHERE visible(relpath) AND shown GROUP BY 1 ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FolderCount
	for rows.Next() {
		var f FolderCount
		if err := rows.Scan(&f.Name, &f.Count); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// groupFolders arranges folder counts by group, in rule order, skipping empty
// groups. Folders are listed largest first.
func groupFolders(counts []FolderCount) []FolderGroup {
	out := []FolderGroup{}
	for _, r := range groupRules {
		g := FolderGroup{Name: r.name}
		for _, f := range counts {
			if f.Name != "" && groupOfFolder(f.Name) == r.name {
				g.Folders = append(g.Folders, f)
				g.Count += f.Count
			}
		}
		if len(g.Folders) > 0 {
			sort.SliceStable(g.Folders, func(i, j int) bool { return g.Folders[i].Count > g.Folders[j].Count })
			out = append(out, g)
		}
	}
	return out
}
