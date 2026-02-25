// Command genmanifest scans the images/chars directory and generates _manifest.json
// from the filenames. Files are named like "A.1.png" where "A" is the character.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const charsDir = "images/chars"

type entry struct {
	File string `json:"file"`
	Char string `json:"char"`
}

func main() {
	files, err := os.ReadDir(charsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading dir: %v\n", err)
		os.Exit(1)
	}

	var entries []entry
	for _, f := range files {
		name := f.Name()
		if !strings.HasSuffix(name, ".png") || strings.HasPrefix(name, "_") {
			continue
		}

		// Parse "X.N.png" -> char is everything before the first "."
		// But for ".." (dot char), the file is "..1.png" -> char is "."
		base := strings.TrimSuffix(name, ".png")

		// Find the last dot - the number separator.
		lastDot := strings.LastIndex(base, ".")
		if lastDot < 0 {
			continue
		}

		ch := base[:lastDot]
		if ch == "" {
			// Handle "..1.png" -> the char before lastDot is empty, meaning char is "."
			ch = "."
		}

		entries = append(entries, entry{File: name, Char: ch})
	}

	// Sort by char, then file.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Char != entries[j].Char {
			return entries[i].Char < entries[j].Char
		}
		return entries[i].File < entries[j].File
	})

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshalling: %v\n", err)
		os.Exit(1)
	}

	outPath := filepath.Join(charsDir, "_manifest.json")
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing: %v\n", err)
		os.Exit(1)
	}

	// Count unique chars.
	charSet := make(map[string]int)
	for _, e := range entries {
		charSet[e.Char]++
	}

	fmt.Printf("Wrote %s: %d entries, %d unique characters\n", outPath, len(entries), len(charSet))
	for ch, count := range charSet {
		fmt.Printf("  %q: %d variants\n", ch, count)
	}
}
