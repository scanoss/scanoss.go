#!/usr/bin/env node

// SPDX-License-Identifier: MIT
/*
 * Copyright (c) 2026, SCANOSS
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in
 * all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
 * THE SOFTWARE.
 */

/**
 * SCANOSS Library wrapper using Koffi (modern FFI for Node.js)
 *
 * Installation:
 *   npm install koffi
 */

const koffi = require('koffi');
const path = require('path');
const os = require('os');

class ScanossLib {
    /**
     * Initialize the SCANOSS library
     *
     * @param {string} libPath - Path to the shared library (optional)
     */
    constructor(libPath = null) {
        if (!libPath) {
            libPath = this._getDefaultLibPath();
        }

        // Load the shared library
        this.lib = koffi.load(libPath);

        // Define function signatures
        this.GetVersion = this.lib.func('GetVersion', 'string', []);
        this.GenerateWFP = this.lib.func('GenerateWFP', 'string', ['string']);
        this.GenerateWFPJSON = this.lib.func('GenerateWFPJSON', 'string', ['string']);
        this.CollectFiles = this.lib.func('CollectFiles', 'string', ['string']);
        this.Scan = this.lib.func('Scan', 'string', ['string', 'string', 'string', 'int', 'int']);
    }

    /**
     * Auto-detect library path based on platform
     * @private
     */
    _getDefaultLibPath() {
        const platform = os.platform();
        // Library is in ../core/ directory
        const coreDir = path.join(__dirname, '..', 'core');

        if (platform === 'linux') {
            return path.join(coreDir, 'libscanoss.so');
        } else if (platform === 'darwin') {
            return path.join(coreDir, 'libscanoss.dylib');
        } else if (platform === 'win32') {
            return path.join(coreDir, 'libscanoss.dll');
        } else {
            throw new Error(`Unsupported platform: ${platform}`);
        }
    }

    /**
     * Get library version
     *
     * @returns {string} Library version
     */
    getVersion() {
        return this.GetVersion();
    }

    /**
     * Generate WFP fingerprint for a file
     *
     * @param {string} filePath - Path to the file
     * @returns {string} Fingerprint string
     */
    generateWFP(filePath) {
        return this.GenerateWFP(filePath);
    }

    /**
     * Generate WFP fingerprint with full metadata as JSON
     *
     * @param {string} filePath - Path to the file
     * @returns {Object} Dictionary with fingerprint metadata
     */
    generateWFPJSON(filePath) {
        const jsonStr = this.GenerateWFPJSON(filePath);
        return jsonStr ? JSON.parse(jsonStr) : {};
    }

    /**
     * Collect all valid files from a directory
     *
     * @param {string} dirPath - Path to file or directory
     * @returns {Array<string>} List of file paths
     */
    collectFiles(dirPath) {
        const jsonStr = this.CollectFiles(dirPath);
        return jsonStr ? JSON.parse(jsonStr) : [];
    }

    /**
     * Perform a complete scan: collect files, generate fingerprints, and send to API
     *
     * @param {string} path - Path to file or directory to scan
     * @param {string} apiUrl - SCANOSS API URL (default: https://api.osskb.org/scan/direct)
     * @param {string} apiKey - API key for authentication (default: empty)
     * @param {number} threads - Number of parallel threads (default: 10)
     * @param {number} postSize - Maximum POST size in bytes (default: 65536)
     * @returns {Object} Dictionary with API scan results
     */
    scan(path, apiUrl = 'https://api.osskb.org/scan/direct', apiKey = '',
         threads = 10, postSize = 65536) {
        const jsonStr = this.Scan(
            path,
            apiUrl,
            apiKey,
            threads,
            postSize
        );
        return jsonStr ? JSON.parse(jsonStr) : {};
    }
}

// Example usage
function main() {
    console.log('SCANOSS Library - Node.js Example (Koffi)');
    console.log('='.repeat(50));
    console.log();

    try {
        // Initialize library
        const lib = new ScanossLib();

        // Get version
        const version = lib.getVersion();
        console.log(`Library version: ${version}`);
        console.log();

        // Example 1: Generate fingerprint for a single file
        const testFile = '../../cmd/scanoss-cli/main.go';
        console.log(`Example 1: Generate WFP for ${testFile}`);
        const fingerprint = lib.generateWFP(testFile);
        if (fingerprint) {
            console.log(`Fingerprint preview: ${fingerprint.substring(0, 200)}...`);
        }
        console.log();

        // Example 2: Generate fingerprint with metadata
        console.log('Example 2: Generate WFP with metadata');
        const metadata = lib.generateWFPJSON(testFile);
        console.log('Metadata:', JSON.stringify(metadata, null, 2));
        console.log();

        // Example 3: Collect files from directory
        const testDir = '../../cmd';
        console.log(`Example 3: Collect files from ${testDir}`);
        const files = lib.collectFiles(testDir);
        console.log(`Found ${files.length} files`);
        if (files.length > 0) {
            files.slice(0, 5).forEach((file, idx) => {
                console.log(`  ${idx + 1}. ${file}`);
            });
            if (files.length > 5) {
                console.log(`  ... and ${files.length - 5} more`);
            }
        }
        console.log();

        console.log('Example 4: Complete scan with API (skipped - requires API)');
        console.log('   Uncomment to test with real API');
        console.log();
        // const scanResults = lib.scan('../../cmd');
        // console.log('Scan results:', JSON.stringify(scanResults, null, 2));

    } catch (error) {
        console.error('Error:', error.message);
        console.error('\nMake sure to install dependencies:');
        console.error('  npm install koffi');
        process.exit(1);
    }
}

// Run if called directly
if (require.main === module) {
    main();
}

// Export for use as module
module.exports = ScanossLib;
