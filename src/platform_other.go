//go:build !darwin

package main

import (
	"errors"
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
