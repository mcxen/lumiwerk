//go:build !darwin

package main

import (
	"errors"
	"os/exec"
)

// shareService is a no-op outside macOS: the share menu reports no services,
// so the platform share sheet is the only sharing path.
type shareService struct {
	Title string
}

func sharingServices(paths []string) ([]shareService, error) {
	return nil, errors.New("此平台暂不支持共享菜单")
}

func releaseShareServices(list []shareService) {}

func (s shareService) perform(paths []string) {}

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

var _ = exec.Command
