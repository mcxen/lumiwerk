package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// Shared (untagged) so the XDG trash logic is testable on any host.

func xdgDataHome() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share")
}

// xdgTrash moves path into dataHome/Trash/files under a free name.
func xdgTrash(path, dataHome string) (string, error) {
	if dataHome == "" {
		return "", errTrashUnsupported
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	files := filepath.Join(dataHome, "Trash", "files")
	infos := filepath.Join(dataHome, "Trash", "info")
	if err := os.MkdirAll(files, 0o700); err != nil {
		return "", err
	}
	if err := os.MkdirAll(infos, 0o700); err != nil {
		return "", err
	}
	base := filepath.Base(abs)
	name := base
	for i := 2; ; i++ {
		_, errF := os.Lstat(filepath.Join(files, name))
		_, errI := os.Lstat(filepath.Join(infos, name+".trashinfo"))
		if os.IsNotExist(errF) && os.IsNotExist(errI) {
			break
		}
		name = fmt.Sprintf("%s.%d", base, i)
	}
	record := "[Trash Info]\nPath=" + (&url.URL{Path: abs}).EscapedPath() +
		"\nDeletionDate=" + time.Now().Format("2006-01-02T15:04:05") + "\n"
	infoPath := filepath.Join(infos, name+".trashinfo")
	if err := os.WriteFile(infoPath, []byte(record), 0o600); err != nil {
		return "", err
	}
	dest := filepath.Join(files, name)
	if err := movePath(abs, dest); err != nil {
		_ = os.Remove(infoPath)
		return "", err
	}
	return dest, nil
}
