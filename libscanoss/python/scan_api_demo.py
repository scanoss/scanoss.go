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
Practical example: Scan a complete project and send to SCANOSS API
"""

import sys
sys.path.append('.')

from scanoss_lib import ScanossLib
import json
from datetime import datetime

class ScanossAPIScanner:
    """Scanner that sends results to SCANOSS API"""

    def __init__(self):
        self.lib = ScanossLib()
        print(f"📦 SCANOSS API Scanner v{self.lib.get_version()}")
        print()

    def scan_and_submit(self, path, api_url="https://api.osskb.org/scan/direct",
                        api_key="", threads=10, post_size=65536):
        """
        Scan a project and send to SCANOSS API

        Args:
            path: Path to the project
            api_url: SCANOSS API URL
            api_key: API key (optional)
            threads: Number of parallel threads
            post_size: Maximum POST size

        Returns:
            Dict with API results
        """
        print(f"🔍 Scanning project: {path}")
        print(f"   API URL: {api_url}")
        print(f"   Algorithm: WFP")
        print(f"   Hash: CRC64")
        print(f"   Threads: {threads}")
        print(f"   POST size: {post_size} bytes")
        print()

        print("⏳ Sending to SCANOSS API...")
        print("   (This includes: collection, fingerprinting, batching and submission)")
        print()

        try:
            # Call Scan() function that does ALL the process
            results = self.lib.scan(
                path=path,
                api_url=api_url,
                api_key=api_key,
                threads=threads,
                post_size=post_size
            )

            if 'error' in results:
                print(f"❌ API Error: {results['error']}")
                return None

            print("✅ Scan completed successfully")
            return results

        except Exception as e:
            print(f"❌ Error during scan: {e}")
            import traceback
            traceback.print_exc()
            return None

    def generate_report(self, api_results, output_file=None):
        """
        Generate a report from API results

        Args:
            api_results: API results
            output_file: Output file (optional)
        """
        print()
        print("📊 Generating results report...")
        print()

        # Count scanned files
        total_files = len(api_results) if isinstance(api_results, dict) else 0

        # Summary
        print("=" * 70)
        print("SCAN REPORT - API RESULTS")
        print("=" * 70)
        print(f"Date: {datetime.now().isoformat()}")
        print(f"Total scanned files: {total_files}")
        print()

        # Show some results
        if isinstance(api_results, dict):
            print("ANALYZED FILES:")
            count = 0
            for file_path, matches in list(api_results.items())[:5]:
                count += 1
                file_name = file_path.split('/')[-1] if '/' in file_path else file_path
                print(f"\n  {count}. {file_name}")

                if isinstance(matches, list) and len(matches) > 0:
                    match = matches[0]
                    if 'id' in match and match['id'] != 'none':
                        print(f"     ✓ Match found:")
                        if 'purl' in match:
                            print(f"       - Component: {match.get('purl', ['N/A'])[0]}")
                        if 'vendor' in match:
                            print(f"       - Vendor: {match.get('vendor', 'N/A')}")
                        if 'component' in match:
                            print(f"       - Component: {match.get('component', 'N/A')}")
                        if 'version' in match:
                            print(f"       - Version: {match.get('version', 'N/A')}")
                        if 'license' in match:
                            print(f"       - License: {match.get('license', 'N/A')}")
                    else:
                        print(f"     ○ No matches")
                else:
                    print(f"     ○ No results")

            if total_files > 5:
                print(f"\n  ... and {total_files - 5} more files")

        print()

        # Save to file if specified
        if output_file:
            with open(output_file, 'w') as f:
                json.dump(api_results, f, indent=2)
            print(f"✓ Complete results saved to: {output_file}")
            print()

        print("=" * 70)

def main():
    """Usage example"""
    scanner = ScanossAPIScanner()

    # Scan cmd/ directory and send to API
    print("🚀 Starting complete scan with API submission...")
    print()

    results = scanner.scan_and_submit(
        path="../cmd",
        api_url="https://api.osskb.org/scan/direct",
        api_key="",  # Empty for public API
        threads=10,
        post_size=65536
    )

    if results:
        # Generate report
        scanner.generate_report(results, output_file="api_scan_results.json")

        print("\n💡 API results include:")
        print("   • Detected open source components")
        print("   • Identified licenses")
        print("   • Known vulnerabilities")
        print("   • Dependency information")
        print()
        print("📝 Complete results in: api_scan_results.json")
    else:
        print("\n⚠️  Could not get API results")

if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print("\n\n⚠️  Scan cancelled by user")
        sys.exit(0)
    except Exception as e:
        print(f"\n❌ Error: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)
