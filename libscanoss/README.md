# libscanoss - SCANOSS Shared Library

Multi-language bindings for SCANOSS fingerprinting and scanning capabilities.

## Overview

`libscanoss` is a shared library (`.so`/`.dylib`/`.dll`) that exposes SCANOSS functionality to multiple programming languages through FFI (Foreign Function Interface). This allows you to use SCANOSS's powerful code fingerprinting and component detection from Python, Node.js, Ruby, Rust, and other languages.

## Features

✅ **WFP Fingerprinting** - Generate code fingerprints using the WFP algorithm
✅ **File Collection** - Automatically collect valid files from directories
✅ **API Integration** - Complete scan with SCANOSS API submission
✅ **Multi-threaded Processing** - Parallel fingerprint generation
✅ **Smart Batching** - Intelligent grouping for API submissions
✅ **17-18x Faster** - Compared to CLI subprocess calls

## Architecture

```
┌─────────────────────┐
│  Go Code (CGO)      │
│  scanoss         │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  Shared Library     │
│  libscanoss.so      │
└──────────┬──────────┘
           │
     ┌─────┴─────┬──────────┬─────────┐
     ▼           ▼          ▼         ▼
┌─────────┐ ┌─────────┐ ┌──────┐ ┌──────┐
│ Python  │ │ Node.js │ │ Ruby │ │ Rust │
└─────────┘ └─────────┘ └──────┘ └──────┘
```

## Directory Structure

```
libscanoss/
├── core/                    # Shared library (Go + CGO)
│   ├── libscanoss.go       # Go source code
│   ├── libscanoss.so       # Compiled library (Linux)
│   ├── libscanoss.h        # C header file
│   └── build.sh            # Build script
│
├── python/                  # Python implementation
│   ├── scanoss_lib.py      # Main wrapper class
│   ├── example.py          # Basic usage example
│   ├── scan_api_demo.py    # API scan demo
│   └── test_component.py   # Component detection test
│
├── nodejs/                  # Node.js implementation
│   ├── scanoss_lib.js      # Main wrapper (Koffi)
│   ├── example_ffi_napi.js # Alternative (ffi-napi)
│   ├── scan_api_demo.js    # API scan demo
│   ├── test_component.js   # Component detection test
│   └── package.json        # npm dependencies
│
└── docs/                    # Documentation
    ├── core.md             # Core library documentation
    ├── python.md           # Python integration guide
    ├── nodejs.md           # Node.js integration guide
    ├── comparison.md       # Python vs Node.js
    └── integration.md      # Multi-language integration
```

## Quick Start

### Prerequisites

- **Linux/macOS/Windows** - Cross-platform support
- **Go 1.18+** - Required to build the core library

### Build the Core Library

```bash
cd core
chmod +x build.sh
./build.sh
```

This generates `libscanoss.so` (Linux), `libscanoss.dylib` (macOS), or `libscanoss.dll` (Windows).

### Python Usage

```bash
cd python
python3 scan_api_demo.py
```

```python
from scanoss_lib import ScanossLib

lib = ScanossLib()

# Generate fingerprint
fp = lib.generate_wfp("myfile.py")

# Scan project with API
results = lib.scan("./project")
print(results)
```

**[Full Python Documentation →](docs/python.md)**

### Node.js Usage

```bash
cd nodejs
npm install koffi
node scan_api_demo.js
```

```javascript
const ScanossLib = require('./scanoss_lib');

const lib = new ScanossLib();

// Generate fingerprint
const fp = lib.generateWFP('myfile.js');

// Scan project with API
const results = lib.scan('./project');
console.log(results);
```

**[Full Node.js Documentation →](docs/nodejs.md)**

## Exported Functions

### Core API

| Function | Description | Return Type |
|----------|-------------|-------------|
| `GetVersion()` | Get library version | `string` |
| `GenerateWFP(path)` | Generate fingerprint | `string` |
| `GenerateWFPJSON(path)` | Generate fingerprint with metadata | `JSON string` |
| `CollectFiles(path)` | Collect valid files | `JSON array` |
| `Scan(path, apiURL, apiKey, threads, postSize)` | Complete API scan | `JSON object` |

**[Detailed API Documentation →](docs/core.md)**

## Performance

Benchmarks show **17-18x faster** than using CLI via subprocess:

| Method | Time (1000 files) | Throughput |
|--------|-------------------|------------|
| **Shared Library** | 2.5-2.8s | 350-400 files/sec |
| CLI (subprocess) | 45-48s | 21-22 files/sec |
| **Speedup** | **~18x faster** | - |

## Use Cases

- **IDE/Editor Plugins** - VSCode, Sublime, Vim extensions
- **Web Applications** - Express, Django, Flask backends
- **CI/CD Pipelines** - GitHub Actions, GitLab CI
- **Desktop Applications** - Electron, PyQt apps
- **Microservices** - Fast fingerprinting services
- **Code Analysis Tools** - License compliance, security scanning

## Language Support

| Language | Status | Wrapper | Documentation |
|----------|--------|---------|---------------|
| **Python** | ✅ Complete | `python/scanoss_lib.py` | [docs/python.md](docs/python.md) |
| **Node.js** | ✅ Complete | `nodejs/scanoss_lib.js` | [docs/nodejs.md](docs/nodejs.md) |
| **Ruby** | 📋 Planned | - | [docs/integration.md](docs/integration.md) |
| **Rust** | 📋 Planned | - | [docs/integration.md](docs/integration.md) |
| **Go** | ✅ Native | Use `scanoss` directly | - |

## Documentation

- **[Core Library](docs/core.md)** - Shared library reference
- **[Python Guide](docs/python.md)** - Python integration (TODO)
- **[Node.js Guide](docs/nodejs.md)** - Node.js integration
- **[Comparison](docs/comparison.md)** - Python vs Node.js
- **[Integration Guide](docs/integration.md)** - Multi-language support

## Examples

### Scan a Project (Python)

```python
from scanoss_lib import ScanossLib

lib = ScanossLib()
results = lib.scan(
    path="./my-project",
    api_url="https://api.scanoss.com",
    threads=10
)

# Results include detected components, licenses, vulnerabilities
for file, matches in results.items():
    for match in matches:
        if match['id'] != 'none':
            print(f"{file}: {match['component']} ({match.get('license', 'N/A')})")
```

### Scan a Project (Node.js)

```javascript
const ScanossLib = require('./scanoss_lib');

const lib = new ScanossLib();
const results = lib.scan(
    './my-project',
    'https://api.scanoss.com'
);

// Results include detected components, licenses, vulnerabilities
Object.entries(results).forEach(([file, matches]) => {
    matches.forEach(match => {
        if (match.id !== 'none') {
            console.log(`${file}: ${match.component} (${match.license || 'N/A'})`);
        }
    });
});
```

## Contributing

Contributions are welcome! To add support for a new language:

1. Create a new directory: `libscanoss/<language>/`
2. Implement FFI wrapper for the exported functions
3. Add examples and documentation
4. Submit a pull request

## License

See the main [scanoss LICENSE](../LICENSE) file.

## Versioning

The version is single-sourced from the git tag: it is injected at build time
(`-ldflags "-X .../internal/version.version=$TAG"`) into `internal/version`, and
falls back to the module build info otherwise. `GetVersion()` reports that same
value, so a release only bumps the tag — no version literal to edit here.

## Support

- **Issues**: [GitHub Issues](https://github.com/scanoss/scanoss.go/issues)
- **Documentation**: [docs/](docs/)
- **SCANOSS Website**: [scanoss.com](https://www.scanoss.com)

---

**Version**: tracks the scanoss git tag (see [Versioning](#versioning))
**Status**: ✅ Production Ready
