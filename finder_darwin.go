package main

import (
	"os/exec"
	"strings"
	"sync"
)

var (
	labelOnce  sync.Once
	labelNames []string
)

// finderLabelNames is the system Finder label order: red, orange, yellow,
// green, blue, purple, gray. Names follow the signed-in language. The vfs
// layer keeps the default order on other platforms.
func finderLabelNamesDarwin() []string {
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
	_, err := exec.Command("open", args...).CombinedOutput()
	return err
}
