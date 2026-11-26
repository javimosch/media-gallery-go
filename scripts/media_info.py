#!/usr/bin/env python3
"""Record each unique file's camera make and model (device filter) and capture date.

Usage: media_info.py INVENTORY_DB MEDIA_ROOT [--seed SCAN_TSV]

Only content without a row yet is scanned, so run it again after adding media.
Files not present under MEDIA_ROOT are left without a row. Reads media headers
only; writes only the media_info and media_dates tables (and creates an empty devices table,
which can hold readable labels: model, label, type).

SCAN_TSV optionally supplies earlier JPEG results, one file per line:
sha256, relpath, size, copies, kind, make, model, ...
"""
import json
import os
import re
import sqlite3
import subprocess
import sys
import time
import warnings

from PIL import Image

JPEG = (".jpg", ".jpeg")
VIDEO = (".mp4", ".mov", ".avi", ".mkv", ".wmv", ".3gp")


def clean(value):
    value = " ".join(str(value or "").replace("\x00", " ").split())
    return "" if set(value) <= set("-?") else value


def read(path):
    """Return (make, model, status)."""
    ext = os.path.splitext(path)[1].lower()
    try:
        if ext in JPEG:
            ex = Image.open(path).getexif()
            return clean(ex.get(271)), clean(ex.get(272)), "ok"
        if ext in VIDEO:
            out = subprocess.run(
                ["ffprobe", "-v", "error", "-show_entries", "format_tags", "-of", "json", path],
                capture_output=True, text=True, timeout=60, check=True).stdout
            tags = {k.lower(): v for k, v in json.loads(out).get("format", {}).get("tags", {}).items()}

            def first(*keys):
                return next((clean(tags[k]) for k in keys if clean(tags.get(k))), "")

            return (first("com.android.manufacturer", "com.apple.quicktime.make", "make"),
                    first("com.android.model", "com.apple.quicktime.model", "model"), "ok")
        return "", "", "unsupported"
    except Exception:
        return "", "", "error"


def exif_date(value):
    """EXIF "YYYY:MM:DD HH:MM:SS" -> "YYYY-MM-DD HH:MM:SS", or ""."""
    m = re.match(r"((?:19|20)\d\d):(\d\d):(\d\d)(?: (\d\d:\d\d:\d\d))?", str(value or ""))
    return "%s-%s-%s %s" % (m[1], m[2], m[3], m[4] or "00:00:00") if m else ""


def read_date(path):
    """Return (taken, source) from the file's own metadata."""
    ext = os.path.splitext(path)[1].lower()
    try:
        if ext in JPEG:
            ex = Image.open(path).getexif()
            taken = exif_date(ex.get_ifd(0x8769).get(36867) or ex.get(306))
            return taken, "exif" if taken else ""
        if ext in VIDEO:
            out = subprocess.run(
                ["ffprobe", "-v", "error", "-show_entries", "format_tags=creation_time", "-of", "json", path],
                capture_output=True, text=True, timeout=60, check=True).stdout
            value = json.loads(out).get("format", {}).get("tags", {}).get("creation_time", "")
            if re.match(r"\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d", value):
                return value[:10] + " " + value[11:19], "video"  # UTC
    except Exception:
        pass
    return "", ""


def record_dates(db, root, seed_file):
    db.execute("create table if not exists media_dates(sha256 text primary key, taken text not null, source text not null)")
    db.commit()
    done = {s for s, in db.execute("select sha256 from media_dates")}
    tops = set(os.listdir(root))
    paths = {}
    for rel, sha in db.execute("select relpath, sha256 from files order by relpath"):
        if sha and sha not in done and rel.split("/")[0] in tops:
            paths.setdefault(sha, rel)
    seed = {}
    if seed_file:
        for line in open(seed_file):
            f = line.rstrip("\n").split("\t")
            if f[4] == "jpeg" and f[1].lower().endswith(JPEG):
                taken = exif_date(f[7])
                seed[f[0]] = (taken, "exif" if taken else "")
    pending, count, start = [], 0, time.time()

    def flush():
        with db:
            db.executemany("insert into media_dates values(?,?,?)", pending)
        pending.clear()

    print("dates to record:", len(paths), "from seed:", len(set(paths) & set(seed)), flush=True)
    for sha, rel in sorted(paths.items(), key=lambda kv: kv[1]):
        info = seed.get(sha)
        if info is None:
            full = os.path.join(root, rel)
            if not os.path.isfile(full):
                continue
            info = read_date(full)
            count += 1
            if count % 1000 == 0:
                print("dates: read", count, "files %.0fs" % (time.time() - start), flush=True)
        pending.append((sha, *info))
        if len(pending) >= 500:
            flush()
    flush()
    print("DATES DONE read %d files in %.0fs" % (count, time.time() - start), flush=True)


def main():
    warnings.filterwarnings("ignore")
    db_path, root = sys.argv[1], sys.argv[2]
    db = sqlite3.connect(db_path, timeout=60)
    db.execute("create table if not exists media_info(sha256 text primary key, make text not null, "
               "model text not null, status text not null, scanned_at integer)")
    db.execute("create index if not exists idx_media_info_model on media_info(model)")
    db.execute("create table if not exists devices(model text primary key, label text not null, type text not null)")
    db.commit()
    done = {s for s, in db.execute("select sha256 from media_info")}
    paths = {}
    tops = set(os.listdir(root))
    for rel, sha in db.execute("select relpath, sha256 from files order by relpath"):
        if sha and sha not in done and rel.split("/")[0] in tops:
            paths.setdefault(sha, rel)

    seed = {}
    if len(sys.argv) > 4 and sys.argv[3] == "--seed":
        for line in open(sys.argv[4]):
            f = line.rstrip("\n").split("\t")
            if f[4] in ("jpeg", "error") and f[1].lower().endswith(JPEG):
                seed[f[0]] = (clean(f[5]), clean(f[6]), "ok" if f[4] == "jpeg" else "error")

    pending, count, start = [], 0, time.time()

    def flush():
        with db:
            db.executemany("insert into media_info values(?,?,?,?,?)", pending)
        pending.clear()

    print("to record:", len(paths), "from seed:", len(set(paths) & set(seed)), flush=True)
    for sha, rel in sorted(paths.items(), key=lambda kv: kv[1]):
        info = seed.get(sha)
        if info is None:
            full = os.path.join(root, rel)
            if not os.path.isfile(full):
                continue
            info = read(full)
            count += 1
            if count % 1000 == 0:
                print("read", count, "files %.0fs" % (time.time() - start), flush=True)
        pending.append((sha, *info, int(time.time())))
        if len(pending) >= 500:
            flush()
    flush()
    print("DONE read %d files in %.0fs" % (count, time.time() - start), flush=True)
    record_dates(db, root, sys.argv[4] if len(sys.argv) > 4 and sys.argv[3] == "--seed" else None)


if __name__ == "__main__":
    main()
