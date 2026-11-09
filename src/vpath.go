package main

// Browsing inside archives.
//
// An archive is a directory as far as the browser is concerned: the path
// "/photos/trip.cbz/ch1/p01.jpg" names the entry ch1/p01.jpg of the real file
// /photos/trip.cbz. Nothing on disk can have children below a regular file, so
// the split is unambiguous, and filepath.Dir/Join/Base (breadcrumbs, going up,
// sorting) keep working on these paths unchanged.
//
// Listing reads the central directory (ZIP, in process) or asks bsdtar (every
// other format). File access goes through realPath, which extracts the entry
// into a cache once and returns the cached file; thumbnails, the large view,
// metadata and playback all call it at their single file-open point.

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// archiveEntry is one member of an archive, with a slash-separated name
// relative to the archive root and no trailing slash.
type archiveEntry struct {
	Name      string
	IsDir     bool
	Size      int64
	ModTime   time.Time
	Encrypted bool
}

// browsableArchiveSuffixes are the formats the browser can open as folders.
// Single-stream compressors (.gz, .xz...) hold one file and stay plain files.
var browsableArchiveSuffixes = []string{
	".zip", ".cbz", ".rar", ".cbr", ".7z", ".cb7", ".tar", ".tgz", ".tbz", ".tbz2", ".txz",
	".tar.gz", ".tar.bz2", ".tar.xz", ".tar.lz4", ".tar.zst", ".tar.lzma", ".lzh", ".lha", ".cab", ".iso",
}

// isBrowsableArchive reports whether name is an archive the browser can enter.
func isBrowsableArchive(name string) bool {
	lower := strings.ToLower(name)
	for _, s := range browsableArchiveSuffixes {
		if strings.HasSuffix(lower, s) {
			return true
		}
	}
	return false
}

// splitArchivePath splits p into the archive file and the slash-separated
// entry path inside it. ok is false for ordinary paths; existing files and
// directories take the fast path with one lstat.
func splitArchivePath(p string) (archive, inner string, ok bool) {
	if p == "" || p == "/" {
		return "", "", false
	}
	if _, err := os.Lstat(p); err == nil {
		return "", "", false
	}
	cur := filepath.Clean(p)
	var tail []string
	for {
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", "", false
		}
		tail = append([]string{filepath.Base(cur)}, tail...)
		cur = parent
		info, err := os.Stat(cur)
		if err != nil {
			continue
		}
		if info.Mode().IsRegular() && isBrowsableArchive(info.Name()) {
			return cur, path.Join(tail...), true
		}
		return "", "", false
	}
}

// archiveLocation is splitArchivePath that also accepts the archive file
// itself (inner ""), which is how an opened archive is addressed.
func archiveLocation(p string) (archive, inner string, ok bool) {
	if a, in, ok := splitArchivePath(p); ok {
		return a, in, true
	}
	if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() && isBrowsableArchive(info.Name()) {
		return p, "", true
	}
	return "", "", false
}

// isVirtualPath is true for paths below an archive (not the archive itself).
func isVirtualPath(p string) bool {
	_, _, ok := splitArchivePath(p)
	return ok
}

// memInfo is the os.FileInfo of a synthesized entry.
type memInfo struct {
	name string
	size int64
	mod  time.Time
	dir  bool
}

func (m memInfo) Name() string { return m.name }
func (m memInfo) Size() int64  { return m.size }
func (m memInfo) Mode() fs.FileMode {
	if m.dir {
		return fs.ModeDir | 0o555
	}
	return 0o444
}
func (m memInfo) ModTime() time.Time { return m.mod }
func (m memInfo) IsDir() bool        { return m.dir }
func (m memInfo) Sys() any           { return nil }

// ---- listing --------------------------------------------------------------

type listKey struct {
	archive string
	mtime   int64
	size    int64
}

var (
	listMu    sync.Mutex
	listCache = map[listKey][]archiveEntry{}
)

func archiveKeyFor(archive string) (listKey, error) {
	info, err := os.Stat(archive)
	if err != nil {
		return listKey{}, err
	}
	return listKey{archive: archive, mtime: info.ModTime().UnixNano(), size: info.Size()}, nil
}

// listArchive returns every member of archive, cached until the file changes.
func listArchive(archive string) ([]archiveEntry, error) {
	key, err := archiveKeyFor(archive)
	if err != nil {
		return nil, err
	}
	listMu.Lock()
	if entries, ok := listCache[key]; ok {
		listMu.Unlock()
		return entries, nil
	}
	listMu.Unlock()

	var entries []archiveEntry
	if isZipName(filepath.Base(archive)) {
		entries, err = listZip(archive)
	} else {
		entries, err = listWithBsdtar(archive)
	}
	if err != nil {
		return nil, err
	}
	listMu.Lock()
	if len(listCache) > 32 {
		listCache = map[listKey][]archiveEntry{}
	}
	listCache[key] = entries
	listMu.Unlock()
	return entries, nil
}

// junkEntry hides what archivers add behind the user's back.
func junkEntry(name string) bool {
	for _, part := range strings.Split(name, "/") {
		if part == "__MACOSX" || part == ".DS_Store" || part == "Thumbs.db" {
			return true
		}
	}
	return false
}

func cleanEntryName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	return strings.TrimPrefix(path.Clean("/"+name), "/")
}

func listZip(archive string) ([]archiveEntry, error) {
	r, err := zip.OpenReader(archive)
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
			Name: name, IsDir: dir, Size: int64(f.UncompressedSize64),
			ModTime: f.Modified, Encrypted: f.Flags&1 != 0,
		})
	}
	return out, nil
}

func listWithBsdtar(archive string) ([]archiveEntry, error) {
	info, err := os.Stat(archive)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("/usr/bin/bsdtar", "-tf", archive)
	cmd.Stdin = bytes.NewReader(nil)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if archivePasswordError(stderr.String()) {
			return nil, errNeedZipPassword
		}
		return nil, fmt.Errorf("读取压缩包失败：%s", strings.TrimSpace(stderr.String()))
	}
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
			Name: name, IsDir: strings.HasSuffix(line, "/"), ModTime: info.ModTime(),
		})
	}
	return entries, nil
}

// archiveChildren lists the direct children of the folder inner ("" = root)
// of archive, synthesizing folders that exist only as a prefix of a member.
func archiveChildren(archive, inner string) ([]scanEntry, error) {
	entries, err := listArchive(archive)
	if err != nil {
		return nil, err
	}
	archInfo, _ := os.Stat(archive)
	base := archive
	prefix := ""
	if inner != "" {
		base = filepath.Join(archive, filepath.FromSlash(inner))
		prefix = inner + "/"
	}
	seen := map[string]bool{}
	var out []scanEntry
	add := func(name string, dir bool, size int64, mod time.Time) {
		if seen[name] {
			return
		}
		seen[name] = true
		if mod.IsZero() && archInfo != nil {
			mod = archInfo.ModTime()
		}
		out = append(out, scanEntry{
			Path: filepath.Join(base, name), Name: name, IsDir: dir,
			Size: size, ModTime: mod, Info: memInfo{name: name, size: size, mod: mod, dir: dir},
			Hidden: strings.HasPrefix(name, "."),
		})
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name, prefix) {
			continue
		}
		rest := e.Name[len(prefix):]
		if rest == "" {
			continue
		}
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			add(rest[:i], true, 0, time.Time{})
			continue
		}
		add(rest, e.IsDir, e.Size, e.ModTime)
	}
	return out, nil
}

// archiveHasDir reports whether inner names a folder of archive.
func archiveHasDir(archive, inner string) bool {
	if inner == "" {
		return true
	}
	entries, err := listArchive(archive)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Name == inner && e.IsDir || strings.HasPrefix(e.Name, inner+"/") {
			return true
		}
	}
	return false
}

// ---- extraction -----------------------------------------------------------

// archivePasswords supplies the passwords to try on encrypted members. The
// app keeps it in step with the password book; tests leave it empty.
var (
	passwordMu   sync.Mutex
	passwordBook []string
)

func setArchivePasswords(list []string) {
	passwordMu.Lock()
	passwordBook = append([]string(nil), list...)
	passwordMu.Unlock()
}

func archivePasswords() []string {
	passwordMu.Lock()
	defer passwordMu.Unlock()
	return append([]string(nil), passwordBook...)
}

// archiveCacheRoot is where extracted members live. Tests replace it.
var archiveCacheRoot = func() string {
	dir, err := os.UserCacheDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "Lumiwerk", "archives")
}

// extractAllLimit is the largest non-ZIP archive extracted whole on first
// touch. bsdtar cannot seek to a member, so pulling one image out of a solid
// RAR rescans it; beyond the limit members are extracted one at a time.
const extractAllLimit = 2 << 30

var (
	extractMu    sync.Mutex
	extractLocks = map[string]*sync.Mutex{}
	cacheSwept   sync.Once
)

func extractLock(key string) *sync.Mutex {
	extractMu.Lock()
	defer extractMu.Unlock()
	l := extractLocks[key]
	if l == nil {
		l = &sync.Mutex{}
		extractLocks[key] = l
	}
	return l
}

func archiveCacheDir(key listKey) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d", key.archive, key.mtime, key.size)))
	return filepath.Join(archiveCacheRoot(), hex.EncodeToString(sum[:8]))
}

// sweepArchiveCache drops cached archives not touched for a week, once per run.
func sweepArchiveCache() {
	cacheSwept.Do(func() {
		root := archiveCacheRoot()
		dirs, err := os.ReadDir(root)
		if err != nil {
			return
		}
		cutoff := time.Now().Add(-7 * 24 * time.Hour)
		for _, d := range dirs {
			if info, err := d.Info(); err == nil && info.ModTime().Before(cutoff) {
				_ = os.RemoveAll(filepath.Join(root, d.Name()))
			}
		}
	})
}

var errNoSuchEntry = errors.New("压缩包里没有这个文件")

// realPath returns a path whose content can be opened with ordinary file
// calls: p itself when it is a normal path, otherwise the entry extracted to
// the cache. The cached copy is reused until the archive changes.
func realPath(p string) (string, error) {
	archive, inner, ok := splitArchivePath(p)
	if !ok {
		return p, nil
	}
	key, err := archiveKeyFor(archive)
	if err != nil {
		return "", err
	}
	sweepArchiveCache()
	dest := filepath.Join(archiveCacheDir(key), filepath.FromSlash(inner))
	if cachedFile(dest) {
		return dest, nil
	}
	lock := extractLock(archive)
	lock.Lock()
	defer lock.Unlock()
	if cachedFile(dest) {
		return dest, nil
	}
	if isZipName(filepath.Base(archive)) {
		err = extractZipEntry(archive, inner, dest)
	} else {
		err = extractNonZip(archive, inner, key, dest)
	}
	if err != nil {
		return "", err
	}
	return dest, nil
}

func cachedFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}

// realPathOr is realPath for callers that tolerate failure by falling back to
// the path they were given; the open then fails the ordinary way.
func realPathOr(p string) string {
	if real, err := realPath(p); err == nil {
		return real
	}
	return p
}

func extractZipEntry(archive, inner, dest string) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if cleanEntryName(f.Name) != inner || f.FileInfo().IsDir() {
			continue
		}
		if f.Flags&1 != 0 {
			return extractWithTool(archive, inner, dest)
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		return writeCached(dest, rc)
	}
	return errNoSuchEntry
}

// writeCached stores src at dest atomically so a half-written file is never
// mistaken for a finished one by a concurrent reader.
func writeCached(dest string, src io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".part-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, dest)
}

// bsdtarArgs prepends the passphrase option when pass is set.
func bsdtarArgs(pass string, args ...string) []string {
	if pass == "" {
		return args
	}
	return append([]string{"--passphrase", pass}, args...)
}

// extractWithTool pulls one member through bsdtar, trying the password book.
// It handles encrypted ZIP members, which archive/zip cannot decrypt.
func extractWithTool(archive, inner, dest string) error {
	tries := append([]string{""}, archivePasswords()...)
	last := errNeedZipPassword
	for _, pass := range tries {
		cmd := exec.Command("/usr/bin/bsdtar", bsdtarArgs(pass, "-xOf", archive, "--", inner)...)
		cmd.Stdin = bytes.NewReader(nil)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err == nil && len(out) > 0 {
			return writeCached(dest, bytes.NewReader(out))
		}
		last = errNeedZipPassword
		if err == nil {
			last = errNoSuchEntry
		}
	}
	return last
}

// extractNonZip extracts the whole archive once when it is small enough,
// otherwise just the one member.
func extractNonZip(archive, inner string, key listKey, dest string) error {
	if key.size > extractAllLimit {
		return extractWithTool(archive, inner, dest)
	}
	dir := archiveCacheDir(key)
	done := filepath.Join(dir, ".complete")
	if _, err := os.Stat(done); err != nil {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := bsdtarExtractAll(archive, dir); err != nil {
			return err
		}
		_ = os.WriteFile(done, nil, 0o644)
	}
	if cachedFile(dest) {
		return nil
	}
	return errNoSuchEntry
}

func bsdtarExtractAll(archive, dir string) error {
	tries := append([]string{""}, archivePasswords()...)
	var last error
	for _, pass := range tries {
		cmd := exec.Command("/usr/bin/bsdtar", bsdtarArgs(pass, "-xf", archive, "-C", dir)...)
		cmd.Stdin = bytes.NewReader(nil)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err == nil {
			return nil
		}
		last = fmt.Errorf("解压失败：%s", strings.TrimSpace(stderr.String()))
		if archivePasswordError(stderr.String()) {
			last = errNeedZipPassword
		}
	}
	return last
}

// isArchiveDir is true for an archive file or a folder inside one: places
// openFolder can enter although os.Stat says they are not directories.
func isArchiveDir(p string) bool {
	archive, inner, ok := archiveLocation(p)
	return ok && archiveHasDir(archive, inner)
}

// rejectArchiveMembers refuses operations on the inside of an archive. The
// members are a read-only view; copy them out by extracting the archive.
func rejectArchiveMembers(op fileOperation) error {
	for _, p := range append(append([]string(nil), op.paths...), op.dest) {
		if p != "" && isVirtualPath(p) {
			return errors.New("压缩包里的项目是只读的，请先解压。")
		}
	}
	return nil
}
