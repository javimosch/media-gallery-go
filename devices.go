package main

import "strings"

// The device filter reads two optional tables, filled by scripts/media_info.py:
//
//	media_info(sha256 TEXT PRIMARY KEY, make, model, status, scanned_at)
//	devices(model TEXT PRIMARY KEY, label TEXT, type TEXT)
//
// A file with no row, or an empty model, has an unknown device.

// unknownDevice is the filter value for files without a known device. It starts
// with a tab so it cannot equal a recorded model, whose whitespace is collapsed.
const unknownDevice = "\tunknown"

const unknownDeviceParam = "-unknown-"

func (d *DB) detectDevices() error {
	var n int
	err := d.conn.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('media_info','devices')").Scan(&n)
	d.hasDevices = n == 2
	return err
}

func (d *DB) deviceCondition(device string) (string, []interface{}) {
	switch {
	case !d.hasDevices:
		return "1=0", nil
	case device == unknownDevice:
		return "COALESCE(sha256,'') NOT IN (SELECT sha256 FROM media_info WHERE model <> '')", nil
	}
	return "sha256 IN (SELECT sha256 FROM media_info WHERE model = ?)", []interface{}{device}
}

type DeviceCount struct {
	Value string `json:"value"` // what to send back as ?device=
	Label string `json:"label"`
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// DeviceCounts lists the devices present in a group, folder and month, counted
// like the folder chips: junk included, assigned copies once. Unknown is last.
func (d *DB) DeviceCounts(group, category, yearMonth string) ([]DeviceCount, error) {
	out := []DeviceCount{}
	if !d.hasDevices {
		return out, nil
	}
	conditions := []string{"visible(relpath)", "shown"}
	var args []interface{}
	if group != "" {
		conditions = append(conditions, "folder_group(vpath) = ?")
		args = append(args, group)
	}
	if category != "" {
		conditions = append(conditions, "vpath LIKE ? || '/%'")
		args = append(args, category)
	}
	if yearMonth != "" {
		conditions = append(conditions, "vpath LIKE '%/' || ? || '/%'")
		args = append(args, yearMonth)
	}
	rows, err := d.conn.Query(`SELECT c.model, COALESCE(v.label, c.model), COALESCE(v.type, 'other'), c.n FROM (
			SELECT COALESCE(m.model, '') AS model, COUNT(*) AS n
			FROM (SELECT sha256 AS content FROM `+d.items+` WHERE `+strings.Join(conditions, " AND ")+`) AS f
			LEFT JOIN media_info m ON m.sha256 = f.content
			GROUP BY 1
		) AS c LEFT JOIN devices v ON v.model = c.model
		ORDER BY c.model = '', c.n DESC, c.model`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c DeviceCount
		if err := rows.Scan(&c.Value, &c.Label, &c.Type, &c.Count); err != nil {
			return nil, err
		}
		if c.Value == "" {
			c.Value, c.Label, c.Type = unknownDeviceParam, "Unknown device", "unknown"
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func deviceFromParam(v string) string {
	if v == unknownDeviceParam {
		return unknownDevice
	}
	return v
}
