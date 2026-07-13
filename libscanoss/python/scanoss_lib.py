#!/usr/bin/env python3

# SPDX-License-Identifier: MIT
#
# Copyright (c) 2026, SCANOSS
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in
# all copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
# THE SOFTWARE.

"""
Example of using libscanoss.so from Python using ctypes
"""

import ctypes
import json
import platform
from pathlib import Path

class ScanossLib:
    """Python wrapper for libscanoss shared library"""

    def __init__(self, lib_path=None):
        """
        Initialize the SCANOSS library

        Args:
            lib_path: Path to the shared library. If None, auto-detects based on platform.
        """
        if lib_path is None:
            lib_path = self._get_default_lib_path()

        self.lib = ctypes.CDLL(str(lib_path))

        # Define function signatures

        # char* GenerateWFP(char* filePath)
        self.lib.GenerateWFP.argtypes = [ctypes.c_char_p]
        self.lib.GenerateWFP.restype = ctypes.c_char_p

        # char* GenerateWFPJSON(char* filePath)
        self.lib.GenerateWFPJSON.argtypes = [ctypes.c_char_p]
        self.lib.GenerateWFPJSON.restype = ctypes.c_char_p

        # char* CollectFiles(char* path)
        self.lib.CollectFiles.argtypes = [ctypes.c_char_p]
        self.lib.CollectFiles.restype = ctypes.c_char_p


        # char* GetVersion()
        self.lib.GetVersion.argtypes = []
        self.lib.GetVersion.restype = ctypes.c_char_p

    def _get_default_lib_path(self):
        """Auto-detect library path based on platform"""
        # Library is in ../core/ directory
        core_dir = Path(__file__).parent.parent / "core"

        system = platform.system()
        if system == "Linux":
            return core_dir / "libscanoss.so"
        elif system == "Darwin":
            return core_dir / "libscanoss.dylib"
        elif system == "Windows":
            return core_dir / "libscanoss.dll"
        else:
            raise RuntimeError(f"Unsupported platform: {system}")

    def _call_and_decode(self, func, *args):
        """Call a C function and free the returned string"""
        result = func(*args)
        if result:
            value = result.decode('utf-8')
            return value
        return ""

    def get_version(self):
        """Get library version"""
        return self._call_and_decode(self.lib.GetVersion)

    def generate_wfp(self, file_path):
        """
        Generate WFP fingerprint for a file

        Args:
            file_path: Path to the file

        Returns:
            Fingerprint string
        """
        return self._call_and_decode(
            self.lib.GenerateWFP,
            file_path.encode('utf-8')
        )

    def generate_wfp_json(self, file_path):
        """
        Generate WFP fingerprint with full metadata as JSON

        Args:
            file_path: Path to the file

        Returns:
            Dictionary with fingerprint metadata
        """
        json_str = self._call_and_decode(
            self.lib.GenerateWFPJSON,
            file_path.encode('utf-8')
        )
        return json.loads(json_str) if json_str else {}

    def collect_files(self, path):
        """
        Collect all valid files from a directory

        Args:
            path: Path to file or directory

        Returns:
            List of file paths
        """
        json_str = self._call_and_decode(
            self.lib.CollectFiles,
            path.encode('utf-8')
        )
        return json.loads(json_str) if json_str else []
    def scan(self, path, api_url="https://osskb.org/api/scan/direct", api_key="",
             threads=10, post_size=65536):
        """
        Perform a complete scan: collect files, generate fingerprints, and send to API

        Args:
            path: Path to file or directory to scan
            api_url: SCANOSS API URL
            api_key: API key for authentication
            threads: Number of parallel threads
            post_size: Maximum POST size in bytes

        Returns:
            Dictionary with API scan results
        """
        # Configure Scan function signature
        self.lib.Scan.argtypes = [
            ctypes.c_char_p,  # path
            ctypes.c_char_p,  # apiURL
            ctypes.c_char_p,  # apiKey
            ctypes.c_int,     # threads
            ctypes.c_int      # postSize
        ]
        self.lib.Scan.restype = ctypes.c_char_p

        json_str = self._call_and_decode(
            self.lib.Scan,
            path.encode('utf-8'),
            api_url.encode('utf-8'),
            api_key.encode('utf-8'),
            ctypes.c_int(threads),
            ctypes.c_int(post_size)
        )
        return json.loads(json_str) if json_str else {}



def main():
    """Example usage"""
    print("SCANOSS Library - Python Example")
    print("=" * 50)

    # Initialize library
    lib = ScanossLib()

    # Get version
    version = lib.get_version()
    print(f"Library version: {version}")
    print()

    # Example 1: Generate fingerprint for a single file
    test_file = "../../cmd/scanoss/main.go"  # Adjust path as needed
    print(f"Example 1: Generate WFP for {test_file}")
    fingerprint = lib.generate_wfp(test_file)
    if fingerprint:
        print(f"Fingerprint preview: {fingerprint[:200]}...")
    print()

    # Example 2: Generate fingerprint with metadata
    print(f"Example 2: Generate WFP with metadata")
    metadata = lib.generate_wfp_json(test_file)
    print(f"Metadata: {json.dumps(metadata, indent=2)}")
    print()

    # Example 3: Collect files from directory
    test_dir = "../../cmd"  # Adjust path as needed
    print(f"Example 3: Collect files from {test_dir}")
    files = lib.collect_files(test_dir)
    print(f"Found {len(files)} files")
    if files and isinstance(files, list):
        for i, f in enumerate(files[:5], 1):
            print(f"  {i}. {f}")
        if len(files) > 5:
            print(f"  ... and {len(files) - 5} more")


    # Example 4: Complete scan with API
    print("Example 4: Complete scan with API (skipped - requires API)")
    # Uncomment to test with real API:
    # scan_results = lib.scan(
    #     path="../../cmd",
    #     api_url="https://osskb.org/api/scan/direct",
    #     threads=10,
    #     post_size=65536
    # )
    # print(f"Scan results: {json.dumps(scan_results, indent=2)}")
    print()


if __name__ == "__main__":
    main()
