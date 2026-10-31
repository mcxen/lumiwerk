package main

import (
	"os/exec"
	"strings"
)

func finderTagsMDLS(path string) []string {
	out, err := exec.Command("/usr/bin/mdls", "-name", "kMDItemUserTags", "-raw", path).Output()
	if err != nil || len(out) == 0 {
		return nil
	}
	s := strings.TrimSpace(string(out))
	if s == "(null)" || s == "" {
		return nil
	}
	s = strings.TrimPrefix(s, "(")
	s = strings.TrimSuffix(s, ")")
	parts := strings.Split(s, ",")
	var tags []string
	for _, p := range parts {
		tag := strings.Trim(strings.TrimSpace(p), `"`)
		// Finder tags render as "name\n5", with the color number after a newline.
		if i := strings.IndexByte(tag, '\n'); i >= 0 {
			tag = tag[:i]
		}
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (a *appState) loadTags() {
	a.mu.Lock()
	paths := make([]string, len(a.Items))
	for i := range a.Items {
		paths[i] = a.Items[i].Path
	}
	a.mu.Unlock()
	tags := make([][]string, len(paths))
	for i, p := range paths {
		tags[i] = finderTags(p)
	}
	a.Window.Update(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		for i := range a.Items {
			if i < len(tags) {
				a.Items[i].Tags = tags[i]
			}
		}
		a.applyFilterLocked()
	})
}

func finderTags(path string) []string {
	if tags, err := urlTags(path); err == nil {
		return tags
	}
	return finderTagsMDLS(path)
}
