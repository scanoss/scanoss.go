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

package parsers

import (
	"strings"
	"testing"
)

// TestParsePomXMLSourceLocation verifies the single-pass collect-then-resolve maven parser.
// The fixture pom contains ALL required scenarios from the design:
//   - A <dependencies> block with multiple deps (one multi-line, one single-line).
//   - A <dependencyManagement> block with a dep sharing a coordinate with a project-level dep
//     (proves only project-level deps are emitted).
//   - A <plugin> <dependency> that must NOT appear in output.
//   - A duplicate coordinate in <dependencies> — output contains it once (first-occurrence line).
//   - A ${property} version where the property is defined AFTER the <dependency> that uses it
//     (proves collect-then-resolve).
func TestParsePomXMLSourceLocation(t *testing.T) {
	t.Parallel()

	// pom.xml with:
	// - <dependencies>: spring-core (multi-line, line 7), logback-classic (single-line, line 14),
	//   and a duplicate of spring-core (line 17) that should be deduped away.
	// - <dependencyManagement>: spring-core (should NOT appear in output)
	// - <build><plugins><plugin><dependencies>: a dep that should NOT appear in output
	// - <properties> defined AFTER the dep that uses ${spring.version} (proves collect-then-resolve)
	pom := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <groupId>com.example</groupId>
  <artifactId>myapp</artifactId>
  <version>1.0.0</version>
  <dependencies>
    <dependency>
      <groupId>org.springframework</groupId>
      <artifactId>spring-core</artifactId>
      <version>${spring.version}</version>
    </dependency>
    <dependency><groupId>ch.qos.logback</groupId><artifactId>logback-classic</artifactId><version>1.4.0</version></dependency>
    <dependency>
      <groupId>org.springframework</groupId>
      <artifactId>spring-core</artifactId>
      <version>${spring.version}</version>
    </dependency>
  </dependencies>
  <dependencyManagement>
    <dependencies>
      <dependency>
        <groupId>org.springframework</groupId>
        <artifactId>spring-core</artifactId>
        <version>6.0.0</version>
      </dependency>
    </dependencies>
  </dependencyManagement>
  <build>
    <plugins>
      <plugin>
        <groupId>org.apache.maven.plugins</groupId>
        <artifactId>maven-compiler-plugin</artifactId>
        <dependencies>
          <dependency>
            <groupId>com.example</groupId>
            <artifactId>plugin-dep</artifactId>
            <version>1.0</version>
          </dependency>
        </dependencies>
      </plugin>
    </plugins>
  </build>
  <properties>
    <spring.version>6.0.11</spring.version>
  </properties>
</project>
`
	result, err := ParsePomXML([]byte(pom), "pom.xml")
	if err != nil {
		t.Fatalf("ParsePomXML error: %v", err)
	}

	// Expected: only spring-core and logback-classic from <dependencies>
	// spring-core appears once (deduped), logback-classic appears once.
	if len(result.Purls) != 2 {
		t.Fatalf("got %d purls, want 2; purls: %+v", len(result.Purls), result.Purls)
	}

	// Build a map for easier assertions
	purlMap := make(map[string]LocalPurl)
	for _, p := range result.Purls {
		purlMap[p.Purl] = p
	}

	// spring-core: ${spring.version} should resolve to 6.0.11 (defined AFTER the dep)
	springPurl := "pkg:maven/org.springframework/spring-core@6.0.11"
	spring, ok := purlMap[springPurl]
	if !ok {
		t.Fatalf("expected purl %q in output, got purls: %+v", springPurl, result.Purls)
	}
	// Line should be the start of the first <dependency> block (line 7 in the pom)
	if spring.Line != 7 {
		t.Errorf("spring-core Line: got %d, want 7", spring.Line)
	}
	// DeclaredText should be the full multi-line block, trimmed exactly as the parser produces it.
	wantSpringText := "<dependency>\n      <groupId>org.springframework</groupId>\n      <artifactId>spring-core</artifactId>\n      <version>${spring.version}</version>\n    </dependency>"
	if spring.DeclaredText != wantSpringText {
		t.Errorf("spring-core DeclaredText:\ngot:  %q\nwant: %q", spring.DeclaredText, wantSpringText)
	}

	// logback-classic: single-line dep
	logbackPurl := "pkg:maven/ch.qos.logback/logback-classic@1.4.0"
	logback, ok := purlMap[logbackPurl]
	if !ok {
		t.Fatalf("expected purl %q in output, got purls: %+v", logbackPurl, result.Purls)
	}
	if logback.Line != 12 {
		t.Errorf("logback-classic Line: got %d, want 12", logback.Line)
	}
	wantLogbackText := "<dependency><groupId>ch.qos.logback</groupId><artifactId>logback-classic</artifactId><version>1.4.0</version></dependency>"
	if logback.DeclaredText != wantLogbackText {
		t.Errorf("logback-classic DeclaredText:\ngot:  %q\nwant: %q", logback.DeclaredText, wantLogbackText)
	}

	// Prove that dependencyManagement dep and plugin dep are NOT in output
	for _, p := range result.Purls {
		if strings.Contains(p.Purl, "plugin-dep") {
			t.Errorf("plugin dep should not be in output, got: %q", p.Purl)
		}
	}

	// Prove the duplicate spring-core appears only ONCE (dedup)
	springCount := 0
	for _, p := range result.Purls {
		if strings.Contains(p.Purl, "spring-core") {
			springCount++
		}
	}
	if springCount != 1 {
		t.Errorf("spring-core should appear exactly once (dedup), got %d times", springCount)
	}
}

// TestParsePomXMLZeroLine verifies that a pom with no project-level dependencies returns empty output.
func TestParsePomXMLZeroLine(t *testing.T) {
	t.Parallel()

	pom := `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <groupId>com.example</groupId>
  <artifactId>myapp</artifactId>
  <version>1.0.0</version>
</project>
`
	result, err := ParsePomXML([]byte(pom), "pom.xml")
	if err != nil {
		t.Fatalf("ParsePomXML error: %v", err)
	}
	if len(result.Purls) != 0 {
		t.Errorf("expected 0 purls for pom with no deps, got %d", len(result.Purls))
	}
}
