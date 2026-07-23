#!/usr/bin/env python3
"""Face scan: detect faces in media files, extract embeddings, store in SQLite.

Usage:
  python3 face_scan.py --db inventory.db --media ./media --faces-db faces.db
  python3 face_scan.py --db inventory.db --media ./media --faces-db faces.db --limit 100
  python3 face_scan.py --db inventory.db --media ./media --faces-db faces.db --status
"""

import argparse
import json
import os
import sqlite3
import sys
import time
import struct
import numpy as np

IMAGE_EXTS = {'.jpg', '.jpeg', '.png', '.bmp', '.tif', '.tiff'}

def init_faces_db(path):
    conn = sqlite3.connect(path)
    conn.execute("""CREATE TABLE IF NOT EXISTS faces (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        sha256 TEXT NOT NULL,
        relpath TEXT NOT NULL,
        face_idx INTEGER NOT NULL,
        bbox TEXT NOT NULL,
        embedding BLOB NOT NULL,
        gender INTEGER,
        age INTEGER,
        thumb_path TEXT,
        scanned_at REAL NOT NULL,
        UNIQUE(sha256, face_idx)
    )""")
    conn.execute("CREATE INDEX IF NOT EXISTS idx_faces_sha ON faces(sha256)")
    conn.execute("CREATE INDEX IF NOT EXISTS idx_faces_relpath ON faces(relpath)")
    conn.execute("""CREATE TABLE IF NOT EXISTS scan_progress (
        sha256 TEXT PRIMARY KEY,
        status TEXT NOT NULL,
        face_count INTEGER DEFAULT 0,
        scanned_at REAL
    )""")
    conn.execute("""CREATE TABLE IF NOT EXISTS face_clusters (
        cluster_id INTEGER PRIMARY KEY AUTOINCREMENT,
        representative_face_id INTEGER,
        member_count INTEGER DEFAULT 0,
        label TEXT
    )""")
    conn.commit()
    return conn

def get_scanned_shas(conn):
    rows = conn.execute("SELECT sha256 FROM scan_progress WHERE status='done'").fetchall()
    return set(r[0] for r in rows)

def get_pending_files(inventory_db, scanned_shas, limit=0):
    conn = sqlite3.connect(inventory_db)
    conn.row_factory = sqlite3.Row
    query = "SELECT relpath, sha256 FROM files WHERE sha256 != '' ORDER BY relpath"
    if limit > 0:
        query += f" LIMIT {limit}"
    rows = conn.execute(query).fetchall()
    conn.close()
    pending = []
    for r in rows:
        ext = os.path.splitext(r['relpath'])[1].lower()
        if ext not in IMAGE_EXTS:
            continue
        if r['sha256'] in scanned_shas:
            continue
        pending.append({'relpath': r['relpath'], 'sha256': r['sha256']})
    return pending

def embedding_to_blob(emb):
    return struct.pack(f'{len(emb)}f', *emb)

def blob_to_embedding(blob):
    n = len(blob) // 4
    return np.array(struct.unpack(f'{n}f', blob), dtype=np.float32)

def save_face_thumb(app, face, img, sha256, face_idx, thumbs_dir):
    """Save a cropped face thumbnail."""
    bbox = face.bbox.astype(int)
    x1, y1, x2, y2 = bbox
    # pad bbox slightly
    h, w = img.shape[:2]
    pad = int((x2 - x1) * 0.3)
    x1 = max(0, x1 - pad)
    y1 = max(0, y1 - pad)
    x2 = min(w, x2 + pad)
    y2 = min(h, y2 + pad)
    face_img = img[y1:y2, x1:x2]
    if face_img.size == 0:
        return None
    import cv2
    subdir = os.path.join(thumbs_dir, sha256[:2])
    os.makedirs(subdir, exist_ok=True)
    thumb_path = os.path.join(subdir, f"{sha256}_face{face_idx}.jpg")
    cv2.imwrite(thumb_path, face_img, [cv2.IMWRITE_JPEG_QUALITY, 85])
    return thumb_path

def main():
    parser = argparse.ArgumentParser(description='Face scan for media gallery')
    parser.add_argument('--db', required=True, help='Inventory SQLite DB path')
    parser.add_argument('--media', required=True, help='Media root directory')
    parser.add_argument('--faces-db', required=True, help='Faces SQLite DB path')
    parser.add_argument('--thumbs', default='./thumbs/faces', help='Face thumbnails dir')
    parser.add_argument('--limit', type=int, default=0, help='Max files to scan (0=all)')
    parser.add_argument('--status', action='store_true', help='Show scan status and exit')
    parser.add_argument('--det-size', type=int, default=320, help='Detection size (lower=faster)')
    args = parser.parse_args()

    conn = init_faces_db(args.faces_db)

    if args.status:
        total = conn.execute("SELECT COUNT(*) FROM scan_progress").fetchone()[0]
        done = conn.execute("SELECT COUNT(*) FROM scan_progress WHERE status='done'").fetchone()[0]
        faces = conn.execute("SELECT COUNT(*) FROM faces").fetchone()[0]
        inv_conn = sqlite3.connect(args.db)
        total_images = inv_conn.execute(
            "SELECT COUNT(*) FROM files WHERE sha256 != '' AND (relpath LIKE '%.jpg' OR relpath LIKE '%.jpeg' OR relpath LIKE '%.png' OR relpath LIKE '%.bmp' OR relpath LIKE '%.tif' OR relpath LIKE '%.tiff')"
        ).fetchone()[0]
        inv_conn.close()
        print(json.dumps({
            "total_images": total_images,
            "scanned": done,
            "pending": total_images - done,
            "faces_found": faces,
            "progress_pct": round(done / max(total_images, 1) * 100, 1)
        }, indent=2))
        conn.close()
        return

    # Init insightface
    os.environ['OMP_NUM_THREADS'] = '2'
    from insightface.app import FaceAnalysis
    app = FaceAnalysis(name='buffalo_l', providers=['CPUExecutionProvider'])
    app.prepare(ctx_id=-1, det_size=(args.det_size, args.det_size))

    import cv2

    scanned = get_scanned_shas(conn)
    pending = get_pending_files(args.db, scanned, args.limit)

    print(f"Pending: {len(pending)} images", file=sys.stderr)

    t0 = time.time()
    processed = 0
    faces_found = 0

    for f in pending:
        full_path = os.path.join(args.media, f['relpath'])
        if not os.path.exists(full_path):
            conn.execute("INSERT OR REPLACE INTO scan_progress (sha256, status, face_count, scanned_at) VALUES (?, 'missing', 0, ?)",
                        (f['sha256'], time.time()))
            conn.commit()
            continue

        try:
            img = cv2.imread(full_path)
            if img is None:
                conn.execute("INSERT OR REPLACE INTO scan_progress (sha256, status, face_count, scanned_at) VALUES (?, 'error', 0, ?)",
                            (f['sha256'], time.time()))
                conn.commit()
                continue

            faces = app.get(img)

            for idx, face in enumerate(faces):
                emb = face.normed_embedding
                emb_blob = embedding_to_blob(emb)
                bbox = json.dumps(face.bbox.astype(int).tolist())
                gender = int(face.gender) if hasattr(face, 'gender') else None
                age = int(face.age) if hasattr(face, 'age') else None
                thumb_path = save_face_thumb(app, face, img, f['sha256'], idx, args.thumbs)

                conn.execute(
                    "INSERT OR REPLACE INTO faces (sha256, relpath, face_idx, bbox, embedding, gender, age, thumb_path, scanned_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
                    (f['sha256'], f['relpath'], idx, bbox, emb_blob, gender, age, thumb_path, time.time())
                )

            faces_found += len(faces)
            conn.execute("INSERT OR REPLACE INTO scan_progress (sha256, status, face_count, scanned_at) VALUES (?, 'done', ?, ?)",
                        (f['sha256'], len(faces), time.time()))
            conn.commit()

            processed += 1
            if processed % 50 == 0:
                elapsed = time.time() - t0
                rate = processed / elapsed
                remaining = (len(pending) - processed) / rate if rate > 0 else 0
                print(f"  {processed}/{len(pending)} | {faces_found} faces | {rate:.1f} img/s | ETA {remaining/60:.0f}min", file=sys.stderr)

        except Exception as e:
            conn.execute("INSERT OR REPLACE INTO scan_progress (sha256, status, face_count, scanned_at) VALUES (?, 'error', 0, ?)",
                        (f['sha256'], time.time()))
            conn.commit()
            print(f"  ERROR {f['relpath']}: {e}", file=sys.stderr)

    elapsed = time.time() - t0
    print(json.dumps({
        "processed": processed,
        "faces_found": faces_found,
        "elapsed_s": round(elapsed, 1),
        "rate_img_s": round(processed / max(elapsed, 1), 2)
    }, indent=2))
    conn.close()

if __name__ == '__main__':
    main()
