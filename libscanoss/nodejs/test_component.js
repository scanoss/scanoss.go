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
 * Test: Scan a real open source component (using Koffi)
 */

const ScanossLib = require('./scanoss_lib_koffi');
const fs = require('fs');

function main() {
    const lib = new ScanossLib();

    console.log('='.repeat(70));
    console.log('TEST: Scan real component (Express.js)');
    console.log('='.repeat(70));
    console.log();

    console.log('📦 Scanning file: /tmp/express.js');
    console.log('   (Known component: Express.js)');
    console.log();

    console.log('⏳ Sending to SCANOSS API...');
    const results = lib.scan('/tmp/express.js');

    console.log('✅ Results received');
    console.log();

    for (const [filePath, matches] of Object.entries(results)) {
        console.log(`📄 File: ${filePath}`);
        console.log();

        if (Array.isArray(matches) && matches.length > 0) {
            matches.slice(0, 3).forEach((match, idx) => {
                if (match.id !== 'none') {
                    console.log(`  ✓ Match #${idx + 1}:`);
                    console.log(`    • Component: ${match.component || 'N/A'}`);
                    console.log(`    • Vendor: ${match.vendor || 'N/A'}`);
                    console.log(`    • Version: ${match.version || 'N/A'}`);
                    console.log(`    • License: ${match.license || 'N/A'}`);
                    console.log(`    • URL: ${match.url || 'N/A'}`);
                    console.log(`    • PURL: ${match.purl ? match.purl[0] : 'N/A'}`);
                    console.log();
                } else {
                    console.log('  ○ No matches found');
                    console.log(`    (Server: ${match.server?.version || 'N/A'})`);
                    console.log();
                }
            });
        }
    }

    fs.writeFileSync('real_component_results_nodejs.json', JSON.stringify(results, null, 2));

    console.log('📝 Complete results saved to: real_component_results_nodejs.json');
    console.log('='.repeat(70));
}

if (require.main === module) {
    try {
        main();
    } catch (error) {
        console.error('\n❌ Error:', error.message);
        console.error('\nMake sure to install dependencies:');
        console.error('  npm install koffi');
        process.exit(1);
    }
}

module.exports = main;
