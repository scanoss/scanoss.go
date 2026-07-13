# libscanoss - SCANOSS Shared Library

SCANOSS shared library that allows using fingerprinting functionalities from other programming languages like Python, Node.js, Ruby, etc.

## Architecture

```
Go Code (scanoss)
       ↓
   CGO Export
       ↓
Shared C Library (.so/.dylib/.dll)
       ↓
FFI (ctypes, cffi, node-ffi, etc.)
       ↓
Python / Node.js / Ruby / etc.
```

## Compilation

### Linux

```bash
cd libscanoss
chmod +x build.sh
./build.sh
```

This will generate `libscanoss.so`

### macOS

```bash
cd libscanoss
go build -buildmode=c-shared -o libscanoss.dylib libscanoss.go
```

### Windows

Requires mingw-w64:

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc \
  go build -buildmode=c-shared -o libscanoss.dll libscanoss.go
```

## Exported Functions

### `GetVersion() -> char*`

Returns the library version.

```python
version = lib.get_version()  # the build/tag version, e.g. "v0.10.0"
```

### `GenerateWFP(filePath) -> char*`

Generates a WFP fingerprint for a file.

**Parameters:**
- `filePath` (char*): Path to the file

**Returns:** String with the fingerprint

```python
fingerprint = lib.generate_wfp("/path/to/file.go")
```

### `GenerateWFPJSON(filePath) -> char*`

Generates a WFP fingerprint with complete metadata in JSON.

**Parameters:** Same as `GenerateWFP`

**Returns:** JSON string with complete metadata

```python
metadata = lib.generate_wfp_json("/path/to/file.go")
# {"Path": "...", "Hash": "...", "Size": 1234, "Fingerprint": "..."}
```

### `CollectFiles(path) -> char*`

Collects all valid files from a directory.

**Parameters:**
- `path` (char*): Path to file or directory

**Returns:** JSON array with file paths

```python
files = lib.collect_files("/path/to/project")
# ["/path/to/project/main.go", "/path/to/project/utils.go", ...]
```

### `Scan(path, apiURL, apiKey, threads, postSize) -> char*`

Performs a complete scan: collects files, generates fingerprints, and sends to API.

**Parameters:**
- `path` (char*): Path to file or directory
- `apiURL` (char*): SCANOSS API URL
- `apiKey` (char*): API key for authentication
- `threads` (int): Number of parallel threads
- `postSize` (int): Maximum POST size in bytes

**Returns:** JSON string with API scan results

```python
results = lib.scan("/path/to/project", "https://api.osskb.org/scan/direct", "", 10, 65536)
# {"file1.go": [{"component": "...", "license": "..."}], ...}
```

## Usage from Python

### Installation

No installation required, you only need the compiled library.

### Basic Example

```python
from example_python import ScanossLib

# Initialize library
lib = ScanossLib()

# Generate fingerprint
fingerprint = lib.generate_wfp("main.go")
print(fingerprint)

# Generate with metadata
metadata = lib.generate_wfp_json("main.go")
print(metadata)

# Collect files
files = lib.collect_files("./cmd")
print(f"Found {len(files)} files")

# Complete API scan
results = lib.scan("./project")
print(results)
```

### Complete Example

Run the example script:

```bash
cd libscanoss
python3 example_python.py
```

## Usage from Node.js

### Installation

```bash
npm install koffi
```

### Example

```javascript
const ScanossLib = require('./scanoss_lib_koffi');

const lib = new ScanossLib();

console.log('Version:', lib.getVersion());

const fingerprint = lib.generateWFP('main.go');
console.log('Fingerprint:', fingerprint);

// Complete API scan
const results = lib.scan('./project');
console.log(results);
```

## Usage from Ruby

```ruby
require 'ffi'

module Scanoss
  extend FFI::Library
  ffi_lib './libscanoss.so'

  attach_function :GetVersion, [], :string
  attach_function :GenerateWFP, [:string], :string
  attach_function :GenerateWFPJSON, [:string], :string
  attach_function :CollectFiles, [:string], :string
  attach_function :Scan, [:string, :string, :string, :int, :int], :string
end

puts "Version: #{Scanoss.GetVersion}"

fingerprint = Scanoss.GenerateWFP('main.go')
puts "Fingerprint: #{fingerprint}"
```

## Memory Management

The `ScanossLib` wrapper classes in Python and Node.js handle memory management automatically.

## Advantages vs CLI

| Aspect | CLI | Shared Library |
|---------|-----|----------------|
| **Overhead** | High (new process) | Low (direct call) |
| **Integration** | Pipes/stdout | Native API |
| **Performance** | Slow for multiple calls | Fast |
| **Memory** | Separate process | Same process |
| **Portability** | Executable binary | FFI in any language |

## Use Cases

1. **IDE/Plugin Integration**: Use from VSCode, Sublime extensions, etc.
2. **Python/Node Services**: Backend that needs fingerprinting
3. **CI/CD Scripts**: Fast analysis without CLI overhead
4. **Desktop Applications**: Electron, PyQt, etc.
5. **High-level Libraries**: Create language-specific wrappers

## Limitations

1. **Thread Safety**: Exported functions must be thread-safe
2. **Garbage Collection**: Go's GC remains active in the library
3. **Size**: Library includes Go runtime (~2-3 MB)
4. **Compatibility**: Requires recompilation for each platform

## Alternatives

If the shared library is not suitable for your use case:

1. **gRPC Server**: For distributed services or microservices
2. **HTTP REST API**: For web integrations
3. **WebAssembly**: For browser usage
4. **CLI + Pipes**: Simpler but with overhead

## Development

### Adding New Functions

1. Add function in `libscanoss.go`:

```go
//export MyNewFunction
func MyNewFunction(param *C.char) *C.char {
    goParam := C.GoString(param)
    result := doSomething(goParam)
    return C.CString(result)
}
```

2. Recompile:

```bash
./build.sh
```

3. Update Python wrapper:

```python
self.lib.MyNewFunction.argtypes = [ctypes.c_char_p]
self.lib.MyNewFunction.restype = ctypes.c_char_p

def my_new_function(self, param):
    return self._call_and_decode(
        self.lib.MyNewFunction,
        param.encode('utf-8')
    )
```

## Testing

```bash
# Compile
cd libscanoss
./build.sh

# Test from Python
python3 example_python.py

# Test from Node.js
node scanoss_lib_koffi.js

# Verify exported symbols (Linux)
nm -D libscanoss.so | grep -i "generate"
```

## Documentation

- [nodejs.md](nodejs.md) - Node.js integration guide
- [python.md](python.md) - Python integration guide
- [comparison.md](comparison.md) - Python vs Node.js comparison
- [integration.md](integration.md) - Multi-language integration guide

## References

- [CGO Documentation](https://pkg.go.dev/cmd/cgo)
- [Python ctypes](https://docs.python.org/3/library/ctypes.html)
- [Koffi (Node.js FFI)](https://github.com/Koromix/koffi)
