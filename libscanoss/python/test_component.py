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
Test: Scan a real open source component
"""

import sys
sys.path.append('.')

from scanoss_lib import ScanossLib
import json

def main():
    lib = ScanossLib()

    print("=" * 70)
    print("TEST: Scan real component (Express.js)")
    print("=" * 70)
    print()

    print("📦 Scanning file: /tmp/express.js")
    print("   (Known component: Express.js)")
    print()

    print("⏳ Sending to SCANOSS API...")
    results = lib.scan(
        path="/tmp/express.js",
        api_url="https://api.scanoss.com",
        api_key="",
        threads=10,
        post_size=65536
    )

    print("✅ Results received")
    print()

    # Show results
    if isinstance(results, dict):
        for file_path, matches in results.items():
            print(f"📄 File: {file_path}")
            print()

            if isinstance(matches, list) and len(matches) > 0:
                for i, match in enumerate(matches[:3], 1):  # Show first 3 matches
                    if match.get('id') != 'none':
                        print(f"  ✓ Match #{i}:")
                        print(f"    • Component: {match.get('component', 'N/A')}")
                        print(f"    • Vendor: {match.get('vendor', 'N/A')}")
                        print(f"    • Version: {match.get('version', 'N/A')}")
                        print(f"    • License: {match.get('license', 'N/A')}")
                        print(f"    • URL: {match.get('url', 'N/A')}")
                        print(f"    • PURL: {match.get('purl', ['N/A'])[0] if 'purl' in match else 'N/A'}")
                        print()
                    else:
                        print(f"  ○ No matches found")
                        print(f"    (Server: {match.get('server', {}).get('version', 'N/A')})")
                        print()

    # Save complete results
    with open('real_component_results.json', 'w') as f:
        json.dump(results, f, indent=2)

    print("📝 Complete results saved to: real_component_results.json")
    print("=" * 70)

if __name__ == "__main__":
    main()
