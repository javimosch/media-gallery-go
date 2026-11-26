#!/usr/bin/env python3
"""Rebuild faces rows (no ML) from face thumbnails named <sha256>_face<N>.jpg.

bbox and embedding are placeholders, gender/age are NULL. scan_progress stays
empty so a later scan does not treat these photos as done.

Usage: faces_from_thumbs.py INVENTORY_DB MEDIA_ROOT THUMBS_DIR NEW_FACES_DB
THUMBS_DIR is stored as-is in thumb_path. NEW_FACES_DB must not exist.
"""
import os
import re
import sqlite3
import sys

from face_scan import init_faces_db

inventory, media, thumbs, out = sys.argv[1:5]
if os.path.exists(out):
    sys.exit(f"refusing to overwrite {out}")

found = []
for sub in os.scandir(thumbs):
    if sub.is_dir():
        for f in os.scandir(sub.path):
            m = re.fullmatch(r"([0-9a-f]{64})_face(\d+)\.jpg", f.name)
            if m and f.stat().st_size > 0:
                found.append((m[1], int(m[2]), f.path, f.stat().st_mtime))

# Prefer a path under a top-level folder present on the media root (one stat
# per folder, not per file).
top_ok = {}
def local(rel):
    top = rel.split("/")[0]
    if top not in top_ok:
        top_ok[top] = os.path.exists(os.path.join(media, top))
    return top_ok[top]

paths = {}
inv = sqlite3.connect(f"file:{inventory}?mode=ro", uri=True)
for sha, rel in inv.execute("SELECT sha256, relpath FROM files WHERE sha256 != '' ORDER BY relpath"):
    if sha not in paths or (not local(paths[sha]) and local(rel)):
        paths[sha] = rel

# The gallery lists faces by id DESC: insert oldest month first.
def month(rel):
    m = re.search(r"/(20\d\d-\d\d)/", rel)
    return m[1] if m else ""

rows = [(s, paths[s], i, p, t) for s, i, p, t in found if s in paths]
rows.sort(key=lambda r: (month(r[1]), r[1], r[2]))

conn = init_faces_db(out)
conn.executemany(
    "INSERT INTO faces (sha256, relpath, face_idx, bbox, embedding, thumb_path, scanned_at)"
    " VALUES (?, ?, ?, '[0,0,0,0]', x'', ?, ?)", rows)
conn.commit()
check = conn.execute("PRAGMA integrity_check").fetchone()[0]
n, images = conn.execute("SELECT COUNT(*), COUNT(DISTINCT sha256) FROM faces").fetchone()
conn.close()
print(f"thumbs={len(found)} inserted={n} images={images} not_in_inventory={len(found) - len(rows)} integrity={check}")
