#!/bin/bash
set -e

echo "Building media-gallery-go..."
CGO_ENABLED=0 go build -o media-gallery-go -ldflags="-s -w" .

if [ -f media-gallery-go ]; then
    size=$(du -h media-gallery-go | cut -f1)
    echo "Build successful: media-gallery-go ($size)"
else
    echo "Build failed"
    exit 1
fi
