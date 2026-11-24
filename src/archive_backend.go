package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mholt/archives"
	"github.com/nwaples/rardecode/v2"
	yzip "github.com/yeka/zip"
)

// archiveBackend abstracts listing and extracting archive formats across the
// pure-Go engine (mholt/archives + yeka/zip) and the external CLI fallback
// (bsdtar / 7z for formats like ISO, CAB, and LZH).
type archiveBackend interface {
	List(archive string, passwords []string) ([]archiveEntry, error)
	ExtractMember(archive, inner, dest string, passwords []string) error
	ExtractAll(archive, destDir string, passwords []string) error
}

var (
	errUnsupportedArchive = errors.New("unsupported archive format")
	errFoundMember        = errors.New("archive member found")
)

var archiveBackends = []archiveBackend{
	goArchiveBackend{},
	cliArchiveBackend{},
}

func listArchiveWithBackends(archive string, passwords []string) ([]archiveEntry, error) {
	var lastErr error
	for _, b := range archiveBackends {
		entries, err := b.List(archive, passwords)
		if err == nil {
			return entries, nil
		}
		if errors.Is(err, errUnsupportedArchive) {
			continue
		}
		if errors.Is(err, errNeedZipPassword) {
			return nil, err
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("无法读取此压缩包格式")
}

func extractMemberWithBackends(archive, inner, dest string, passwords []string) error {
	var lastErr error
	for _, b := range archiveBackends {
		err := b.ExtractMember(archive, inner, dest, passwords)
		if err == nil {
			return nil
		}
		if errors.Is(err, errUnsupportedArchive) {
			continue
		}
		if errors.Is(err, errNeedZipPassword) || errors.Is(err, errNoSuchEntry) {
			return err
		}
		lastErr = err
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("无法解压此压缩包成员")
}

func extractAllWithBackends(archive, destDir string, passwords []string) error {
	var lastErr error
	for _, b := range archiveBackends {
		err := b.ExtractAll(archive, destDir, passwords)
		if err == nil {
			return nil
		}
		if errors.Is(err, errUnsupportedArchive) {
			continue
		}
		if errors.Is(err, errNeedZipPassword) {
			return err
		}
		lastErr = err
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("无法解压此压缩包")
}

// passwordAttemptList builds the ordered, deduplicated list of passwords to try.
func passwordAttemptList(includeEmpty bool, typed string, book []string) []string {
	merged := mergePasswords(append([]string{typed}, book...))
	if includeEmpty {
		return append([]string{""}, merged...)
	}
	return merged
}

// tryPasswords runs fn for each candidate password until one succeeds or a
// non-password error occurs.
func tryPasswords(tries []string, fn func(pass string) error) error {
	if len(tries) == 0 {
		return errNeedZipPassword
	}
	last := errNeedZipPassword
	for _, pass := range tries {
		err := fn(pass)
		if err == nil {
			return nil
		}
		if isArchivePasswordErr(err) {
			last = errNeedZipPassword
			continue
		}
		return err
	}
	return last
}

func isArchivePasswordErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errNeedZipPassword) ||
		errors.Is(err, yzip.ErrPassword) ||
		errors.Is(err, yzip.ErrDecryption) ||
		errors.Is(err, yzip.ErrAuthentication) ||
		errors.Is(err, rardecode.ErrBadPassword) {
		return true
	}
	return archivePasswordError(err.Error())
}

func archivePasswordError(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "password") ||
		strings.Contains(lower, "passphrase") ||
		strings.Contains(lower, "decrypt") ||
		strings.Contains(lower, "encrypted") ||
		strings.Contains(lower, "incorrect password")
}

// safeArchivePath prevents path traversal ("..") outside destDir when extracting.
func safeArchivePath(destDir, entryName string) (string, error) {
	normalized := strings.ReplaceAll(entryName, "\\", "/")
	for _, seg := range strings.Split(normalized, "/") {
		if seg == ".." {
			return "", fmt.Errorf("unsafe archive path: %s", entryName)
		}
	}
	cleaned := cleanEntryName(normalized)
	if cleaned == "" || cleaned == "." {
		return "", errors.New("invalid entry path")
	}
	target := filepath.Join(destDir, filepath.FromSlash(cleaned))
	rel, err := filepath.Rel(filepath.Clean(destDir), filepath.Clean(target))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe archive path: %s", entryName)
	}
	return target, nil
}

// ---- pure-Go archive backend (yeka/zip + mholt/archives) ------------------

type goArchiveBackend struct{}

func (goArchiveBackend) List(archive string, passwords []string) ([]archiveEntry, error) {
	if isZipName(filepath.Base(archive)) {
		return listZipEntries(archive)
	}
	info, err := os.Stat(archive)
	if err != nil {
		return nil, err
	}
	var entries []archiveEntry
	tries := passwordAttemptList(true, "", passwords)
	err = tryPasswords(tries, func(pass string) error {
		entries = nil
		return runGoExtractor(archive, pass, func(_ context.Context, f archives.FileInfo) error {
			name := cleanEntryName(f.NameInArchive)
			if name == "" || junkEntry(name) {
				return nil
			}
			mod := f.ModTime()
			if mod.IsZero() {
				mod = info.ModTime()
			}
			entries = append(entries, archiveEntry{
				Name:    name,
				IsDir:   f.IsDir(),
				Size:    f.Size(),
				ModTime: mod,
			})
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func (goArchiveBackend) ExtractMember(archive, inner, dest string, passwords []string) error {
	if isZipName(filepath.Base(archive)) {
		return extractZipMember(archive, inner, dest, passwords)
	}
	tries := passwordAttemptList(true, "", passwords)
	return tryPasswords(tries, func(pass string) error {
		err := runGoExtractor(archive, pass, func(_ context.Context, f archives.FileInfo) error {
			if f.IsDir() || cleanEntryName(f.NameInArchive) != inner {
				return nil
			}
			rc, openErr := f.Open()
			if openErr != nil {
				return openErr
			}
			defer rc.Close()
			if writeErr := writeCached(dest, rc); writeErr != nil {
				return writeErr
			}
			return errFoundMember
		})
		if errors.Is(err, errFoundMember) {
			return nil
		}
		if err == nil {
			return errNoSuchEntry
		}
		return err
	})
}

func (goArchiveBackend) ExtractAll(archive, destDir string, passwords []string) error {
	if isZipName(filepath.Base(archive)) {
		return extractZipAll(archive, destDir, passwords)
	}
	tries := passwordAttemptList(true, "", passwords)
	return tryPasswords(tries, func(pass string) error {
		return runGoExtractor(archive, pass, func(_ context.Context, f archives.FileInfo) error {
			name := cleanEntryName(f.NameInArchive)
			if name == "" || junkEntry(name) {
				return nil
			}
			target, err := safeArchivePath(destDir, name)
			if err != nil {
				return nil
			}
			if f.IsDir() {
				return os.MkdirAll(target, 0o755)
			}
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			return writeCached(target, rc)
		})
	})
}

func runGoExtractor(archive, password string, handler archives.FileHandler) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()

	ctx := context.Background()
	format, stream, err := archives.Identify(ctx, filepath.Base(archive), f)
	if err != nil {
		if errors.Is(err, archives.NoMatch) {
			return errUnsupportedArchive
		}
		return err
	}
	switch v := format.(type) {
	case archives.SevenZip:
		v.Password = password
		err = v.Extract(ctx, stream, handler)
		if err != nil && password == "" && sevenZipNeedsPassword(err) {
			return errNeedZipPassword
		}
		return err
	case archives.Rar:
		v.Password = password
		return v.Extract(ctx, stream, handler)
	default:
		ext, ok := format.(archives.Extractor)
		if !ok {
			return errUnsupportedArchive
		}
		return ext.Extract(ctx, stream, handler)
	}
}

func sevenZipNeedsPassword(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "password") ||
		strings.Contains(msg, "encrypted") ||
		strings.Contains(msg, "checksum") ||
		strings.Contains(msg, "unexpected id")
}

func listZipEntries(archive string) ([]archiveEntry, error) {
	r, err := yzip.OpenReader(archive)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var out []archiveEntry
	for _, f := range r.File {
		name := cleanEntryName(f.Name)
		if name == "" || junkEntry(name) {
			continue
		}
		dir := f.FileInfo().IsDir() || strings.HasSuffix(f.Name, "/")
		out = append(out, archiveEntry{
			Name:      name,
			IsDir:     dir,
			Size:      int64(f.UncompressedSize64),
			ModTime:   f.ModTime(),
			Encrypted: f.IsEncrypted(),
		})
	}
	return out, nil
}

func extractZipMember(archive, inner, dest string, passwords []string) error {
	r, err := yzip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if cleanEntryName(f.Name) != inner || f.FileInfo().IsDir() {
			continue
		}
		return extractZipFile(f, dest, passwords)
	}
	return errNoSuchEntry
}

func extractZipAll(archive, destDir string, passwords []string) error {
	r, err := yzip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		name := cleanEntryName(f.Name)
		if name == "" || junkEntry(name) {
			continue
		}
		target, err := safeArchivePath(destDir, name)
		if err != nil {
			continue
		}
		if f.FileInfo().IsDir() || strings.HasSuffix(f.Name, "/") {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := extractZipFile(f, target, passwords); err != nil {
			return err
		}
	}
	return nil
}

func extractZipFile(f *yzip.File, dest string, passwords []string) error {
	tries := passwordAttemptList(!f.IsEncrypted(), "", passwords)
	return tryPasswords(tries, func(pass string) error {
		if f.IsEncrypted() {
			f.SetPassword(pass)
		}
		rc, err := f.Open()
		if err != nil {
			if f.IsEncrypted() {
				return errNeedZipPassword
			}
			return err
		}
		defer rc.Close()
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, rc); err != nil {
			if f.IsEncrypted() || isArchivePasswordErr(err) {
				return errNeedZipPassword
			}
			return err
		}
		return writeCached(dest, bytes.NewReader(buf.Bytes()))
	})
}

// writeZipArchive creates a plain or ZipCrypto-encrypted ZIP archive in pure Go.
func writeZipArchive(paths []string, dest, password string) error {
	cwd := commonParent(paths)
	if cwd == "" {
		return errors.New("没有要压缩的项目")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".zip-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	zw := yzip.NewWriter(tmp)
	seen := map[string]bool{}
	addEntry := func(fullPath string, info fs.FileInfo) error {
		rel, relErr := filepath.Rel(cwd, fullPath)
		if relErr != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return errors.New("压缩项目不在同一目录树内。")
		}
		name := filepath.ToSlash(rel)
		if info.IsDir() {
			if !strings.HasSuffix(name, "/") {
				name += "/"
			}
			if seen[name] {
				return nil
			}
			seen[name] = true
			hdr, err := yzip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			hdr.Name = name
			hdr.Method = yzip.Store
			_, err = zw.CreateHeader(hdr)
			return err
		}
		if seen[name] {
			return nil
		}
		seen[name] = true
		hdr, err := yzip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = name
		hdr.Method = yzip.Deflate
		if password != "" {
			hdr.SetPassword(password)
			hdr.SetEncryptionMethod(yzip.StandardEncryption)
		}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(fullPath)
			if err != nil {
				return err
			}
			_, err = io.WriteString(w, target)
			return err
		}
		f, err := os.Open(fullPath)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	}

	for _, rootPath := range paths {
		linfo, err := os.Lstat(rootPath)
		if err != nil {
			_ = zw.Close()
			_ = tmp.Close()
			return err
		}
		if !linfo.IsDir() || linfo.Mode()&os.ModeSymlink != 0 {
			if err := addEntry(rootPath, linfo); err != nil {
				_ = zw.Close()
				_ = tmp.Close()
				return err
			}
			continue
		}
		err = filepath.WalkDir(rootPath, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			return addEntry(p, info)
		})
		if err != nil {
			_ = zw.Close()
			_ = tmp.Close()
			return err
		}
	}

	if err := zw.Close(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// ---- external CLI fallback (bsdtar / 7z for ISO, CAB, LZH) ----------------

type cliArchiveBackend struct{}

var (
	archiveToolMu    sync.Mutex
	archiveToolPaths = map[string]string{}
)

func archiveToolBin(kind string) string {
	archiveToolMu.Lock()
	defer archiveToolMu.Unlock()
	if p, ok := archiveToolPaths[kind]; ok {
		return p
	}
	var p string
	switch kind {
	case "bsdtar":
		var extra []string
		if runtime.GOOS == "windows" {
			sysRoot := os.Getenv("SystemRoot")
			if sysRoot == "" {
				sysRoot = `C:\Windows`
			}
			extra = append(extra, filepath.Join(sysRoot, "System32"))
			p = findToolNamed([]string{"bsdtar", "tar"}, extra...)
		} else {
			p = findToolNamed([]string{"bsdtar"})
		}
	case "7z":
		var extra []string
		if runtime.GOOS == "windows" {
			for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432"} {
				if d := os.Getenv(env); d != "" {
					extra = append(extra, filepath.Join(d, "7-Zip"))
				}
			}
			extra = append(extra, `C:\Program Files\7-Zip`, `C:\Program Files (x86)\7-Zip`)
		}
		p = findToolNamed([]string{"7z", "7zz", "7za"}, extra...)
	}
	archiveToolPaths[kind] = p
	return p
}

func bsdtarBin() string   { return archiveToolBin("bsdtar") }
func sevenZipBin() string { return archiveToolBin("7z") }

func bsdtarArgs(pass string, args ...string) []string {
	if pass == "" {
		return args
	}
	return append([]string{"--passphrase", pass}, args...)
}

func (cliArchiveBackend) List(archive string, passwords []string) ([]archiveEntry, error) {
	info, err := os.Stat(archive)
	if err != nil {
		return nil, err
	}
	var bsdtarErr error
	if bin := bsdtarBin(); bin != "" {
		cmd := execCommand(bin, "-tf", archive)
		cmd.Stdin = bytes.NewReader(nil)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err == nil {
			var entries []archiveEntry
			for _, line := range strings.Split(string(out), "\n") {
				line = strings.TrimRight(line, "\r")
				if line == "" {
					continue
				}
				name := cleanEntryName(line)
				if name == "" || junkEntry(name) {
					continue
				}
				entries = append(entries, archiveEntry{
					Name:    name,
					IsDir:   strings.HasSuffix(line, "/"),
					ModTime: info.ModTime(),
				})
			}
			return entries, nil
		}
		if archivePasswordError(stderr.String()) {
			bsdtarErr = errNeedZipPassword
		} else {
			bsdtarErr = fmt.Errorf("读取压缩包失败：%s", strings.TrimSpace(stderr.String()))
		}
	}
	if sz := sevenZipBin(); sz != "" {
		return listWithSevenZip(sz, archive, info.ModTime(), passwords)
	}
	if bsdtarErr != nil {
		return nil, bsdtarErr
	}
	return nil, errors.New("未找到 bsdtar 或 7-Zip，无法读取此压缩包")
}

func (cliArchiveBackend) ExtractMember(archive, inner, dest string, passwords []string) error {
	bsdtar := bsdtarBin()
	sz := sevenZipBin()
	if bsdtar == "" && sz == "" {
		return errors.New("未找到 bsdtar 或 7-Zip，无法解压此压缩包")
	}
	tries := passwordAttemptList(true, "", passwords)
	return tryPasswords(tries, func(pass string) error {
		if bsdtar != "" {
			cmd := execCommand(bsdtar, bsdtarArgs(pass, "-xOf", archive, "--", inner)...)
			cmd.Stdin = bytes.NewReader(nil)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			if err == nil && len(out) > 0 {
				return writeCached(dest, bytes.NewReader(out))
			}
			if err == nil && sz == "" {
				return errNoSuchEntry
			}
		}
		if sz != "" {
			out, err := sevenZipExtractOneBytes(sz, archive, inner, pass)
			if err == nil && len(out) > 0 {
				return writeCached(dest, bytes.NewReader(out))
			}
			if err == nil {
				return errNoSuchEntry
			}
			return err
		}
		return errNeedZipPassword
	})
}

func (cliArchiveBackend) ExtractAll(archive, destDir string, passwords []string) error {
	bsdtar := bsdtarBin()
	sz := sevenZipBin()
	if bsdtar == "" && sz == "" {
		return errors.New("未找到 bsdtar 或 7-Zip，无法解压此压缩包")
	}
	tries := passwordAttemptList(true, "", passwords)
	return tryPasswords(tries, func(pass string) error {
		var err error
		if bsdtar != "" {
			args := bsdtarArgs(pass, "-xf", archive, "-C", destDir)
			err = runTool(bsdtar, args, "")
			if err == nil {
				return nil
			}
			if archivePasswordError(err.Error()) {
				err = errNeedZipPassword
			}
		}
		if sz != "" {
			return sevenZipExtractAll(sz, archive, destDir, pass)
		}
		return err
	})
}

func listWithSevenZip(bin, archive string, fallbackMod time.Time, passwords []string) ([]archiveEntry, error) {
	var entries []archiveEntry
	err := tryPasswords(passwordAttemptList(true, "", passwords), func(pass string) error {
		cmd := execCommand(bin, "l", "-slt", "-p"+pass, "--", archive)
		cmd.Stdin = bytes.NewReader(nil)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err == nil {
			entries = parseSevenZipSLT(string(out), fallbackMod)
			return nil
		}
		msg := strings.TrimSpace(stderr.String())
		if archivePasswordError(msg) {
			return errNeedZipPassword
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("读取压缩包失败：%s", msg)
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func sevenZipExtractOneBytes(bin, archive, inner, pass string) ([]byte, error) {
	cmd := execCommand(bin, "x", "-so", "-y", "-p"+pass, "--", archive, inner)
	cmd.Stdin = bytes.NewReader(nil)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if archivePasswordError(msg) {
			return nil, errNeedZipPassword
		}
		if msg == "" {
			msg = err.Error()
		}
		return nil, errors.New(msg)
	}
	return out, nil
}

func sevenZipExtractAll(bin, archive, dir, pass string) error {
	cmd := execCommand(bin, "x", "-y", "-o"+dir, "-p"+pass, "--", archive)
	cmd.Stdin = bytes.NewReader(nil)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if archivePasswordError(msg) {
		return errNeedZipPassword
	}
	if msg == "" {
		msg = err.Error()
	}
	return fmt.Errorf("解压失败：%s", msg)
}

// parseSevenZipSLT parses `7z l -slt` output into archiveEntry rows.
func parseSevenZipSLT(out string, fallbackMod time.Time) []archiveEntry {
	var entries []archiveEntry
	var (
		inItems   bool
		hasRecord bool
		cur       archiveEntry
		isFolder  bool
	)
	flush := func() {
		if !hasRecord {
			return
		}
		name := cleanEntryName(cur.Name)
		if name != "" && !junkEntry(name) {
			cur.Name = name
			cur.IsDir = isFolder
			if cur.ModTime.IsZero() {
				cur.ModTime = fallbackMod
			}
			entries = append(entries, cur)
		}
		hasRecord = false
		cur = archiveEntry{}
		isFolder = false
	}

	for _, rawLine := range strings.Split(out, "\n") {
		line := strings.TrimRight(rawLine, "\r")
		if line == "----------" {
			inItems = true
			flush()
			continue
		}
		if !inItems {
			continue
		}
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		k, v, ok := strings.Cut(line, " = ")
		if !ok {
			continue
		}
		switch k {
		case "Path":
			hasRecord = true
			cur.Name = v
		case "Size":
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
				cur.Size = n
			}
		case "Folder":
			if strings.TrimSpace(v) == "+" {
				isFolder = true
			}
		case "Attributes":
			if strings.HasPrefix(strings.TrimSpace(v), "D") {
				isFolder = true
			}
		case "Encrypted":
			if strings.TrimSpace(v) == "+" {
				cur.Encrypted = true
			}
		case "Modified":
			s := strings.TrimSpace(v)
			if len(s) >= 19 {
				if t, err := time.ParseInLocation("2006-01-02 15:04:05", s[:19], time.Local); err == nil {
					cur.ModTime = t
				}
			}
		}
	}
	flush()
	return entries
}
