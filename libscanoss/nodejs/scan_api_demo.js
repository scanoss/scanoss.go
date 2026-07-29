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
 * Practical example: Scan a complete project and send to SCANOSS API (using Koffi)
 */

const ScanossLib = require('./scanoss_lib_koffi');
const fs = require('fs');

class ScanossAPIScanner {
    constructor() {
        this.lib = new ScanossLib();
        console.log(`📦 SCANOSS API Scanner v${this.lib.getVersion()}`);
        console.log();
    }

    async scanAndSubmit(path, options = {}) {
        const {
            apiUrl = 'https://api.scanoss.com',
            apiKey = '',
            threads = 10,
            postSize = 65536
        } = options;

        console.log(`🔍 Scanning project: ${path}`);
        console.log(`   API URL: ${apiUrl}`);
        console.log(`   Algorithm: WFP`);
        console.log(`   Hash: CRC64`);
        console.log(`   Threads: ${threads}`);
        console.log(`   POST size: ${postSize} bytes`);
        console.log();

        console.log('⏳ Sending to SCANOSS API...');
        console.log('   (This includes: collection, fingerprinting, batching and submission)');
        console.log();

        try {
            const results = this.lib.scan(path, apiUrl, apiKey, threads, postSize);

            if (results.error) {
                console.log(`❌ API Error: ${results.error}`);
                return null;
            }

            console.log('✅ Scan completed successfully');
            return results;

        } catch (error) {
            console.error(`❌ Error during scan: ${error.message}`);
            console.error(error.stack);
            return null;
        }
    }

    generateReport(apiResults, outputFile = null) {
        console.log();
        console.log('📊 Generating results report...');
        console.log();

        const totalFiles = Object.keys(apiResults).length;

        console.log('='.repeat(70));
        console.log('SCAN REPORT - API RESULTS');
        console.log('='.repeat(70));
        console.log(`Date: ${new Date().toISOString()}`);
        console.log(`Total scanned files: ${totalFiles}`);
        console.log();

        console.log('ANALYZED FILES:');
        let count = 0;
        const entries = Object.entries(apiResults).slice(0, 5);

        for (const [filePath, matches] of entries) {
            count++;
            const fileName = filePath.split('/').pop();
            console.log(`\n  ${count}. ${fileName}`);

            if (Array.isArray(matches) && matches.length > 0) {
                const match = matches[0];
                if (match.id && match.id !== 'none') {
                    console.log('     ✓ Match found:');
                    if (match.purl) {
                        console.log(`       - Component: ${match.purl[0] || 'N/A'}`);
                    }
                    if (match.vendor) {
                        console.log(`       - Vendor: ${match.vendor}`);
                    }
                    if (match.component) {
                        console.log(`       - Component: ${match.component}`);
                    }
                    if (match.version) {
                        console.log(`       - Version: ${match.version}`);
                    }
                    if (match.license) {
                        console.log(`       - License: ${match.license}`);
                    }
                } else {
                    console.log('     ○ No matches');
                }
            } else {
                console.log('     ○ No results');
            }
        }

        if (totalFiles > 5) {
            console.log(`\n  ... and ${totalFiles - 5} more files`);
        }

        console.log();

        if (outputFile) {
            fs.writeFileSync(outputFile, JSON.stringify(apiResults, null, 2));
            console.log(`✓ Complete results saved to: ${outputFile}`);
            console.log();
        }

        console.log('='.repeat(70));
    }
}

async function main() {
    console.log('🚀 Starting complete scan with API submission...');
    console.log();

    const scanner = new ScanossAPIScanner();

    const results = await scanner.scanAndSubmit('../cmd', {
        apiUrl: 'https://api.scanoss.com',
        apiKey: '',
        threads: 10,
        postSize: 65536
    });

    if (results) {
        scanner.generateReport(results, 'api_scan_results_nodejs.json');

        console.log('\n💡 API results include:');
        console.log('   • Detected open source components');
        console.log('   • Identified licenses');
        console.log('   • Known vulnerabilities');
        console.log('   • Dependency information');
        console.log();
        console.log('📝 Complete results in: api_scan_results_nodejs.json');
    } else {
        console.log('\n⚠️  Could not get API results');
    }
}

if (require.main === module) {
    main().catch(error => {
        console.error('\n❌ Error:', error.message);
        process.exit(1);
    });
}

module.exports = ScanossAPIScanner;
