# media-gallery-go

High-performance media gallery web UI written in Go with an embedded Vue 3 frontend. Browse, search, and stream photos and videos from a SQLite inventory database. Optional AI-powered face recognition via InsightFace.

## Features

- **Single binary** — Go + embedded Vue 3 UI, no runtime dependencies
- **Fast browsing** — SQLite-backed pagination, category and date filters
- **Thumbnail generation** — ffmpeg-powered, memory-safe, concurrent with semaphore
- **Video streaming** — HTTP Range support for seeking
- **Duplicate detection** — SHA256-based dupe groups
- **Face recognition** (opt-in) — InsightFace ArcFace embeddings, similarity search
- **Mobile-first UI** — native app feel with bottom tab bar, swipe-to-close lightbox, infinite scroll
- **Basic auth** — optional username/password protection

## Quick Start

```sh
# Build
./build.sh

# Run (defaults to ./media and ./inventory.db)
./media-gallery-go start -media /path/to/media -db /path/to/inventory.db

# With auth
./media-gallery-go start -media ./media -db ./inventory.db -user admin -pass secret

# With face recognition (requires Python + insightface)
./media-gallery-go start -media ./media -db ./inventory.db -faces-db ./faces.db
```

## Inventory Database

The gallery reads from a SQLite database with a `files` table:

```sql
CREATE TABLE files (
    relpath     TEXT PRIMARY KEY,
    size        INTEGER,
    mtime       INTEGER,
    sha256      TEXT,
    scanned_at  INTEGER
);
```

Paths in `relpath` are relative to the media root. The expected structure is `category/YYYY-MM/filename.ext`.

## Face Recognition (Optional)

Face recognition is an opt-in feature powered by [InsightFace](https://github.com/deepinsight/insightface) (ArcFace ResNet-100, 512D embeddings).

### Setup

```sh
pip install insightface onnxruntime
```

### Scan

```sh
# Scan all images (incremental — safe to stop/restart)
python3 scripts/face_scan.py --db inventory.db --media ./media --faces-db faces.db

# Check progress
python3 scripts/face_scan.py --db inventory.db --media ./media --faces-db faces.db --status

# Scan limited batch
python3 scripts/face_scan.py --db inventory.db --media ./media --faces-db faces.db --limit 100
```

The scan stores face embeddings, bounding boxes, gender/age estimates, and cropped face thumbnails. It runs in the background and won't block the gallery server.

### Enable in server

```sh
./media-gallery-go start -media ./media -db ./inventory.db -faces-db ./faces.db
```

## API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /api/browse` | Browse files (paginated, filter by category/year-month) |
| `GET /api/search?q=` | Search files by path |
| `GET /api/dupes` | List duplicate groups |
| `GET /api/dupes/:sha` | Files in a dupe group |
| `GET /api/stats` | Collection statistics |
| `GET /api/categories` | List top-level categories |
| `GET /api/yearmonths` | List year-month folders |
| `GET /api/thumb/:sha` | Thumbnail (auto-generated, cached) |
| `GET /api/download/:sha` | Download original file |
| `GET /api/stream/:sha` | Stream video with Range support |
| `GET /api/faces` | List detected faces (opt-in) |
| `GET /api/faces/status` | Face scan progress (opt-in) |
| `GET /api/faces/:id/similar` | Find similar faces by embedding (opt-in) |
| `GET /api/face-thumb/:id` | Cropped face thumbnail (opt-in) |

## UI

- **Vue 3** (CDN, no build step)
- **Tailwind CSS** (CDN)
- **Lucide icons**
- Mobile-first with bottom tab bar, desktop sidebar
- Swipe-to-close lightbox
- Infinite scroll
- Horizontal scrolling filter chips

## Build

```sh
./build.sh
# or manually:
CGO_ENABLED=0 go build -ldflags="-s -w" -o media-gallery-go .
```

## License

MIT
