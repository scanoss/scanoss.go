# Integration Guide - libscanoss

This guide shows how to integrate SCANOSS functionalities in different languages and platforms using the `libscanoss` shared library.

## Table of Contents

- [Python](#python)
- [Node.js](#nodejs)
- [Ruby](#ruby)
- [Rust](#rust)
- [Use Cases](#use-cases)
- [Performance](#performance)

---

## Python

### Installation

No additional dependencies required, just Python 3 with `ctypes` (included by default).

### Basic Usage

```python
from scanoss_lib import ScanossLib

# Initialize
lib = ScanossLib()

# Generate fingerprint
fingerprint = lib.generate_wfp("myfile.py")
print(fingerprint)

# Get complete metadata
metadata = lib.generate_wfp_json("myfile.py")
print(f"Path: {metadata['Path']}")
print(f"Hash: {metadata['Hash']}")
print(f"Size: {metadata['Size']} bytes")

# Collect files
files = lib.collect_files("./src")
print(f"Found {len(files)} files")
```

### Complete Example

```bash
cd libscanoss/python
python3 scanoss_lib.py
```

### Project Integration

```python
# myproject/scanner.py
import sys
sys.path.append('/path/to/libscanoss/python')

from scanoss_lib import ScanossLib

class ProjectScanner:
    def __init__(self):
        self.lib = ScanossLib()

    def scan_directory(self, path):
        files = self.lib.collect_files(path)
        results = []

        for file in files:
            metadata = self.lib.generate_wfp_json(file)
            results.append(metadata)

        return results

# Usage
scanner = ProjectScanner()
results = scanner.scan_directory("./my-code")
for r in results:
    print(f"Scanned: {r['Path']}")
```

---

## Node.js

### Installation

```bash
npm install koffi
```

### Basic Usage

```javascript
const ScanossLib = require('./scanoss_lib');

// Initialize
const lib = new ScanossLib();

// Generate fingerprint
const fingerprint = lib.generateWFP('myfile.js');
console.log(fingerprint);

// Get complete metadata
const metadata = lib.generateWFPJSON('myfile.js');
console.log('Path:', metadata.Path);
console.log('Hash:', metadata.Hash);
console.log('Size:', metadata.Size, 'bytes');

// Collect files
const files = lib.collectFiles('./src');
console.log(`Found ${files.length} files`);
```

### Complete Example

```bash
cd libscanoss/nodejs
npm install
node scanoss_lib.js
```

### Express API Integration

```javascript
// server.js
const express = require('express');
const ScanossLib = require('./libscanoss/nodejs/scanoss_lib');

const app = express();
const lib = new ScanossLib();

app.get('/api/scan/:filepath', (req, res) => {
    try {
        const filepath = req.params.filepath;
        const metadata = lib.generateWFPJSON(filepath);
        res.json(metadata);
    } catch (error) {
        res.status(500).json({ error: error.message });
    }
});

app.get('/api/files/:dirpath', (req, res) => {
    try {
        const dirpath = req.params.dirpath;
        const files = lib.collectFiles(dirpath);
        res.json({ files });
    } catch (error) {
        res.status(500).json({ error: error.message });
    }
});

app.listen(3000, () => {
    console.log('SCANOSS API listening on port 3000');
});
```

---

## Ruby

### Installation

```bash
gem install ffi
```

### Basic Usage

```ruby
require 'ffi'
require 'json'

module Scanoss
  extend FFI::Library
  ffi_lib './libscanoss.so'

  attach_function :GetVersion, [], :string
  attach_function :GenerateWFP, [:string], :string
  attach_function :GenerateWFPJSON, [:string], :string
  attach_function :CollectFiles, [:string], :string
end

# Usage
puts "Version: #{Scanoss.GetVersion}"

# Generate fingerprint
fingerprint = Scanoss.GenerateWFP('myfile.rb')
puts fingerprint

# Complete metadata
json_str = Scanoss.GenerateWFPJSON('myfile.rb')
metadata = JSON.parse(json_str)
puts "Path: #{metadata['Path']}"
puts "Hash: #{metadata['Hash']}"

# Collect files
files_str = Scanoss.CollectFiles('./lib')
files = JSON.parse(files_str)
puts "Found #{files.length} files"
```

### Object-Oriented Ruby Wrapper

```ruby
# scanoss_lib.rb
require 'ffi'
require 'json'

class ScanossLib
  extend FFI::Library

  ffi_lib File.join(__dir__, 'libscanoss.so')

  attach_function :GetVersion, [], :string
  attach_function :GenerateWFP, [:string], :string
  attach_function :GenerateWFPJSON, [:string], :string
  attach_function :CollectFiles, [:string], :string

  def self.version
    GetVersion()
  end

  def self.generate_wfp(file_path)
    GenerateWFP(file_path)
  end

  def self.generate_wfp_json(file_path)
    json_str = GenerateWFPJSON(file_path)
    JSON.parse(json_str)
  end

  def self.collect_files(path)
    json_str = CollectFiles(path)
    JSON.parse(json_str)
  end
end

# Usage
puts "Version: #{ScanossLib.version}"

metadata = ScanossLib.generate_wfp_json('test.rb')
puts "Scanned: #{metadata['Path']}"

files = ScanossLib.collect_files('./lib')
files.each { |f| puts "  - #{f}" }
```

---

## Rust

### Installation

```toml
# Cargo.toml
[dependencies]
libloading = "0.8"
serde_json = "1.0"
```

### Basic Usage

```rust
use libloading::{Library, Symbol};
use std::ffi::{CStr, CString};
use std::os::raw::c_char;

type GetVersionFn = unsafe extern "C" fn() -> *const c_char;
type GenerateWFPFn = unsafe extern "C" fn(*const c_char) -> *const c_char;
type GenerateWFPJSONFn = unsafe extern "C" fn(*const c_char) -> *const c_char;

fn main() {
    unsafe {
        // Load library
        let lib = Library::new("./libscanoss.so").expect("Failed to load library");

        // Get functions
        let get_version: Symbol<GetVersionFn> = lib.get(b"GetVersion")
            .expect("Failed to load GetVersion");

        let generate_wfp: Symbol<GenerateWFPFn> = lib.get(b"GenerateWFP")
            .expect("Failed to load GenerateWFP");

        // Use functions
        let version_ptr = get_version();
        let version = CStr::from_ptr(version_ptr).to_str().unwrap();
        println!("Version: {}", version);

        // Generate fingerprint
        let file_path = CString::new("test.rs").unwrap();
        let fp_ptr = generate_wfp(file_path.as_ptr());
        let fingerprint = CStr::from_ptr(fp_ptr).to_str().unwrap();
        println!("Fingerprint: {}", fingerprint);
    }
}
```

---

## Use Cases

### 1. VSCode / IDE Plugin

```javascript
// VSCode Extension
const vscode = require('vscode');
const ScanossLib = require('./libscanoss');

function activate(context) {
    const lib = new ScanossLib();

    let disposable = vscode.commands.registerCommand('scanoss.scan', function () {
        const editor = vscode.window.activeTextEditor;
        if (editor) {
            const filePath = editor.document.fileName;
            const metadata = lib.generateWFPJSON(filePath);

            vscode.window.showInformationMessage(
                `Scanned: ${metadata.Path} (${metadata.Size} bytes)`
            );
        }
    });

    context.subscriptions.push(disposable);
}
```

### 2. CI/CD Pipeline (Python)

```python
#!/usr/bin/env python3
"""
SCANOSS CI/CD Scanner
Integrate into .gitlab-ci.yml or GitHub Actions
"""
import sys
from scanoss_lib import ScanossLib

def scan_project(path):
    lib = ScanossLib()
    files = lib.collect_files(path)

    results = []
    for file in files:
        try:
            metadata = lib.generate_wfp_json(file)
            results.append(metadata)
        except Exception as e:
            print(f"Error scanning {file}: {e}", file=sys.stderr)

    return results

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: scan_ci.py <project-path>")
        sys.exit(1)

    results = scan_project(sys.argv[1])
    print(f"Scanned {len(results)} files")

    # Save results for artifact
    import json
    with open('scan-results.json', 'w') as f:
        json.dump(results, f, indent=2)
```

### 3. Web Server (Node.js + Express)

```javascript
const express = require('express');
const multer = require('multer');
const ScanossLib = require('./libscanoss');

const app = express();
const upload = multer({ dest: 'uploads/' });
const lib = new ScanossLib();

// API to scan uploaded file
app.post('/api/scan/upload', upload.single('file'), (req, res) => {
    try {
        const metadata = lib.generateWFPJSON(req.file.path);
        res.json({
            success: true,
            data: metadata
        });
    } catch (error) {
        res.status(500).json({
            success: false,
            error: error.message
        });
    }
});

// API to scan server directory
app.post('/api/scan/directory', express.json(), (req, res) => {
    try {
        const { path } = req.body;
        const files = lib.collectFiles(path);

        const results = files.map(file => {
            try {
                return lib.generateWFPJSON(file);
            } catch (e) {
                return { error: e.message, path: file };
            }
        });

        res.json({
            success: true,
            count: results.length,
            data: results
        });
    } catch (error) {
        res.status(500).json({
            success: false,
            error: error.message
        });
    }
});

app.listen(3000);
```

### 4. Desktop App (Electron)

```javascript
// main.js (Electron main process)
const { app, BrowserWindow, ipcMain } = require('electron');
const ScanossLib = require('./libscanoss');

let lib;

app.whenReady().then(() => {
    lib = new ScanossLib();

    // Handler for IPC
    ipcMain.handle('scan-file', async (event, filePath) => {
        try {
            return lib.generateWFPJSON(filePath);
        } catch (error) {
            throw new Error(error.message);
        }
    });

    ipcMain.handle('scan-directory', async (event, dirPath) => {
        try {
            return lib.collectFiles(dirPath);
        } catch (error) {
            throw new Error(error.message);
        }
    });
});

// renderer.js (Electron renderer process)
document.getElementById('scan-btn').addEventListener('click', async () => {
    const filePath = document.getElementById('file-input').value;
    const metadata = await window.electron.scanFile(filePath);
    console.log('Scan result:', metadata);
});
```

---

## Performance

### Benchmarks

Comparison of different approaches:

```python
import time
from scanoss_lib import ScanossLib

lib = ScanossLib()

# Test: Generate 1000 fingerprints
files = lib.collect_files("./large-project")[:1000]

# Using shared library
start = time.time()
for file in files:
    fp = lib.generate_wfp(file)
shared_lib_time = time.time() - start

print(f"Shared Library: {shared_lib_time:.2f}s")
print(f"Throughput: {len(files)/shared_lib_time:.2f} files/sec")

# Compared to subprocess (CLI)
import subprocess
start = time.time()
for file in files:
    subprocess.run(['./scanoss', 'wfp', file],
                   capture_output=True)
cli_time = time.time() - start

print(f"CLI Subprocess: {cli_time:.2f}s")
print(f"Speedup: {cli_time/shared_lib_time:.2f}x faster")
```

**Typical results:**
- Shared Library: ~2.5s for 1000 files (400 files/sec)
- CLI Subprocess: ~45s for 1000 files (22 files/sec)
- **Speedup: ~18x faster** using shared library

### Optimizations

1. **Result caching**:
```python
from functools import lru_cache

@lru_cache(maxsize=1000)
def cached_wfp(file_path):
    return lib.generate_wfp(file_path)
```

2. **Parallel processing**:
```python
from concurrent.futures import ThreadPoolExecutor

def scan_parallel(files, workers=4):
    with ThreadPoolExecutor(max_workers=workers) as executor:
        results = list(executor.map(
            lambda f: lib.generate_wfp_json(f),
            files
        ))
    return results
```

3. **Batch processing**:
```python
def scan_in_batches(directory, batch_size=100):
    files = lib.collect_files(directory)

    for i in range(0, len(files), batch_size):
        batch = files[i:i+batch_size]
        yield [lib.generate_wfp_json(f) for f in batch]
```

---

## Troubleshooting

### Error: Library not found

```python
# Specify absolute path
lib = ScanossLib('/absolute/path/to/libscanoss.so')
```

### Error: Symbol not found

Verify that the library has exported symbols:
```bash
nm -D libscanoss.so | grep -i "generate"
```

### Error: Segmentation fault

Make sure not to manually free memory (Go GC handles it).

### Performance issues

- Use release version: `go build -buildmode=c-shared -ldflags="-s -w"`
- Enable optimizations: `CGO_ENABLED=1 go build -buildmode=c-shared -o libscanoss.so`

---

## Distribution

### Create Python package

```python
# setup.py
from setuptools import setup

setup(
    name='scanoss-lib',
    version='0.0.0',  # set at publish (single-sourced from the git tag)
    py_modules=['scanoss_lib'],
    package_data={'': ['libscanoss.so', 'libscanoss.dylib', 'libscanoss.dll']},
    install_requires=[],
)
```

### Create npm package

```json
{
  "name": "scanoss-lib",
  "version": "0.0.0",
  "main": "scanoss_lib.js",
  "files": ["scanoss_lib.js", "libscanoss.so", "libscanoss.dylib", "libscanoss.dll"],
  "dependencies": {
    "koffi": "^2.15.0"
  }
}
```

### Multi-platform

Compile for each platform:
```bash
# Linux
GOOS=linux GOARCH=amd64 go build -buildmode=c-shared -o libscanoss_linux_amd64.so

# macOS
GOOS=darwin GOARCH=amd64 go build -buildmode=c-shared -o libscanoss_darwin_amd64.dylib

# Windows (requires mingw)
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc \
  go build -buildmode=c-shared -o libscanoss_windows_amd64.dll
```

---

## Conclusion

The `libscanoss` shared library provides an efficient and flexible way to integrate SCANOSS capabilities into any language that supports FFI. It's ideal for:

- ✅ High-performance applications
- ✅ Native integrations in IDEs and editors
- ✅ Web services and APIs
- ✅ CI/CD tools
- ✅ Desktop applications

For more information, see the [main README](../README.md).
