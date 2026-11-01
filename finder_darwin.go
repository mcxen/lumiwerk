package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

var (
	labelOnce  sync.Once
	labelNames []string
)

// finderLabelNames is the system Finder label order: red, orange, yellow,
// green, blue, purple, gray. Names follow the signed-in language.
func finderLabelNames() []string {
	labelOnce.Do(func() {
		labelNames = systemFinderLabels()
		if len(labelNames) == 0 {
			labelNames = []string{"红色", "橙色", "黄色", "绿色", "蓝色", "紫色", "灰色"}
		}
	})
	return labelNames
}

func systemFinderLabels() []string {
	ensureAppKit()
	if classWorkspace == 0 {
		return nil
	}
	var out []string
	withAutoreleasePool(func() {
		labels := objcMsg(objcMsg(classWorkspace, selSharedSpace), selFileLabels)
		n := int(objcMsg(labels, selCount))
		if n == 0 {
			return
		}
		order := []int{6, 7, 5, 2, 4, 3, 1}
		if n < 8 {
			order = nil
			for i := 1; i < n; i++ {
				order = append(order, i)
			}
		}
		for _, index := range order {
			if index < 0 || index >= n {
				continue
			}
			name := goNSString(objcMsg(labels, selObject, uintptr(index)))
			if name != "" {
				out = append(out, name)
			}
		}
	})
	return out
}

func urlTags(path string) ([]string, error) { return readTagXattr(path) }

func setURLTags(path string, tags []string) error {
	stored := make([]string, len(tags))
	for i, tag := range tags {
		stored[i] = canonicalTag(tag)
	}
	return writeTagXattr(path, stored)
}

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

type openerApp struct {
	Name string
	Path string
}

func openerApps(path string) []openerApp {
	ensureAppKit()
	if classWorkspace == 0 {
		return nil
	}
	var apps []openerApp
	withAutoreleasePool(func() {
		url := objcMsg(classURL, selFileURL, nsString(path))
		list := objcMsg(objcMsg(classWorkspace, selSharedSpace), selAppsForURL, url)
		n := int(objcMsg(list, selCount))
		if n > 12 {
			n = 12
		}
		seen := map[string]bool{}
		for i := 0; i < n; i++ {
			item := objcMsg(list, selObject, uintptr(i))
			appPath := goNSString(objcMsg(item, selURLPath))
			name := strings.TrimSuffix(goNSString(objcMsg(item, selLastComp)), ".app")
			if appPath == "" || name == "" || seen[appPath] {
				continue
			}
			seen[appPath] = true
			apps = append(apps, openerApp{Name: name, Path: appPath})
		}
	})
	return apps
}

func openWithApp(app string, paths []string) error {
	args := append([]string{"-a", app}, paths...)
	out, err := exec.Command("open", args...).CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			return err
		}
		return err
	}
	return nil
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

func setRatingTags(tags []string, rating int) []string {
	var next []string
	for _, tag := range tags {
		if isRatingTag(tag) {
			continue
		}
		next = append(next, tag)
	}
	if name := ratingTagName(rating); name != "" {
		next = append(next, name)
	}
	return next
}

func ratingTagName(rating int) string {
	if rating < 1 || rating > 5 {
		return ""
	}
	return "评分" + string(rune('0'+rating))
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

func applyRating(paths []string, rating int) error {
	for _, path := range paths {
		tags, err := urlTags(path)
		if err != nil {
			return err
		}
		if err := setURLTags(path, setRatingTags(tags, rating)); err != nil {
			return err
		}
	}
	return nil
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
