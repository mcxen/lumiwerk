//go:build !darwin && !windows

package main

import "errors"

// openerApps: without LaunchServices there is no per-file app list, so the
// "open with" menu only offers the default handler through openWithDefault.
func openerApps(path string) []openerApp { return nil }

func openWithApp(app string, paths []string) error {
	return errors.New("此平台暂不支持指定应用打开")
}

type openerApp struct {
	Name string
	Path string
}
