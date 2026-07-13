# Python Integration Guide

Complete guide for using libscanoss from Python.

## Installation

No external dependencies required - Python's built-in `ctypes` library is used for FFI.

```bash
# Just ensure you have the compiled library
cd libscanoss/core
./build.sh
```

## Quick Start

```python
import sys
sys.path.append('../python')

from scanoss_lib import ScanossLib

# Initialize
lib = ScanossLib('../core/libscanoss.so')

# Get version
print(lib.get_version())  # the build/tag version, e.g. "v0.10.0"

# Generate fingerprint
fp = lib.generate_wfp("myfile.py")
print(fp)
```

## API Reference

### `ScanossLib(lib_path=None)`

Initialize the library wrapper.

**Parameters:**
- `lib_path` (str, optional): Path to the shared library. Auto-detects if not provided.

**Example:**
```python
lib = ScanossLib()  # Auto-detect
# or
lib = ScanossLib('/path/to/libscanoss.so')  # Explicit path
```

### `get_version() -> str`

Get the library version.

**Returns:** Version string — the build/tag version (e.g., "v0.10.0")

**Example:**
```python
version = lib.get_version()
print(f"Library version: {version}")
```

### `generate_wfp(file_path) -> str`

Generate a WFP fingerprint for a file.

**Parameters:**
- `file_path` (str): Path to the file

**Returns:** Fingerprint string

**Example:**
```python
fp = lib.generate_wfp("main.py")
print(fp)
```

### `generate_wfp_json(file_path) -> dict`

Generate a WFP fingerprint with complete metadata.

**Parameters:** Same as `generate_wfp`

**Returns:** Dictionary with metadata

**Example:**
```python
metadata = lib.generate_wfp_json("main.py")
print(f"Path: {metadata['Path']}")
print(f"Hash: {metadata['Hash']}")
print(f"Size: {metadata['Size']} bytes")
print(f"Fingerprint: {metadata['Fingerprint'][:100]}...")
```

**Response Structure:**
```json
{
  "Path": "/path/to/file.py",
  "Hash": "abc123...",
  "Size": 1234,
  "Fingerprint": "file=..."
}
```

### `collect_files(path) -> list`

Collect all valid files from a directory or return single file.

**Parameters:**
- `path` (str): Path to file or directory

**Returns:** List of file paths

**Example:**
```python
files = lib.collect_files("./src")
print(f"Found {len(files)} files")
for file in files:
    print(f"  - {file}")
```

### `scan(path, api_url, api_key, threads, post_size) -> dict`

Perform complete scan: collect files, generate fingerprints, send to API.

**Parameters:**
- `path` (str): Path to file or directory
- `api_url` (str): SCANOSS API URL (default: "https://api.osskb.org/scan/direct")
- `api_key` (str): API key (default: "")
- `threads` (int): Number of parallel threads (default: 10)
- `post_size` (int): Maximum POST size in bytes (default: 65536)

**Returns:** Dictionary with API scan results

**Example:**
```python
results = lib.scan(
    path="./my-project",
    api_url="https://api.osskb.org/scan/direct",
    api_key="",
    threads=10,
    post_size=65536
)

# Process results
for file_path, matches in results.items():
    print(f"\nFile: {file_path}")
    for match in matches:
        if match['id'] != 'none':
            print(f"  Component: {match.get('component', 'N/A')}")
            print(f"  License: {match.get('license', 'N/A')}")
            print(f"  Version: {match.get('version', 'N/A')}")
```

**Response Structure:**
```json
{
  "/path/to/file.py": [
    {
      "id": "file",
      "component": "requests",
      "vendor": "psf",
      "version": "2.28.0",
      "licenses": [
        {"name": "Apache-2.0"}
      ],
      "purl": ["pkg:pypi/requests"],
      "matched": "95%"
    }
  ]
}
```

## Examples

### Example 1: Basic Fingerprinting

```python
from scanoss_lib import ScanossLib

lib = ScanossLib()

# Single file
fp = lib.generate_wfp("app.py")
print(f"Fingerprint: {fp[:100]}...")

# With metadata
metadata = lib.generate_wfp_json("app.py")
print(f"File: {metadata['Path']}")
print(f"Size: {metadata['Size']} bytes")
print(f"Hash: {metadata['Hash']}")
```

### Example 2: Batch Processing

```python
from scanoss_lib import ScanossLib

lib = ScanossLib()

# Collect all files
files = lib.collect_files("./src")
print(f"Processing {len(files)} files...")

# Generate fingerprints for all
for file_path in files:
    try:
        metadata = lib.generate_wfp_json(file_path)
        print(f"✓ {file_path}: {metadata['Size']} bytes")
    except Exception as e:
        print(f"✗ {file_path}: {e}")
```

### Example 3: Complete API Scan

```python
from scanoss_lib import ScanossLib
import json

lib = ScanossLib()

print("Scanning project...")
results = lib.scan(
    path="./my-project",
    api_url="https://api.osskb.org/scan/direct",
    threads=15
)

# Save results
with open('scan_results.json', 'w') as f:
    json.dump(results, f, indent=2)

# Summary
total_files = len(results)
components_found = sum(
    1 for matches in results.values()
    for m in matches if m.get('id') != 'none'
)

print(f"\nScan complete:")
print(f"  Files scanned: {total_files}")
print(f"  Components found: {components_found}")
```

### Example 4: Component Detection

```python
from scanoss_lib import ScanossLib

lib = ScanossLib()

# Scan a known file (e.g., Express.js)
results = lib.scan("/tmp/express.js")

for file_path, matches in results.items():
    print(f"File: {file_path}")
    for match in matches:
        if match['id'] != 'none':
            print(f"  ✓ Component: {match['component']}")
            print(f"    Vendor: {match['vendor']}")
            print(f"    License: {match.get('license', 'N/A')}")
            print(f"    Match: {match.get('matched', 'N/A')}")
```

### Example 5: CI/CD Integration

```python
#!/usr/bin/env python3
"""
SCANOSS CI/CD Scanner
"""
import sys
from scanoss_lib import ScanossLib

def scan_project(path):
    lib = ScanossLib()

    try:
        results = lib.scan(path)

        # Check for components
        for file_path, matches in results.items():
            for match in matches:
                if match['id'] != 'none':
                    # Check licenses
                    licenses = match.get('licenses', [])
                    for lic in licenses:
                        if lic['name'] in ['GPL-3.0', 'AGPL-3.0']:
                            print(f"⚠️  Copyleft license found: {lic['name']}")
                            print(f"   File: {file_path}")
                            print(f"   Component: {match['component']}")
                            return 1  # Fail CI

        print("✅ No license issues found")
        return 0

    except Exception as e:
        print(f"❌ Scan failed: {e}")
        return 1

if __name__ == "__main__":
    path = sys.argv[1] if len(sys.argv) > 1 else "."
    sys.exit(scan_project(path))
```

## Running the Examples

```bash
# Basic example
cd python
python3 example.py

# API scan demo
python3 scan_api_demo.py

# Component detection test
python3 test_component.py
```

## Error Handling

```python
from scanoss_lib import ScanossLib

lib = ScanossLib()

try:
    results = lib.scan("./project")

    if 'error' in results:
        print(f"API Error: {results['error']}")
    else:
        print(f"Scanned {len(results)} files")

except FileNotFoundError:
    print("Library not found. Did you compile it?")
except Exception as e:
    print(f"Error: {e}")
```

## Performance Tips

### 1. Adjust Thread Count

```python
# More threads for larger projects
results = lib.scan("./large-project", threads=20)
```

### 2. Increase POST Size

```python
# Larger batches for better throughput
results = lib.scan("./project", post_size=131072)  # 128KB
```

### 3. Use Caching

```python
from functools import lru_cache

@lru_cache(maxsize=1000)
def cached_fingerprint(file_path):
    lib = ScanossLib()
    return lib.generate_wfp(file_path)
```

## Platform-Specific Notes

### Linux
```python
lib = ScanossLib('../core/libscanoss.so')
```

### macOS
```python
lib = ScanossLib('../core/libscanoss.dylib')
```

### Windows
```python
lib = ScanossLib('../core/libscanoss.dll')
```

Or use auto-detection:
```python
lib = ScanossLib()  # Automatically detects platform
```

## Troubleshooting

### Library Not Found
```python
# Specify absolute path
lib = ScanossLib('/absolute/path/to/libscanoss.so')
```

### Permission Denied
```bash
chmod +x ../core/libscanoss.so
```

### Symbol Not Found
```bash
# Verify exported symbols
nm -D ../core/libscanoss.so | grep Generate
```

## Next Steps

- See [scan_api_demo.py](../python/scan_api_demo.py) for a complete example
- Check [comparison.md](comparison.md) for Python vs Node.js
- Read [integration.md](integration.md) for other languages

---

**[← Back to Main README](../README.md)**
