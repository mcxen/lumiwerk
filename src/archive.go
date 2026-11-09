package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// errNeedZipPassword asks the UI to collect a password and try again.
var errNeedZipPassword = errors.New("需要密码。")

// discardPath removes a source after a successful archive operation.
// FlowVision moves it to the Trash. Tests replace this.
var discardPath = func(path string) error {
	_, err := trashPaths([]string{path})
	return err
}

func isArchiveName(name string) bool {
	lower := strings.ToLower(name)
	for _, suffix := range []string{".tar.gz", ".tar.bz2", ".tar.xz", ".tar.lz4", ".tar.zst", ".tar.lzma"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".zip", ".cbz", ".rar", ".cbr", ".7z", ".tar", ".tgz", ".tbz", ".tbz2", ".txz",
		".gz", ".bz2", ".xz", ".lz4", ".zst", ".lzma", ".cab", ".iso":
		return true
	default:
		return false
	}
}

func isZipName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".zip", ".cbz":
		return true
	default:
		return false
	}
}

func allArchives(paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	for _, path := range paths {
		if !isArchiveName(filepath.Base(path)) {
			return false
		}
	}
	return true
}

func archivesOnly(paths []string) []string {
	var out []string
	for _, path := range paths {
		if isArchiveName(filepath.Base(path)) {
			out = append(out, path)
		}
	}
	return out
}

func sortedPaths(paths []string) []string {
	out := append([]string(nil), paths...)
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

// archiveBaseName strips one extension, or a compound tar suffix, the way
// FlowVision names the folder an archive extracts into.
func archiveBaseName(name string) string {
	lower := strings.ToLower(name)
	for _, ext := range []string{".tar.gz", ".tar.bz2", ".tar.xz"} {
		if strings.HasSuffix(lower, ext) {
			return name[:len(name)-len(ext)]
		}
	}
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func configuredZipPassword(p prefsFile) (string, bool) {
	pass := strings.TrimSpace(p.ZipPassword)
	if pass == "" {
		return "", false
	}
	return pass, true
}

// commonParent is the deepest folder that holds every path: the folder
// itself when they share one, an ancestor when a recursive listing mixed
// files from several subfolders.
func commonParent(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	parent := filepath.Dir(paths[0])
	for _, path := range paths[1:] {
		for parent != filepath.Dir(parent) && !pathWithin(parent, path) {
			parent = filepath.Dir(parent)
		}
	}
	return parent
}

// pathWithin reports that path lies inside dir.
func pathWithin(dir, path string) bool {
	rel, err := filepath.Rel(pathKey(dir), pathKey(path))
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// zipDestination matches FlowVision: one item becomes basename.zip, several
// become Archive_yyyyMMdd_HHmmss.zip, and an existing name becomes name_2.
// It lands in the items' folder, or in the folder above them all when they
// come from several.
func zipDestination(paths []string, now time.Time) (string, error) {
	if len(paths) == 0 {
		return "", errors.New("没有要压缩的项目")
	}
	parent := commonParent(paths)
	var name string
	if len(paths) == 1 {
		base := strings.TrimSuffix(filepath.Base(paths[0]), filepath.Ext(paths[0]))
		name = base + ".zip"
	} else {
		name = "Archive_" + now.Format("20060102_150405") + ".zip"
	}
	return flowUnique(filepath.Join(parent, name), false, nil), nil
}

func extractDestination(archive string) string {
	base := archiveBaseName(filepath.Base(archive))
	return flowUnique(filepath.Join(filepath.Dir(archive), base), false, nil)
}

// zipArgs is FlowVision's /usr/bin/zip -r -y, with -P only for an encrypted zip.
// Items are stored by their path below the common folder, so files from
// several subfolders keep their folders in the archive.
func zipArgs(paths []string, dest, password string) (string, []string, error) {
	if len(paths) == 0 {
		return "", nil, errors.New("没有要压缩的项目")
	}
	cwd := commonParent(paths)
	args := []string{"-r", "-y"}
	if password != "" {
		args = append(args, "-P", password)
	}
	args = append(args, dest)
	for _, path := range paths {
		rel, err := filepath.Rel(cwd, path)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return "", nil, errors.New("压缩项目不在同一目录树内。")
		}
		args = append(args, rel)
	}
	return cwd, args, nil
}

func compressPaths(paths []string, password string, now time.Time) (string, error) {
	if len(paths) == 0 {
		return "", errors.New("没有要压缩的项目")
	}
	paths = sortedPaths(paths)
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("压缩源文件不存在：%s", filepath.Base(path))
		}
	}
	dest, err := zipDestination(paths, now)
	if err != nil {
		return "", err
	}
	cwd, args, err := zipArgs(paths, dest, password)
	if err != nil {
		return "", err
	}
	if err := runTool("/usr/bin/zip", args, cwd); err != nil {
		_ = os.Remove(dest)
		return "", fmt.Errorf("压缩失败。%s", err.Error())
	}
	return dest, nil
}

func zipEncrypted(path string) bool {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.Flags&1 != 0 {
			return true
		}
	}
	return false
}

func archivePasswordError(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "password") || strings.Contains(lower, "passphrase") || strings.Contains(lower, "decrypt")
}

type extractedArchive struct {
	archive string
	dest    string
}

func extractArchives(paths []string, password string, candidates ...string) ([]extractedArchive, []string, error) {
	archives := archivesOnly(sortedPaths(paths))
	if len(archives) == 0 {
		return nil, nil, errors.New("请先选择压缩包文件。")
	}
	var extracted []extractedArchive
	var failed []string
	passwordFails := 0
	for _, archive := range archives {
		dest, err := extractWithPasswords(archive, password, candidates)
		if err != nil {
			failed = append(failed, filepath.Base(archive))
			if errors.Is(err, errNeedZipPassword) {
				passwordFails++
			}
			continue
		}
		extracted = append(extracted, extractedArchive{archive: archive, dest: dest})
	}
	if len(extracted) == 0 {
		if passwordFails == len(archives) {
			return nil, failed, errNeedZipPassword
		}
		return nil, failed, errors.New("解压失败。")
	}
	return extracted, failed, nil
}

// zipPasswordCandidates is the ordered, de-duplicated list of saved passwords
// to try on an encrypted archive: the default password first, then the book.
func zipPasswordCandidates(p prefsFile) []string {
	return mergePasswords(append([]string{p.ZipPassword}, p.ZipPasswords...))
}

func mergePasswords(list []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, pass := range list {
		pass = strings.TrimSpace(pass)
		if pass == "" || seen[pass] {
			continue
		}
		seen[pass] = true
		out = append(out, pass)
	}
	return out
}

// extractWithPasswords opens an archive without a password first, then with
// the one the user typed, then with every saved candidate. Only when none of
// them fit does it report errNeedZipPassword so the UI can ask.
func extractWithPasswords(archive, password string, candidates []string) (string, error) {
	tries := mergePasswords(append([]string{password}, candidates...))
	if strings.TrimSpace(password) == "" {
		tries = append([]string{""}, tries...)
	} else {
		tries[0] = password
	}
	var last error = errNeedZipPassword
	for _, pass := range tries {
		dest, err := extractOne(archive, pass)
		if err == nil {
			return dest, nil
		}
		if !errors.Is(err, errNeedZipPassword) {
			return "", err
		}
		last = err
	}
	return "", last
}

func extractOne(archive, password string) (string, error) {
	if isZipName(filepath.Base(archive)) && password == "" && zipEncrypted(archive) {
		return "", errNeedZipPassword
	}
	dest := extractDestination(archive)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", err
	}
	args := []string{"-xf", archive, "-C", dest}
	if password != "" {
		args = []string{"--passphrase", password, "-xf", archive, "-C", dest}
	}
	if err := runTool("/usr/bin/bsdtar", args, ""); err != nil {
		_ = os.RemoveAll(dest)
		// A wrong password does not always say so: depending on the bsdtar
		// build it surfaces as a bare read or CRC error. An encrypted zip
		// that fails while we hold a password means that password is wrong.
		if archivePasswordError(err.Error()) ||
			(password != "" && isZipName(filepath.Base(archive)) && zipEncrypted(archive)) {
			return "", errNeedZipPassword
		}
		return "", err
	}
	return dest, nil
}

func previewNames(names []string) string {
	if len(names) > 3 {
		names = names[:3]
	}
	return strings.Join(names, "、")
}

func runTool(name string, args []string, dir string) error {
	if _, err := os.Stat(name); err != nil {
		return fmt.Errorf("找不到 %s", filepath.Base(name))
	}
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(nil)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		msg = err.Error()
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return errors.New(msg)
}
