package main

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

// collectionFolderName matches FlowVision's CollectedFiles_yyyyMMdd_HHmmss.
func collectionFolderName(now time.Time) string {
	return "CollectedFiles_" + now.Format("20060102_150405")
}

type collectResult struct {
	folder string
	copied int
	failed int
}

// collectSubfolderFiles copies every regular file under the selected folders
// into one new sibling folder, flattening names the way FlowVision does.
func collectSubfolderFiles(folders []string, parent string, now time.Time) (collectResult, error) {
	if len(folders) < 2 {
		return collectResult{}, errors.New("请选择多个文件夹。")
	}
	for _, folder := range folders {
		info, err := os.Stat(folder)
		if err != nil || !info.IsDir() {
			return collectResult{}, errors.New("请仅选择文件夹。")
		}
	}
	target := flowUnique(filepath.Join(parent, collectionFolderName(now)), false, nil)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return collectResult{}, errors.New("创建归集文件夹失败。")
	}
	var reserved []string
	copied, failed := 0, 0
	for _, folder := range folders {
		walkErr := filepath.WalkDir(folder, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				failed++
				return nil
			}
			if d.IsDir() || !d.Type().IsRegular() {
				return nil
			}
			dest := flowUnique(filepath.Join(target, d.Name()), false, reserved)
			if copyErr := copyFile(path, dest); copyErr != nil {
				failed++
				return nil
			}
			reserved = append(reserved, dest)
			copied++
			return nil
		})
		if walkErr != nil {
			failed++
		}
	}
	if copied == 0 {
		_ = os.RemoveAll(target)
		if failed > 0 {
			return collectResult{}, errors.New("未能从子文件夹中归集到文件。")
		}
		return collectResult{}, errors.New("子文件夹中未找到可归集文件。")
	}
	return collectResult{folder: target, copied: copied, failed: failed}, nil
}

func allDirectories(paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			return false
		}
	}
	return true
}
