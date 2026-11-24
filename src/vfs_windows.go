//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

// fileCreated reports a file's creation time, which NTFS keeps for real.
func fileCreated(info os.FileInfo) (time.Time, bool) {
	st, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return time.Time{}, false
	}
	// Filetime is 100-ns intervals since 1601-01-01.
	nsec := int64(st.CreationTime.HighDateTime)<<32 | int64(st.CreationTime.LowDateTime)
	if nsec == 0 {
		return time.Time{}, false
	}
	const epochDiff = 116444736000000000 // 1601 → 1970 in 100-ns units
	return time.Unix(0, (nsec-epochDiff)*100), true
}

// fileHidden reads the Windows hidden attribute.
func fileHidden(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return false
	}
	return st.FileAttributes&0x2 != 0 // FILE_ATTRIBUTE_HIDDEN
}

const (
	supportsTrashUntrash = true

	winFoDelete     = 3
	winFofSilent    = 0x0004
	winFofNoConfirm = 0x0010
	winFofAllowUndo = 0x0040
	winFofNoErrorUI = 0x0400
	winTrashFlags   = winFofSilent | winFofNoConfirm | winFofAllowUndo | winFofNoErrorUI
)

// shFileOpStruct matches Windows SDK SHFILEOPSTRUCTW on 64-bit Windows
// (amd64 / arm64, 56 bytes with natural alignment).
type shFileOpStruct struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

var (
	modShell32          = syscall.NewLazyDLL("shell32.dll")
	procSHFileOperation = modShell32.NewProc("SHFileOperationW")
)

// shFileOpFrom encodes paths as a double-NUL-terminated UTF-16 buffer for SHFileOperationW.
func shFileOpFrom(paths []string) ([]uint16, []string, error) {
	if len(paths) == 0 {
		return nil, nil, errors.New("没有要移到回收站的项目")
	}
	var buf []uint16
	absList := make([]string, 0, len(paths))
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, nil, err
		}
		abs = filepath.Clean(abs)
		if _, err := os.Stat(abs); err != nil {
			return nil, nil, err
		}
		absList = append(absList, abs)
		buf = append(buf, utf16.Encode([]rune(abs))...)
		buf = append(buf, 0)
	}
	buf = append(buf, 0)
	return buf, absList, nil
}

// platformTrashBatch moves one or more paths to the Windows Recycle Bin in a
// single synchronous SHFileOperationW call (no PowerShell spawn, no UI prompt).
func platformTrashBatch(paths []string) ([]pathPair, error) {
	from, absList, err := shFileOpFrom(paths)
	if err != nil {
		return nil, err
	}
	op := shFileOpStruct{
		wFunc:  winFoDelete,
		pFrom:  &from[0],
		fFlags: winTrashFlags,
	}
	r1, _, callErr := procSHFileOperation.Call(uintptr(unsafe.Pointer(&op)))
	if r1 != 0 {
		if callErr != nil && callErr != syscall.Errno(0) {
			return nil, fmt.Errorf("移到回收站失败（错误码 0x%x）：%v", r1, callErr)
		}
		return nil, fmt.Errorf("移到回收站失败（错误码 0x%x）", r1)
	}
	if op.fAnyOperationsAborted != 0 {
		return nil, errors.New("移到回收站操作被中止")
	}
	moved := make([]pathPair, len(absList))
	for i, abs := range absList {
		moved[i] = pathPair{From: abs}
	}
	return moved, nil
}

func platformTrash(path string) (string, error) {
	if _, err := platformTrashBatch([]string{path}); err != nil {
		return "", err
	}
	return "", nil
}

func windowsUntrashScript(abs string) string {
	escaped := strings.ReplaceAll(abs, "'", "''")
	return fmt.Sprintf(`$target = '%s'
$parent = [System.IO.Path]::GetDirectoryName($target)
$leaf = [System.IO.Path]::GetFileName($target)
$noext = [System.IO.Path]::GetFileNameWithoutExtension($target)
$bin = (New-Object -ComObject Shell.Application).NameSpace(10)
$hit = $null
foreach ($it in $bin.Items()) {
  $from = $it.ExtendedProperty('System.Recycle.DeletedFrom')
  if (-not $from -or ($from.TrimEnd('\') -ine $parent.TrimEnd('\'))) { continue }
  $p = $it.Path
  $dir = [System.IO.Path]::GetDirectoryName($p)
  $fn = [System.IO.Path]::GetFileName($p)
  $ok = $false
  if ($fn.StartsWith('$R') -and $dir) {
    $meta = Join-Path $dir ('$I' + $fn.Substring(2))
    if (Test-Path -LiteralPath $meta) {
      $b = [System.IO.File]::ReadAllBytes($meta)
      if ($b.Length -ge 28 -and [BitConverter]::ToInt64($b, 0) -eq 2) {
        $len = [BitConverter]::ToInt32($b, 24)
        if ($len -gt 0 -and (28 + $len * 2) -le $b.Length) {
          $orig = [System.Text.Encoding]::Unicode.GetString($b, 28, $len * 2).TrimEnd([char]0)
          if ($orig -ieq $target) { $ok = $true }
        }
      } elseif ($b.Length -ge 544 -and [BitConverter]::ToInt64($b, 0) -eq 1) {
        $orig = [System.Text.Encoding]::Unicode.GetString($b, 24, 520).TrimEnd([char]0)
        if ($orig -ieq $target) { $ok = $true }
      }
    }
  }
  if (-not $ok -and ($it.Name -ieq $leaf -or $it.Name -ieq $noext)) { $ok = $true }
  if ($ok) { $hit = $it }
}
if ($null -eq $hit) { exit 2 }
$hit.InvokeVerb('undelete')`, escaped)
}

// platformUntrash restores one item previously moved to the Windows Recycle Bin.
func platformUntrash(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	abs = filepath.Clean(abs)
	if _, err := os.Stat(abs); err == nil {
		return fmt.Errorf("目标位置已存在同名项目：%s", filepath.Base(abs))
	}
	script := windowsUntrashScript(abs)
	out, err := execCommand("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("从回收站还原失败：%s", msg)
	}
	for range 10 {
		if _, statErr := os.Stat(abs); statErr == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("从回收站还原未完成：%s", filepath.Base(abs))
}

// mediaThumbJPEG builds a JPEG thumbnail of edge on the long side. Windows
// has no system image tool, so everything decodes through Go; videos go
// through ffmpeg when installed.
func mediaThumbJPEG(path string, video bool, edge int) ([]byte, error) {
	if video {
		return videoThumb(path, edge)
	}
	if edge < 64 {
		edge = 64
	}
	return goThumbnailJPEG(path, edge)
}

// platformVideoThumb: no built-in video thumbnailer is used here yet.
func platformVideoThumb(path string, edge int) ([]byte, error) {
	return nil, errors.New("no platform thumbnailer")
}

// platformTags reads tags from the app's own tag store; this platform has
// no file-attached tag metadata the app can write.
func platformTags(path string) ([]string, error) {
	return fileTagStore.get(path), nil
}

func platformSetTags(path string, tags []string) error {
	return fileTagStore.set(path, tags)
}

// imageResampleJPEG decodes and downsamples in one pass through Go.
func imageResampleJPEG(path string, edge int, quality string) ([]byte, error) {
	return goThumbnailJPEG(path, edge)
}

// decodeLargeImage reads the original for 1:1 zoom.
func decodeLargeImage(path string, edge int) ([]byte, error) {
	_ = edge
	return os.ReadFile(path)
}

// finderLabelNames has no Windows counterpart; rating tags stay unnamed.
func finderLabelNames() []string {
	return []string{"红色", "橙色", "黄色", "绿色", "蓝色", "紫色", "灰色"}
}

// sameDevice: Windows has no Stat_t.Dev, so same local-drive paths are
// treated as one device and UNC shares as another.
func sameDevice(a, b string) bool {
	volA, volB := filepath.VolumeName(a), filepath.VolumeName(b)
	if volA != "" || volB != "" {
		return volA == volB
	}
	// Both plain relative or both UNC (\servershare...).
	return (len(a) >= 2 && a[:2] == `\`) == (len(b) >= 2 && b[:2] == `\`)
}
