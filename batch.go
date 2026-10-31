package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// renameRule describes a FlowVision-style batch rename rule.
type renameRule struct {
	Mode        string // format, replace, add
	Pattern     string // e.g. "IMG_{num:3}_{date}"
	Find        string
	Replace     string
	StartNumber int
	AddPosition string // prefix, suffix
}

// previewRenames maps current absolute paths to proposed new names in their dir.
func previewRenames(paths []string, rule renameRule, now time.Time) map[string]string {
	out := make(map[string]string, len(paths))
	n := rule.StartNumber
	if n <= 0 {
		n = 1
	}
	for _, path := range paths {
		dir := filepath.Dir(path)
		ext := filepath.Ext(path)
		base := strings.TrimSuffix(filepath.Base(path), ext)
		var name string
		switch rule.Mode {
		case "replace":
			name = strings.ReplaceAll(base, rule.Find, rule.Replace)
		case "add":
			if rule.AddPosition == "suffix" {
				name = base + rule.Pattern
			} else {
				name = rule.Pattern + base
			}
		default: // format
			name = rule.Pattern
			name = replaceNum(name, n)
			name = strings.ReplaceAll(name, "{date}", now.Format("2006-01-02"))
			name = strings.ReplaceAll(name, "{time}", now.Format("15-04-05"))
			name = strings.ReplaceAll(name, "{name}", base)
			name = strings.ReplaceAll(name, "{ext}", strings.TrimPrefix(ext, "."))
			n++
		}
		out[path] = filepath.Join(dir, name+ext)
	}
	return out
}

func numWidth(pattern string) int {
	i := strings.Index(pattern, "{num:")
	if i < 0 {
		return 1
	}
	rest := pattern[i+5:]
	end := strings.Index(rest, "}")
	if end < 0 {
		return 1
	}
	n := 0
	for _, r := range rest[:end] {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	if n <= 0 {
		return 1
	}
	return n
}

// applyRenames performs one two-phase rename for the whole mapping, so files
// can swap names without a temporary collision.
func applyRenames(mapping map[string]string) error {
	var pairs []pathPair
	for src, dst := range mapping {
		if src != dst {
			pairs = append(pairs, pathPair{From: src, To: dst})
		}
	}
	return applyRenameMappings(pairs)
}

// replaceNum expands {num:N} with a zero-padded number.
func replaceNum(s string, n int) string {
	for {
		start := strings.Index(s, "{num:")
		if start < 0 {
			return s
		}
		rest := s[start+5:]
		end := strings.Index(rest, "}")
		if end < 0 {
			return s
		}
		width := 0
		for _, r := range rest[:end] {
			if r < '0' || r > '9' {
				break
			}
			width = width*10 + int(r-'0')
		}
		if width <= 0 {
			width = 1
		}
		replacement := fmt.Sprintf("%0*d", width, n)
		s = s[:start] + replacement + s[start+5+end+1:]
	}
}
