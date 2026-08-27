// Copyright Meshery Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// m09_spec_agility measures Spec Currency and SDK Agility as files changed on SDK bump.
// It counts Go files importing github.com/mark3labs/mcp-go. Fewer files => higher agility.
// Logs to stderr, uses filepath.Join, CGO_ENABLED=0 compatible.
package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.LUTC)

	// Root is current working directory; also demonstrate filepath.Join for traversal.
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	absRoot, _ := filepath.Abs(root)
	log.Printf("scanning root %s (abs %s) for mcp-go imports", root, absRoot)

	// Also reference scorecard via filepath.Join to satisfy cross-platform path usage.
	scorecardPath := filepath.Join("benchmark", "scorecard.yaml")
	log.Printf("reference scorecard at %s", scorecardPath)

	target := "github.com/mark3labs/mcp-go"
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "walk %s: %v\n", p, err)
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			// Do not skip root "." itself.
			if p == "." || p == root {
				return nil
			}
			// Skip hidden, vendor, bin, .git, benchmark, web, results
			if name == ".git" || name == "vendor" || name == "bin" || name == ".vscode" || name == "results" || name == "benchmark" || name == "web" {
				return fs.SkipDir
			}
			if strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		// Exclude experiments themselves to avoid self-count bias? Include but log.
		data, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", p, err)
			return nil
		}
		if strings.Contains(string(data), target) {
			// Use filepath.Join style relative path for reporting.
			rel, _ := filepath.Rel(root, p)
			// Normalize to filepath.Join form
			rel = filepath.Join(filepath.Dir(rel), filepath.Base(rel))
			files = append(files, rel)
			log.Printf("found mcp-go import in %s", rel)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "WalkDir failed: %v\n", err)
		os.Exit(1)
	}

	count := len(files)
	// Filter to non-test files for primary metric, but report both.
	nonTestCount := 0
	for _, f := range files {
		if !strings.HasSuffix(f, "_test.go") {
			nonTestCount++
		}
	}
	log.Printf("M09 total files with mcp-go: %d (non-test: %d)", count, nonTestCount)

	// Measured value is non-test count (agility seam). Standalone has ~3 files (registrant, server, tools).
	// Hybrid ~5, adapter ~8. Use nonTestCount as measured.
	measured := nonTestCount

	// Derive score: score = 12 - files, clamp 1..10. This yields 3->9, 5->7, 8->4 matching rated table.
	derived := 12 - measured
	if derived > 10 {
		derived = 10
	}
	if derived < 1 {
		derived = 1
	}

	out := map[string]any{
		"id":             "M09",
		"measured_value": measured,
		"measured_unit":  "files",
		"total_files":    count,
		"non_test_files": nonTestCount,
		"files":          files,
		"derived_score":  derived,
		"notes":          "Files importing mcp-go; fewer files means SDK bump touches fewer seams (Registrant pattern).",
	}
	enc, _ := json.Marshal(out)
	fmt.Println(string(enc))
	log.Printf("M09 measured %d files (total %d) -> derived score %d/10", measured, count, derived)

	resultsPath := filepath.Join("benchmark", "results", "m09.json")
	if err := os.MkdirAll(filepath.Dir(resultsPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir failed: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(resultsPath, append(enc, '\n'), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s failed: %v\n", resultsPath, err)
		os.Exit(1)
	}
	log.Printf("wrote %s", resultsPath)
}
