// File tags: platform IO goes through vfs; the toggle/rating logic is
// shared by every platform.
package main

import (
	"path/filepath"
	"strings"
)

// urlTags reads one path's platform tags.
func urlTags(path string) ([]string, error) { return mediaTags(path) }

// setURLTags canonicalizes label colors and writes the tags back.
func setURLTags(path string, tags []string) error {
	stored := make([]string, len(tags))
	for i, tag := range tags {
		stored[i] = canonicalTag(tag)
	}
	return setMediaTags(path, stored)
}

// canonicalTag suffixes a Finder label color index to a label name, the
// form the tags xattr stores ("红色\n6").
func canonicalTag(name string) string {
	names := finderLabelNames()
	indexes := []int{6, 7, 5, 2, 4, 3, 1}
	for i, label := range names {
		if label == name && i < len(indexes) {
			return name + "\n" + itoaSmall(indexes[i])
		}
	}
	return name
}

func itoaSmall(n int) string {
	if n < 0 || n > 9 {
		return "0"
	}
	return string(rune('0' + n))
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func toggleFinderTag(paths []string, name string) (bool, error) {
	current := make([][]string, len(paths))
	allHave := len(paths) > 0
	for i, path := range paths {
		tags, err := urlTags(path)
		if err != nil {
			return false, err
		}
		current[i] = tags
		if !containsString(tags, name) {
			allHave = false
		}
	}
	adding := !allHave
	for i, path := range paths {
		next := withoutTag(current[i], name)
		if adding {
			next = append(next, name)
		}
		if err := setURLTags(path, next); err != nil {
			return adding, err
		}
	}
	return adding, nil
}

func clearFinderTags(paths []string) error {
	for _, path := range paths {
		if err := setURLTags(path, nil); err != nil {
			return err
		}
	}
	return nil
}

func isRatingTag(tag string) bool {
	tag = strings.TrimSpace(tag)
	if !strings.HasPrefix(tag, "评分") {
		return false
	}
	rest := strings.TrimPrefix(tag, "评分")
	return len(rest) == 1 && rest[0] >= '1' && rest[0] <= '5'
}

func withoutTag(tags []string, name string) []string {
	var out []string
	for _, tag := range tags {
		if tag != name {
			out = append(out, tag)
		}
	}
	return out
}

func refreshItemTags(items []mediaItem, paths []string) {
	want := map[string]struct{}{}
	for _, path := range paths {
		want[path] = struct{}{}
	}
	for i := range items {
		if _, ok := want[items[i].Path]; !ok {
			continue
		}
		if tags, err := urlTags(items[i].Path); err == nil {
			items[i].Tags = tags
		}
	}
}

func allImages(paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	for _, path := range paths {
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
		if !isImageFile(ext) {
			return false
		}
	}
	return true
}

// finderTagsFallback answers when platformTags fails; Spotlight on macOS.
var finderTagsFallback = func(path string) []string { return nil }

// finderTags reads one path's tags.
func finderTags(path string) []string {
	if tags, err := urlTags(path); err == nil {
		return tags
	}
	return finderTagsFallback(path)
}

// loadTags fills every item's tags from the platform and refilters.
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
	a.update(func() {
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
