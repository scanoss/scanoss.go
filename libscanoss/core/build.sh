#!/bin/bash

# Build shared library for different platforms

echo "Building SCANOSS shared library..."

# Linux
echo "Building for Linux (libscanoss.so)..."
go build -buildmode=c-shared -o libscanoss.so libscanoss.go

# macOS (if running on macOS)
if [[ "$OSTYPE" == "darwin"* ]]; then
    echo "Building for macOS (libscanoss.dylib)..."
    go build -buildmode=c-shared -o libscanoss.dylib libscanoss.go
fi

# Windows (requires mingw-w64 cross compiler)
# GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc go build -buildmode=c-shared -o libscanoss.dll libscanoss.go

echo "Build complete!"
ls -lh libscanoss.*
