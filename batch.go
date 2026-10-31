package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// renameRule describes a FlowVision batch rename. The fields apply in order:
// replace, then format, then prefix, then suffix. The extension stays unless
// the format itself contains {ext}.
type renameRule struct {
	Mode        string
	Pattern     string
	Find        string
	Replace     string
	Prefix      string
	Suffix      string
	StartNumber int
	AddPosition string
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
		folder := filepath.Base(dir)
		if folder == "." || folder == "" || folder == string(filepath.Separator) {
			folder = "Folder"
		}
		name := composeBatchName(base, ext, folder, n, rule, now)
		n++
		if name == "" || name == "." || name == ".." || strings.Contains(name, "/") {
			out[path] = path
			continue
		}
		out[path] = filepath.Join(dir, name)
	}
	return out
}

// composeBatchName applies replace, format, prefix and suffix, in that order.
func composeBatchName(base, ext, folder string, index int, rule renameRule, now time.Time) string {
	if rule.Mode == "add" {
		if rule.AddPosition == "suffix" {
			return base + rule.Pattern + ext
		}
		return rule.Pattern + base + ext
	}
	if rule.Find != "" {
		base = strings.ReplaceAll(base, rule.Find, rule.Replace)
	}
	format := strings.TrimSpace(rule.Pattern)
	if format == "" {
		format = "{name}"
	}
	name := format
	name = strings.ReplaceAll(name, "{date}", now.Format("2006-01-02"))
	name = strings.ReplaceAll(name, "{time}", now.Format("15-04-05"))
	name = strings.ReplaceAll(name, "{name}", base)
	name = strings.ReplaceAll(name, "{folder}", folder)
	name = strings.ReplaceAll(name, "{ext}", strings.TrimPrefix(ext, "."))
	// Count tokens run after the name is substituted, as FlowVision does,
	// so a file whose name contains {index} is numbered too.
	name = expandCountTokens(name, index)
	if rule.Mode != "replace" {
		name = rule.Prefix + name + rule.Suffix
	}
	name = strings.TrimSpace(name)
	if !strings.Contains(format, "{ext}") {
		name += ext
	}
	return name
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

// expandCountTokens replaces {num}, {index} and their padded forms.
func expandCountTokens(s string, n int) string {
	for {
		start, token := earliestCountToken(s)
		if start < 0 {
			return s
		}
		if token == "{num}" || token == "{index}" {
			s = s[:start] + fmt.Sprintf("%d", n) + s[start+len(token):]
			continue
		}
		rest := s[start+len(token):]
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
		s = s[:start] + fmt.Sprintf("%0*d", width, n) + s[start+len(token)+end+1:]
	}
}

func earliestCountToken(s string) (int, string) {
	best := -1
	kind := ""
	for _, token := range []string{"{num:", "{index:", "{num}", "{index}"} {
		i := strings.Index(s, token)
		if i >= 0 && (best < 0 || i < best) {
			best = i
			kind = token
		}
	}
	return best, kind
}

// replaceNum expands {num:N} with a zero-padded number.
func replaceNum(s string, n int) string {
	return expandCountTokens(s, n)
}
