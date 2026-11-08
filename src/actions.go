package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

// infoState tracks overlays in the large view.
type infoState struct {
	EXIF bool
	File bool
}

// opState tracks the current rename dialog.
type opState struct {
	Renaming   bool
	RenamePath string
	RenameVal  string
}

func (a *appState) selectedPaths() []string {
	// menuPath is the sidebar row whose menu is open. It is set only while
	// that menu is built, so a command from the menu acts on the row even
	// when the folder is not in the current listing.
	if a.menuPath != "" {
		return []string{a.menuPath}
	}
	if a.InLargeView {
		if item := a.currentItem(); item != nil {
			return []string{item.Path}
		}
	}
	if a.Selected.Len() > 0 {
		var out []string
		for i := range a.Items {
			if a.Selected.Has(a.Items[i].Path) {
				out = append(out, a.Items[i].Path)
			}
		}
		return out
	}
	if a.Chosen < 0 || a.Chosen >= len(a.Visible) {
		return nil
	}
	item := a.visibleItem(a.Chosen)
	if item == nil {
		return nil
	}
	return []string{item.Path}
}

func (a *appState) operationPaths() []string {
	paths := a.selectedPaths()
	if len(paths) > 0 {
		return paths
	}
	if a.InLargeView {
		if item := a.currentItem(); item != nil {
			return []string{item.Path}
		}
	}
	return nil
}

func (a *appState) doCopy() { a.copySelection(false) }

func (a *appState) doCut() { a.copySelection(true) }

// copySelection puts the selection on the system clipboard. Cut keeps the
// FlowVision dimmed-source state until the next paste moves the files.
func (a *appState) copySelection(cut bool) {
	paths := a.selectedPaths()
	if a.InLargeView {
		if item := a.currentItem(); item != nil {
			paths = []string{item.Path}
		}
	}
	if len(paths) == 0 {
		return
	}
	a.Clip = append([]string(nil), paths...)
	a.Cut = nil
	if cut {
		a.Cut = map[string]bool{}
		for _, path := range paths {
			a.Cut[path] = true
		}
	}
	if data, err := transfer.FileData(paths...); err == nil {
		_ = mygo.Clipboard.Write(data)
	}
}

func (a *appState) doCopyTo() { a.transferToFolder(opCopy) }

func (a *appState) doMove() { a.transferToFolder(opMove) }

func (a *appState) transferToFolder(kind fileOp) {
	paths := a.selectedPaths()
	if a.InLargeView {
		if item := a.currentItem(); item != nil {
			paths = []string{item.Path}
		}
	}
	if len(paths) == 0 {
		return
	}
	title := "复制到目录"
	if kind == opMove {
		title = "移动到目录"
	}
	safeGo("actions.go:112", func() {
		dest, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.Window, Title: title, Directory: true})
		if err != nil || len(dest) == 0 {
			return
		}
		a.update(func() {
			a.beginTransfer(fileOperation{op: kind, paths: paths, dest: dest[0]})
		})
	})
}

func (a *appState) doTrash() { a.deleteSelection(false) }

func (a *appState) doPermanentDelete() { a.deleteSelection(true) }

func (a *appState) deleteSelection(permanent bool) {
	paths := a.selectedPaths()
	if a.InLargeView {
		if item := a.currentItem(); item != nil {
			paths = []string{item.Path}
		}
	}
	if len(paths) == 0 {
		return
	}
	if !a.confirmDelete(len(paths), permanent) {
		return
	}
	// Moving to the Trash goes through Finder one file at a time; run it off
	// the main thread so a large selection does not freeze the window.
	a.runOperation(fileOperation{op: opDelete, paths: paths, permanent: permanent})
}

func (a *appState) startRename() {
	paths := a.selectedPaths()
	if len(paths) == 0 {
		return
	}
	a.Renaming = true
	a.RenamePath = paths[0]
	a.RenameVal = filepath.Base(paths[0])
	a.renameArm = ""
	a.RenameInline = len(paths) == 1 && a.inlineRenameIndex(paths[0]) >= 0
	if a.RenameInline {
		a.renameFresh = 3
		a.revealFrames = 2
	}
}

func (a *appState) commitRename() {
	if !a.Renaming || a.RenamePath == "" || a.RenameVal == "" {
		a.Renaming = false
		return
	}
	if a.RenameVal == filepath.Base(a.RenamePath) {
		a.Renaming = false
		return
	}
	if err := a.performTracked(fileOperation{op: opRename, paths: []string{a.RenamePath}, newName: a.RenameVal}); err != nil {
		return
	}
	a.Renaming = false
	a.RenamePath = ""
}

func (a *appState) newFolder() { a.promptFolder(nil) }

func (a *appState) newFolderWithSelection() {
	paths := a.selectedPaths()
	if len(paths) == 0 {
		a.promptFolder(nil)
		return
	}
	a.promptFolder(paths)
}

func (a *appState) promptFolder(take []string) {
	if a.Root == "" {
		return
	}
	a.NewFolder = true
	a.FolderName = "新建文件夹"
	a.FolderTake = append([]string(nil), take...)
}

func (a *appState) commitFolder() {
	if !a.NewFolder {
		return
	}
	name := a.FolderName
	if err := validBaseName(name); err != nil {
		a.toast(err.Error())
		return
	}
	dir := flowUnique(filepath.Join(a.Root, name), false, nil)
	if filepath.Base(dir) != name {
		a.toast("文件或文件夹已存在：" + name)
		return
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		a.toast(err.Error())
		return
	}
	take := append([]string(nil), a.FolderTake...)
	a.NewFolder = false
	a.FolderName = ""
	a.FolderTake = nil
	if len(take) == 0 {
		a.PendingSelect = []string{dir}
		a.refresh()
		return
	}
	_ = a.performTracked(fileOperation{op: opMove, paths: take, dest: dir})
}

func (a *appState) mediaContextMenu(m *ui.Menu, index int) {
	if item := a.visibleItem(index); item != nil {
		a.Chosen = index
		if !a.Selected.Has(item.Path) {
			a.Selected.Clear()
			a.Selected.Add(item.Path)
		}
	}
	a.fileMenu(m, true)
}

// fileContextMenu is the menu of empty space in a folder: folder-level
// actions only (new, paste, refresh), no item actions.
func (a *appState) fileContextMenu(m *ui.Menu) {
	none := a.Root == ""
	if m.Item("粘贴").Disabled(none || !a.canPaste()).Chosen() {
		a.doPaste(false)
	}
	if m.Item("移动到此").Disabled(none || !a.canPaste()).Chosen() {
		a.doPaste(true)
	}
	m.Separator()
	if m.Item("新建文件夹").Disabled(none).Chosen() {
		a.newFolder()
	}
	if m.Item("新建文本文件").Disabled(none).Chosen() {
		a.newTextFile()
	}
	m.Separator()
	if m.Item("刷新").Disabled(none).Chosen() {
		a.refresh()
	}
	if m.Item("在终端中打开").Disabled(none).Chosen() {
		a.openInTerminal()
	}
	favLabel := "收藏当前目录"
	if favoriteContains(a.Profiles.Favorites, a.Root) {
		favLabel = "取消收藏当前目录"
	}
	if m.Item(favLabel).Disabled(none).Chosen() {
		a.toggleFavorite(a.Root)
	}
}

func (a *appState) largeContextMenu(m *ui.Menu, item *mediaItem) {
	m.Submenu("缩放", func(sub *ui.Menu) {
		if sub.Item("适应窗口").Chosen() {
			a.resetZoom()
		}
		if sub.Item("实际大小").Chosen() {
			a.zoom100()
		}
	})
	if item != nil && item.IsVideo && m.Item("另存视频…").Chosen() {
		a.chooseVideoSave(item)
	}
	m.Separator()
	a.fileMenu(m, false)
}

// fileMenu is the menu of one or more items. The top level stays short;
// everything else is in native submenus. Shortcuts live in the app menu
// and key handlers, so moving an item here does not change its key.
func (a *appState) fileMenu(m *ui.Menu, canOpen bool) {
	paths := a.operationPaths()
	none := len(paths) == 0
	one := len(paths) == 1
	var video *mediaItem
	if a.InLargeView {
		if item := a.currentItem(); item != nil && item.IsVideo {
			video = item
		}
	}
	videos := !none && allVideos(paths)

	if canOpen && m.Item("打开").Disabled(none).Chosen() {
		if a.menuPath != "" && isDir(a.menuPath) {
			a.openFolder(a.menuPath)
		} else {
			a.openItem(a.Chosen)
		}
	}
	if one && isDir(paths[0]) && m.Item("在新标签页中打开").Chosen() {
		a.openInNewTab(paths[0])
	}
	if one && videos && m.Item("用系统播放器打开").Chosen() {
		_ = openWithDefault(paths[0])
	}
	a.openWithMenu(m, paths)
	if m.Item(revealLabel).Disabled(none).Chosen() {
		a.revealInFinder()
	}
	if m.Item("文件简介").Disabled(none).Chosen() {
		a.showInfo()
	}
	m.Separator()
	if m.Item("复制").Disabled(none).Chosen() {
		a.doCopy()
	}
	pasteDest := a.Root
	if a.menuPath != "" && isDir(a.menuPath) {
		pasteDest = a.menuPath
	}
	if m.Item("粘贴").Disabled(pasteDest == "").Chosen() {
		a.doPaste(false)
	}
	if len(paths) > 1 {
		if m.Item("批量重命名…").Chosen() {
			a.startBatchRename()
		}
	} else if m.Item("重命名…").Disabled(none).Chosen() {
		a.startRename()
	}
	hasFiles := false
	for _, path := range paths {
		if path != "" && !isDir(path) {
			hasFiles = true
			break
		}
	}
	if m.Item("按文件夹名重命名").Disabled(!hasFiles).Chosen() {
		a.renameByFolderName()
	}
	m.Separator()

	m.Submenu("复制与移动", func(sub *ui.Menu) {
		if sub.Item("剪切").Disabled(none).Chosen() {
			a.doCut()
		}
		if sub.Item("复制路径").Disabled(none).Chosen() {
			a.copyPath()
		}
		if sub.Item("制作副本").Disabled(none).Chosen() {
			a.duplicateSelection()
		}
		sub.Separator()
		if sub.Item("复制到…").Disabled(none).Chosen() {
			a.doCopyTo()
		}
		if sub.Item("移动到…").Disabled(none).Chosen() {
			a.doMove()
		}
		if sub.Item("复制到目标").Disabled(none).Chosen() {
			a.copyToConfigured()
		}
		if sub.Item("复制到下载").Disabled(none).Chosen() {
			a.transferToDownloads(opCopy)
		}
		if sub.Item("移动到下载").Disabled(none).Chosen() {
			a.transferToDownloads(opMove)
		}
		sub.Separator()
		if sub.Item("用所选项目新建文件夹").Disabled(none).Chosen() {
			a.newFolderWithSelection()
		}
		if len(paths) > 1 && allDirectories(paths) && sub.Item("提取子文件夹文件并归集").Chosen() {
			a.collectSelection()
		}
	})
	if !videos && !(one && isDir(paths[0])) {
		m.Submenu("旋转", func(sub *ui.Menu) {
			if sub.Item("向右旋转 90°").Disabled(none).Chosen() {
				a.rotateSelection(90)
			}
			if sub.Item("向左旋转 90°").Disabled(none).Chosen() {
				a.rotateSelection(270)
			}
			if sub.Item("旋转 180°").Disabled(none).Chosen() {
				a.rotateSelection(180)
			}
		})
	}
	if videos {
		m.Submenu("视频", func(sub *ui.Menu) {
			if video != nil && sub.Item("截存当前画面").Chosen() {
				a.captureCurrentFrame()
			}
			if video != nil && sub.Item("无损裁剪片段…").Chosen() {
				a.promptTrim()
			}
			if sub.Item("裁剪视频尺寸…").Chosen() {
				a.promptCrop(paths)
			}
		})
	}
	if !none {
		m.Submenu("压缩", func(sub *ui.Menu) {
			if allArchives(paths) {
				if sub.Item("解压到当前目录").Chosen() {
					a.extractSelection(false)
				}
				if sub.Item("解压并删除压缩包").Chosen() {
					a.extractSelection(true)
				}
				sub.Separator()
			}
			if sub.Item("压缩为 ZIP").Chosen() {
				a.quickCompress()
			}
			if sub.Item("加密压缩…").Chosen() {
				a.encryptCompress(false)
			}
			if _, ok := configuredZipPassword(a.Profiles); ok && sub.Item("使用默认密码加密压缩").Chosen() {
				a.compressWithPassword(false)
			}
			if sub.Item("压缩并删除源文件").Chosen() {
				a.compressPlain(true)
			}
		})
	}
	if tagsSupported && len(paths) > 0 {
		a.tagMenu(m, paths) // carries 评级 as its last entry
	} else {
		a.ratingMenu(m, paths)
	}
	a.shareMenu(m, paths)
	if one {
		label := "添加到收藏"
		if favoriteContains(a.Profiles.Favorites, paths[0]) {
			label = "取消收藏"
		}
		if !isDir(paths[0]) {
			label = ""
		}
		if label != "" && m.Item(label).Chosen() {
			a.toggleFavorite(paths[0])
		}
		if filepath.Dir(paths[0]) != a.Root && m.Item(openParentLabel).Chosen() {
			a.showOriginalFolder()
		}
	}
	m.Separator()
	if m.Item("移到废纸篓").Disabled(none).Chosen() {
		a.doTrash()
	}
}

func (a *appState) quickCompress() {
	if a.Profiles.ZipUsePassword {
		if _, ok := configuredZipPassword(a.Profiles); !ok {
			a.toast("默认压缩密码为空，请先在设置中配置。")
			return
		}
		a.compressWithPassword(false)
		return
	}
	a.compressPlain(false)
}

func (a *appState) compressPlain(drop bool) {
	paths := a.operationPaths()
	if len(paths) == 0 {
		return
	}
	a.startArchive(fileOperation{op: opCompress, paths: paths, dropSource: drop})
}

func (a *appState) compressWithPassword(drop bool) {
	pass, ok := configuredZipPassword(a.Profiles)
	if !ok {
		a.toast("默认压缩密码为空。")
		return
	}
	paths := a.operationPaths()
	if len(paths) == 0 {
		return
	}
	a.startArchive(fileOperation{op: opCompress, paths: paths, password: pass, dropSource: drop})
}

func (a *appState) encryptCompress(drop bool) {
	paths := a.operationPaths()
	if len(paths) == 0 {
		return
	}
	a.promptZip("compress", paths, drop)
}

func (a *appState) extractSelection(drop bool) {
	paths := archivesOnly(a.operationPaths())
	if len(paths) == 0 {
		a.toast("请先选择压缩包文件。")
		return
	}
	a.startArchive(fileOperation{op: opExtract, paths: paths, dropSource: drop})
}

func (a *appState) promptZip(mode string, paths []string, drop bool) {
	a.ZipAsk = true
	a.ZipFor = mode
	a.ZipPaths = append([]string(nil), paths...)
	a.ZipDrop = drop
	if pass, ok := configuredZipPassword(a.Profiles); ok {
		a.ZipPass = pass
	} else {
		a.ZipPass = ""
	}
}

func (a *appState) commitZipPrompt() {
	if strings.TrimSpace(a.ZipPass) == "" {
		a.toast("密码不能为空。")
		return
	}
	op := fileOperation{paths: append([]string(nil), a.ZipPaths...), password: a.ZipPass, dropSource: a.ZipDrop}
	if a.ZipFor == "extract" {
		op.op = opExtract
	} else {
		op.op = opCompress
	}
	a.ZipAsk = false
	a.ZipPass = ""
	a.startArchive(op)
}

func (a *appState) zipDialog(c *ui.Context) {
	if !a.ZipAsk {
		return
	}
	title := "加密压缩 ZIP"
	detail := "请输入 ZIP 密码。"
	if a.ZipFor == "extract" {
		title = "解压密码"
		detail = "这个压缩包需要密码。"
	}
	m := ui.Modal(c, &a.ZipAsk, func() {
		ui.Text(c, title).Bold().FontSize(14)
		ui.Text(c, detail).FontSize(12).TextColor(c.Theme().TextMuted)
		field := ui.TextInput(c, &a.ZipPass).Width(280).Password()
		if field.Submitted() {
			a.commitZipPrompt()
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c.Key("zip-cancel"), "取消").Clicked() {
				a.ZipAsk = false
				a.ZipPass = ""
			}
			if ui.Button(c.Key("zip-ok"), "确定").Clicked() {
				a.commitZipPrompt()
			}
		})
	})
	if m.Dismissed() {
		a.ZipPass = ""
	}
}

// renameDialog shows a real editable field and commits on Enter.
func (a *appState) renameDialog(c *ui.Context) {
	if !a.Renaming || a.RenameInline {
		return
	}
	ui.Modal(c, &a.Renaming, func() {
		ui.Text(c, "重命名").Bold().FontSize(14)
		field := ui.TextInput(c, &a.RenameVal).Width(280)
		if field.Submitted() {
			a.commitRename()
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c.Key("rename-cancel"), "取消").Clicked() {
				a.Renaming = false
			}
			if ui.Button(c.Key("rename-ok"), "确定").Clicked() {
				a.commitRename()
			}
		})
	})
}

// infoOverlay shows real EXIF and file metadata in the large view.
func (a *appState) infoOverlay(c *ui.Context, item *mediaItem) {
	if !a.Info.EXIF && !a.Info.File {
		return
	}
	if !item.InfoLoaded {
		// Probing spawns processes on macOS: never on the main thread
		// while drawing. The overlay fills in when the probe answers.
		if a.LargeIndex >= 0 && a.LargeIndex < len(a.Visible) {
			a.requestInfo(a.Visible[a.LargeIndex])
		}
	}
	t := c.Theme()
	lines := []string{}
	if a.Info.EXIF {
		if item.Info.Camera != "" || item.Info.Model != "" {
			lines = append(lines, fmt.Sprintf("相机: %s %s", item.Info.Camera, item.Info.Model))
		}
		if item.Info.Created != "" {
			lines = append(lines, "拍摄: "+item.Info.Created)
		}
		if item.Info.Width > 0 {
			lines = append(lines, fmt.Sprintf("尺寸: %d×%d", item.Info.Width, item.Info.Height))
		}
		if item.Info.Duration != "" {
			lines = append(lines, "时长: "+item.Info.Duration)
		}
		if !item.InfoLoaded {
			lines = append(lines, "读取中…")
		}
	}
	if a.Info.File {
		lines = append(lines, "路径: "+item.Path)
		lines = append(lines, fmt.Sprintf("大小: %.1f MB", float64(item.Size)/(1024*1024)))
		lines = append(lines, "修改: "+time.Unix(item.ModTime, 0).Format("2006-01-02 15:04:05"))
	}
	if len(lines) == 0 {
		return
	}
	panel := ui.Column(c).Attach(ui.AnchorTopRight, ui.AnchorTopRight).Width(320).Padding(12).Gap(4).
		Background(t.Surface).Border(1, t.Border).Radius(8)
	panel.Children(func() {
		for _, line := range lines {
			ui.Text(c, line).FontSize(11).MaxLines(1).SingleLine()
		}
	})
}

// batchState tracks the batch rename dialog.
type batchState struct {
	Open    bool
	Rule    renameRule
	Preview map[string]string
}

func (a *appState) startBatchRename() {
	paths := a.selectedPaths()
	if len(paths) == 0 {
		return
	}
	a.Batch.Open = true
	a.Batch.Rule = renameRule{Mode: "format", Pattern: "{name}", StartNumber: 1}
	a.Batch.Preview = previewRenames(paths, a.Batch.Rule, time.Now())
}

func (a *appState) updateBatchPreview() {
	a.Batch.Preview = previewRenames(a.selectedPaths(), a.Batch.Rule, time.Now())
}

func (a *appState) commitBatchRename() {
	if len(a.Batch.Preview) == 0 {
		a.Batch.Open = false
		return
	}
	var mappings []pathPair
	for src, dst := range a.Batch.Preview {
		if src != dst {
			mappings = append(mappings, pathPair{From: src, To: dst})
		}
	}
	if err := a.performTracked(fileOperation{op: opRename, mappings: mappings}); err != nil {
		return
	}
	a.Batch.Open = false
	a.Batch.Preview = nil
}

func (a *appState) batchDialog(c *ui.Context) {
	if !a.Batch.Open {
		return
	}
	m := ui.Modal(c, &a.Batch.Open, func() {
		ui.Text(c, "批量重命名").Bold().FontSize(16)
		ui.Text(c, "顺序：替换、格式、前缀、后缀。默认保留扩展名。").FontSize(11).TextColor(c.Theme().TextMuted)
		if ui.TextInput(c, &a.Batch.Rule.Prefix).Label("前缀").Width(480).Changed() {
			a.updateBatchPreview()
		}
		if ui.TextInput(c, &a.Batch.Rule.Suffix).Label("后缀").Width(480).Changed() {
			a.updateBatchPreview()
		}
		if ui.TextInput(c, &a.Batch.Rule.Find).Label("查找").Width(230).Changed() {
			a.updateBatchPreview()
		}
		if ui.TextInput(c, &a.Batch.Rule.Replace).Label("替换为").Width(230).Changed() {
			a.updateBatchPreview()
		}
		if ui.TextInput(c, &a.Batch.Rule.Pattern).Label("格式").Width(480).Changed() {
			a.updateBatchPreview()
		}
		ui.Text(c, "{name} {folder} {ext} {index} {index:03} {date} {time}").FontSize(11).TextColor(c.Theme().TextMuted)
		ui.Divider(c)
		ui.Text(c, "预览").Bold().FontSize(12)
		for _, src := range a.selectedPaths() {
			dst := a.Batch.Preview[src]
			if dst == "" {
				continue
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, filepath.Base(src)).FontSize(11).TextColor(c.Theme().TextMuted).MaxLines(1)
				ui.Text(c, "→").FontSize(11)
				ui.Text(c, filepath.Base(dst)).FontSize(11).MaxLines(1)
			})
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c.Key("batch-cancel"), "取消").Clicked() {
				a.Batch.Open = false
			}
			if ui.Button(c.Key("batch-ok"), "执行").Clicked() {
				a.commitBatchRename()
			}
		})
	})
	if m.Dismissed() {
		a.Batch.Preview = nil
	}
}

func (a *appState) currentDestination() string {
	if a.Profiles.Active < 0 || a.Profiles.Active >= len(a.Profiles.Profiles) {
		return ""
	}
	return a.Profiles.Profiles[a.Profiles.Active].Destination
}

func (a *appState) setCopyDestination() {
	dest, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.Window, Title: "设置复制目录", Directory: true})
	if err != nil || len(dest) == 0 || a.Profiles.Active < 0 || a.Profiles.Active >= len(a.Profiles.Profiles) {
		return
	}
	a.update(func() {
		a.Profiles.Profiles[a.Profiles.Active].Destination = dest[0]
		savePrefs(a.Profiles)
	})
}

func (a *appState) copyToConfigured() { a.copyToDestination(a.configuredCopyRequest()) }

// configuredCopyRequest silently copies the selection into the active
// profile's destination.
func (a *appState) configuredCopyRequest() copyRequest {
	return copyRequest{dest: a.currentDestination()}
}

// copyLargeRule says which items a copy takes while the large view is open.
type copyLargeRule int

const (
	// copyLargeSelection ignores the large view and copies the selection.
	copyLargeSelection copyLargeRule = iota
	// copyLargeCurrentOrSelection copies the shown item, or the selection
	// when no item is shown.
	copyLargeCurrentOrSelection
	// copyLargeCurrentOnly copies the shown item, or nothing.
	copyLargeCurrentOnly
)

// copyRequest describes one "copy to a configured folder" command.
type copyRequest struct {
	dest        string
	missingDest string // toast when dest is unset or not a folder; "" stays silent
	noItems     string // toast when nothing qualifies; "" stays silent
	large       copyLargeRule
	// keep filters candidates; item is the shown item in the large view and
	// nil for selected paths. A nil keep accepts everything.
	keep func(path string, item *mediaItem) bool
}

// copyToDestination validates the destination, collects the paths the
// request's rules pick, and starts one copy transfer.
func (a *appState) copyToDestination(req copyRequest) {
	if req.dest == "" || !isDir(req.dest) {
		if req.missingDest != "" {
			a.toast(req.missingDest)
		}
		return
	}
	paths := a.copyCandidates(req)
	if len(paths) == 0 {
		if req.noItems != "" {
			a.toast(req.noItems)
		}
		return
	}
	a.beginTransfer(fileOperation{op: opCopy, paths: paths, dest: req.dest})
}

func (a *appState) copyCandidates(req copyRequest) []string {
	if a.InLargeView && req.large != copyLargeSelection {
		item := a.currentItem()
		if item != nil {
			if req.keep == nil || req.keep(item.Path, item) {
				return []string{item.Path}
			}
			return nil
		}
		if req.large == copyLargeCurrentOnly {
			return nil
		}
	}
	selected := a.selectedPaths()
	if req.keep == nil {
		return selected
	}
	var paths []string
	for _, path := range selected {
		if req.keep(path, nil) {
			paths = append(paths, path)
		}
	}
	return paths
}

func (a *appState) canPaste() bool {
	paths, _ := a.filesForPaste(false)
	return len(paths) > 0
}

func (a *appState) filesForPaste(forceMove bool) ([]string, bool) {
	paths := readClipboardFiles()
	if len(paths) == 0 {
		paths = append([]string(nil), a.Clip...)
	}
	cut := forceMove
	if !forceMove && len(a.Cut) > 0 && samePaths(paths, a.Clip) {
		cut = true
	}
	return paths, cut
}

func samePaths(aPaths, bPaths []string) bool {
	if len(aPaths) != len(bPaths) {
		return false
	}
	seen := map[string]int{}
	for _, path := range aPaths {
		seen[pathKey(path)]++
	}
	for _, path := range bPaths {
		key := pathKey(path)
		seen[key]--
		if seen[key] < 0 {
			return false
		}
	}
	return true
}

func readClipboardFiles() []string {
	data, err := mygo.Clipboard.Read(transfer.FileList, transfer.URIList)
	if err != nil {
		return nil
	}
	files, err := data.Files()
	if err != nil {
		return nil
	}
	return files
}

func (a *appState) doPaste(forceMove bool) {
	dest := a.Root
	if a.menuPath != "" && isDir(a.menuPath) {
		dest = a.menuPath
	}
	if dest == "" {
		return
	}
	paths, cut := a.filesForPaste(forceMove)
	if len(paths) == 0 {
		a.toast("剪贴板里没有文件")
		return
	}
	op := fileOperation{op: opCopy, paths: paths, dest: dest}
	if cut {
		op.op = opMove
		op.cut = true
	}
	a.beginTransfer(op)
}

func (a *appState) openInTerminal() {
	path := a.Root
	if paths := a.operationPaths(); len(paths) == 1 && paths[0] != "" {
		path = paths[0]
		if !isDir(path) {
			path = filepath.Dir(path)
		}
	}
	if path == "" {
		return
	}
	if err := openTerminalAt(path); err != nil {
		a.toast("无法打开终端")
	}
}

func (a *appState) revealInFinder() {
	paths := a.selectedPaths()
	if a.InLargeView {
		if item := a.currentItem(); item != nil {
			paths = []string{item.Path}
		}
	}
	if len(paths) == 0 {
		return
	}
	_ = revealInFileBrowser(paths[0])
}

func (a *appState) copyPath() {
	paths := a.selectedPaths()
	if a.InLargeView {
		if item := a.currentItem(); item != nil {
			paths = []string{item.Path}
		}
	}
	if len(paths) == 0 {
		return
	}
	mygo.Clipboard.WriteText(paths[0])
	a.toast(paths[0])
}

func (a *appState) quickRenameFolder() {
	if a.Root == "" {
		return
	}
	var paths []string
	for _, index := range a.Visible {
		paths = append(paths, a.Items[index].Path)
	}
	if len(paths) == 0 {
		a.toast("当前目录没有可重命名的文件")
		return
	}
	rule := a.Profiles.QuickRename
	mappings := quickRenameMappings(paths, pathName(a.Root), rule)
	_ = a.performTracked(fileOperation{op: opRename, mappings: mappings})
}

// filesForFolderRename returns selected files (or the current file) in
// Visible order, skipping folders.
func (a *appState) filesForFolderRename() []string {
	wanted := map[string]struct{}{}
	for _, path := range a.operationPaths() {
		if path == "" || isDir(path) {
			continue
		}
		wanted[pathKey(path)] = struct{}{}
	}
	if len(wanted) == 0 {
		return nil
	}
	var out []string
	for _, index := range a.Visible {
		item := a.visibleItem(index)
		if item == nil || item.IsDir {
			continue
		}
		if _, ok := wanted[pathKey(item.Path)]; ok {
			out = append(out, item.Path)
		}
	}
	if len(out) == 0 {
		for _, path := range a.operationPaths() {
			if path != "" && !isDir(path) {
				out = append(out, path)
			}
		}
	}
	return out
}

func (a *appState) confirmFolderRename(n int, exampleFrom, exampleTo string) bool {
	message := fmt.Sprintf("按文件夹名重命名 %d 个文件？", n)
	detail := fmt.Sprintf("例如：%s → %s", filepath.Base(exampleFrom), filepath.Base(exampleTo))
	if n == 1 {
		message = "按文件夹名重命名此文件？"
		detail = fmt.Sprintf("%s → %s", filepath.Base(exampleFrom), filepath.Base(exampleTo))
	}
	res, err := mygo.Dialog.Message(mygo.MessageOptions{
		Parent:       a.Window,
		Type:         mygo.MessageWarning,
		Title:        "按文件夹名重命名",
		Message:      message,
		Detail:       detail,
		Buttons:      []string{"重命名", "取消"},
		CancelButton: 1,
	})
	return err == nil && res.Button == 0
}

// renameByFolderName renames the selected files to <parent>_<seq>.ext
// (or <parent>.ext for one file), after a short confirm.
func (a *appState) renameByFolderName() {
	paths := a.filesForFolderRename()
	if len(paths) == 0 {
		a.toast("没有可重命名的文件")
		return
	}
	mappings := folderNameRenameMappings(paths)
	var changed []pathPair
	for _, m := range mappings {
		if pathKey(m.From) != pathKey(m.To) {
			changed = append(changed, m)
		}
	}
	if len(changed) == 0 {
		a.toast("名称已是目标形式")
		return
	}
	if a.Window != nil && !a.confirmFolderRename(len(changed), changed[0].From, changed[0].To) {
		return
	}
	a.runOperation(fileOperation{op: opRename, mappings: changed, paths: mappingSources(changed)})
}

func mappingSources(mappings []pathPair) []string {
	out := make([]string, 0, len(mappings))
	for _, m := range mappings {
		out = append(out, m.From)
	}
	return out
}

func (a *appState) isCut(path string) bool {
	return a.Cut != nil && a.Cut[path]
}

func (a *appState) folderDialog(c *ui.Context) {
	if !a.NewFolder {
		return
	}
	m := ui.Modal(c, &a.NewFolder, func() {
		title := "新建文件夹"
		if len(a.FolderTake) > 0 {
			title = "用所选项目新建文件夹"
		}
		ui.Text(c, title).Bold().FontSize(14)
		field := ui.TextInput(c, &a.FolderName).Width(280)
		if field.Submitted() {
			a.commitFolder()
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c.Key("folder-cancel"), "取消").Clicked() {
				a.NewFolder = false
				a.FolderTake = nil
			}
			if ui.Button(c.Key("folder-ok"), "确定").Clicked() {
				a.commitFolder()
			}
		})
	})
	if m.Dismissed() {
		a.FolderTake = nil
		a.FolderName = ""
	}
}

func (a *appState) beginTransfer(op fileOperation) {
	if op.dest == "" || len(op.paths) == 0 {
		return
	}
	if !op.replace && hasNameConflict(op.paths, op.dest) {
		choice, ok := a.askConflict()
		if !ok {
			return
		}
		op.replace = choice == 1
	}
	a.runOperation(op)
}

func (a *appState) askConflict() (int, bool) {
	res, err := mygo.Dialog.Message(mygo.MessageOptions{
		Parent:       a.Window,
		Type:         mygo.MessageWarning,
		Title:        "文件已存在",
		Message:      "目标位置已有同名文件或文件夹",
		Detail:       "自动重命名会保留两边。覆盖会替换同名文件，同名文件夹会合并内容，且不能撤销。",
		Buttons:      []string{"自动重命名", "覆盖", "取消"},
		CancelButton: 2,
	})
	if err != nil || res.Button == 2 {
		return 0, false
	}
	return res.Button, true
}

func (a *appState) confirmDelete(n int, permanent bool) bool {
	message := fmt.Sprintf("确定要把 %d 个项目移到废纸篓吗？", n)
	detail := "之后可以从废纸篓放回。"
	if permanent {
		message = fmt.Sprintf("确定要永久删除 %d 个项目吗？", n)
		detail = "此操作无法撤销。"
	}
	res, err := mygo.Dialog.Message(mygo.MessageOptions{
		Parent:             a.Window,
		Type:               mygo.MessageWarning,
		Title:              "删除",
		Message:            message,
		Detail:             detail,
		Buttons:            []string{"删除", "取消"},
		CancelButton:       1,
		DestructiveButtons: []int{0},
	})
	return err == nil && res.Button == 0
}

func downloadsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Downloads")
	if !isDir(dir) {
		return "", fmt.Errorf("下载目录不存在")
	}
	return dir, nil
}

func (a *appState) transferToDownloads(kind fileOp) {
	dir, err := downloadsDir()
	if err != nil {
		a.toast(err.Error())
		return
	}
	paths := a.selectedPaths()
	if a.InLargeView {
		if item := a.currentItem(); item != nil {
			paths = []string{item.Path}
		}
	}
	if len(paths) == 0 {
		return
	}
	a.beginTransfer(fileOperation{op: kind, paths: paths, dest: dir})
}

func (a *appState) newTextFile() {
	if a.Root == "" {
		return
	}
	path := flowUnique(filepath.Join(a.Root, "未命名.txt"), false, nil)
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		a.toast(err.Error())
		return
	}
	a.PendingSelect = []string{path}
	a.Renaming = true
	a.RenamePath = path
	a.RenameVal = filepath.Base(path)
	a.refresh()
}

func (a *appState) dragPaths(path string) []string {
	if path != "" && a.Selected.Has(path) {
		if paths := a.selectedPaths(); len(paths) > 0 {
			return paths
		}
	}
	if path == "" {
		return nil
	}
	return []string{path}
}

// folderContextMenu is the file-operation menu of a sidebar folder.
// menuPath makes the shared file commands act on path.
func (a *appState) folderContextMenu(m *ui.Menu, path string) {
	a.menuPath = path
	defer func() { a.menuPath = "" }()
	a.fileMenu(m, true)
}

func (a *appState) historyContextMenu(m *ui.Menu, path string) {
	a.folderContextMenu(m, path)
	m.Separator()
	if m.Item("清除历史记录").Chosen() {
		a.clearHistory()
	}
}

// bindFolderRow makes a sidebar folder a drag source, a drop target, and
// a file-operation menu. Drag moves or copies the folder; a drop on it
// files the dragged items there.
func (a *appState) bindFolderRow(c *ui.Context, row ui.Element, path string, menu func(*ui.Menu)) {
	a.fileDragSource(row, path)
	a.folderDropTarget(c, row, path)
	row.ContextMenu(menu)
}

func (a *appState) fileDragSource(card ui.Element, path string) {
	card.DragDataFrom(func() transfer.Data {
		paths := a.dragPaths(path)
		data, err := transfer.FileData(paths...)
		if err != nil {
			return transfer.Data{}
		}
		return data
	}, transfer.DragOptions{Operations: transfer.Copy | transfer.Move})
	if card.Dragging() && !a.isCut(path) {
		card.Opacity(0.55)
	}
}

// It runs every frame for each sidebar folder, folder row and the main view,
// so it must not touch the file system: an os.Stat per row per frame stalled
// the main thread (and every text redraw with it) on slow or sleeping
// volumes. receiveFolderDrop checks that dest is a folder when a drop lands.
func (a *appState) folderDropTarget(c *ui.Context, el ui.Element, dest string) {
	if dest == "" {
		return
	}
	opts := transfer.DropOptions{
		Formats:    []transfer.Format{transfer.FileList, transfer.URIList},
		Operations: transfer.Copy | transfer.Move,
	}
	if paths, over := ui.DragOver[[]string](el); over && len(paths) > 0 {
		el.Border(2, c.Theme().Accent)
	}
	if _, over := ui.DataDragOver(el, opts); over {
		el.Border(2, c.Theme().Accent)
	}
	if paths, ok := ui.Drop[[]string](el); ok {
		a.receiveFolderDrop(paths, dest, true)
		return
	}
	if drop, ok := ui.DropData(el, opts); ok {
		files, err := drop.Data.Files()
		if err != nil || len(files) == 0 {
			return
		}
		a.receiveFolderDrop(files, dest, c.Modifiers()&ui.Alt == 0)
	}
}

func (a *appState) receiveFolderDrop(files []string, dest string, move bool) {
	if dest == "" || !isDir(dest) || len(files) == 0 {
		return
	}
	var paths []string
	for _, path := range files {
		if path == "" || pathKey(path) == pathKey(dest) || pathKey(filepath.Dir(path)) == pathKey(dest) {
			continue
		}
		if copiesIntoSelf(path, dest) {
			a.toast("不能将文件或文件夹复制到自身或其子目录中。")
			return
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return
	}
	if move && !sameDevice(paths[0], dest) {
		move = false
	}
	kind := opCopy
	if move {
		kind = opMove
	}
	a.beginTransfer(fileOperation{op: kind, paths: paths, dest: dest})
}

func toggleFavoriteList(list []string, path string) []string {
	path = filepath.Clean(path)
	if path == "" || path == "." {
		return list
	}
	for i, item := range list {
		if pathKey(item) == pathKey(path) {
			return append(append([]string{}, list[:i]...), list[i+1:]...)
		}
	}
	return append([]string{path}, list...)
}

func favoriteContains(list []string, path string) bool {
	for _, item := range list {
		if pathKey(item) == pathKey(path) {
			return true
		}
	}
	return false
}

func (a *appState) toggleFavorite(path string) {
	path = filepath.Clean(path)
	if path == "" || !isDir(path) {
		return
	}
	a.Profiles.Favorites = toggleFavoriteList(a.Profiles.Favorites, path)
	savePrefs(a.Profiles)
}

func frameFileName(videoPath string, at float64) string {
	base := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	ms := int(at * 1000)
	if ms < 0 {
		ms = 0
	}
	return fmt.Sprintf("%s_frame_%d.png", base, ms)
}

func writeVideoFrame(videoPath string, at float64, dest string) error {
	if at < 0 {
		at = 0
	}
	cmd := exec.Command(toolPath("ffmpeg"), "-hide_banner", "-loglevel", "error", "-y",
		"-ss", fmt.Sprintf("%.3f", at), "-i", videoPath, "-frames:v", "1", dest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

func (a *appState) captureCurrentFrame() {
	item := a.currentItem()
	if item == nil || !item.IsVideo || a.Video == nil {
		a.toast("请先打开视频")
		return
	}
	if a.videoPlaying() {
		a.toggleVideoPlayback()
	}
	at := a.playhead()
	dest := flowUnique(filepath.Join(filepath.Dir(item.Path), frameFileName(item.Path, at)), false, nil)
	path := item.Path
	safeGo("actions.go:1119", func() {
		err := writeVideoFrame(path, at, dest)
		if a.Window == nil {
			return
		}
		a.update(func() {
			if err != nil {
				a.toast("抓取当前视频帧失败。")
				return
			}
			a.toast("画面已保存：" + filepath.Base(dest))
			a.KeepLarge = true
			a.PendingLarge = path
			a.refresh()
		})
	})
}

func rotateImageFile(path string, degrees int) error {
	switch degrees {
	case 90, 180, 270:
	default:
		return fmt.Errorf("不支持的旋转角度")
	}
	cmd := execCommand("sips", "-r", strconv.Itoa(degrees), path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

func forgetThumb(path string) {
	for _, edge := range []int{384, 512, 768, 1024} {
		thumbLRU.Delete(thumbCacheKey(path, edge))
	}
}

func (a *appState) rotateSelection(degrees int) {
	var media []string
	for _, path := range a.operationPaths() {
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
		if isImageFile(ext) || isVideoFile(ext) {
			media = append(media, path)
		}
	}
	if len(media) == 0 {
		a.toast("请先选择图片或视频")
		return
	}
	work := a.beginWork("旋转", len(media))
	safeGo("actions.go:1173", func() {
		defer a.endWork(work)
		var failed []string
		for _, path := range media {
			ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
			var err error
			if isVideoFile(ext) {
				err = rotateVideoFile(path, degrees)
			} else {
				err = rotateImageFile(path, degrees)
			}
			if err != nil {
				failed = append(failed, filepath.Base(path))
			}
			a.stepWork(work)
		}
		if a.Window == nil {
			return
		}
		a.update(func() {
			for _, path := range media {
				forgetThumb(path)
				for i := range a.Items {
					if a.Items[i].Path == path {
						a.Items[i].Bitmap = nil
						a.Items[i].Large = nil
						a.Items[i].Decoded = false
						a.Items[i].LargeLoading = false
						a.Items[i].Loading = false
						a.Items[i].InfoLoaded = false
					}
				}
			}
			if a.Video != nil {
				for _, path := range media {
					if a.Video.path == path {
						a.closeVideo()
						break
					}
				}
			}
			if len(failed) > 0 {
				a.toast("部分文件旋转失败：" + previewNames(failed))
			} else {
				a.toast("旋转完成")
			}
			a.refresh()
		})
	})
}

func (a *appState) importDropped(files []string) {

	if a.Root == "" || len(files) == 0 {
		return
	}
	var paths []string
	for _, path := range files {
		if path == "" || pathKey(filepath.Dir(path)) == pathKey(a.Root) {
			continue
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return
	}
	kind := opCopy
	if sameDevice(paths[0], a.Root) {
		kind = opMove
	}
	a.beginTransfer(fileOperation{op: kind, paths: paths, dest: a.Root})
}

func (a *appState) collectSelection() {
	paths := a.operationPaths()
	if len(paths) < 2 || !allDirectories(paths) {
		a.toast("请仅选择多个文件夹。")
		return
	}
	parent := a.Root
	if parent == "" {
		parent = filepath.Dir(paths[0])
	}
	work := a.beginWork("归集", 0)
	safeGo("actions.go:1255", func() {
		result, err := collectSubfolderFiles(paths, parent, time.Now())
		if a.Window == nil {
			return
		}
		a.update(func() {
			a.endWork(work)
			if err != nil {
				a.toast(err.Error())
				return
			}
			applied := fileOperation{op: opCopy, created: []string{result.folder}}
			if inverse, ok := inverseOf(applied); ok {
				a.Undo.record(applied, inverse)
			}
			msg := fmt.Sprintf("已归集 %d 个文件", result.copied)
			if result.failed > 0 {
				msg = fmt.Sprintf("已归集 %d 个文件（%d 个失败）", result.copied, result.failed)
			}
			a.toastUndo(msg + " → " + filepath.Base(result.folder))
			a.PendingSelect = []string{result.folder}
			a.refresh()
		})
	})
}

func (a *appState) shareMenu(m *ui.Menu, paths []string) {
	releaseShareServices(a.ShareHold)
	a.ShareHold = nil
	if len(paths) == 0 {
		return
	}
	services, err := sharingServices(paths)
	if err != nil || len(services) == 0 {
		if m.Item("共享…").Disabled(true).Chosen() {
			a.toast("无法打开共享。")
		}
		return
	}
	a.ShareHold = services
	kept := append([]string(nil), paths...)
	m.Submenu("共享", func(sub *ui.Menu) {
		for i := range services {
			i := i
			if sub.Item(services[i].Title).Chosen() {
				a.ShareHold[i].perform(kept)
			}
		}
	})
}

func (a *appState) promptCrop(paths []string) {
	if !allVideos(paths) {
		a.toast("请先选择视频。")
		return
	}
	if len(paths) == 1 && a.openVideoEditor("crop", paths[0]) {
		return
	}
	a.CropAsk = true
	a.CropX, a.CropY = "", ""
	a.CropPaths = append([]string(nil), paths...)
	if a.CropW == "" {
		a.CropW = "1920"
	}
	if a.CropH == "" {
		a.CropH = "1080"
	}
}

func (a *appState) commitCrop() {
	width, height, err := parseCropSize(a.CropW, a.CropH)
	if err != nil {
		a.toast(err.Error())
		return
	}
	paths := append([]string(nil), a.CropPaths...)
	xText, yText := strings.TrimSpace(a.CropX), strings.TrimSpace(a.CropY)
	a.CropAsk = false
	if xText == "" && yText == "" {
		a.runCrop(paths, width, height)
		return
	}
	x, errX := strconv.Atoi(xText)
	y, errY := strconv.Atoi(yText)
	if errX != nil || errY != nil || x < 0 || y < 0 {
		a.toast("请输入有效的裁剪起点。")
		a.CropAsk = true
		return
	}
	a.runCropRect(paths, x, y, width, height)
}

func (a *appState) runCrop(paths []string, width, height int) {
	a.finishCrop(paths, func(path string) error { return cropVideoFile(path, width, height) })
}

func (a *appState) runCropRect(paths []string, x, y, width, height int) {
	a.finishCrop(paths, func(path string) error { return cropVideoRect(path, x, y, width, height) })
}

func (a *appState) finishCrop(paths []string, crop func(string) error) {
	work := a.beginWork("裁剪", len(paths))
	safeGo("actions.go:1356", func() {
		var failed []string
		for _, path := range paths {
			if err := crop(path); err != nil {
				failed = append(failed, filepath.Base(path))
			}
			a.stepWork(work)
		}
		if a.Window == nil {
			return
		}
		a.update(func() {
			a.endWork(work)
			if len(failed) == len(paths) {
				a.toast("视频裁剪失败。")
				return
			}
			if len(failed) > 0 {
				a.toast("部分视频裁剪失败：" + previewNames(failed))
			} else {
				a.toast("裁剪完成")
			}
			a.closeVideo()
			a.refresh()
		})
	})
}

func (a *appState) promptTrim() {
	if a.openVideoEditor("trim", "") {
		return
	}
	item := a.currentItem()
	if item == nil || !item.IsVideo {
		a.toast("请先打开一个视频。")
		return
	}
	a.TrimAsk = true
	a.TrimStart = "0"
	end := item.Info.DurationSec
	if end <= 0 && a.Video != nil {
		end = a.videoDuration()
	}
	if end <= 0 {
		end = 1
	}
	a.TrimEnd = strconv.FormatFloat(end, 'f', 3, 64)
}

func (a *appState) commitTrim() {
	item := a.currentItem()
	if item == nil || !item.IsVideo {
		a.TrimAsk = false
		return
	}
	start, end, err := parseTrimRange(a.TrimStart, a.TrimEnd)
	if err != nil {
		a.toast(err.Error())
		return
	}
	src := item.Path
	a.TrimAsk = false
	dest := flowUnique(filepath.Join(filepath.Dir(src), trimOutputName(src, start, end, 0)), false, nil)
	work := a.beginWork("导出片段", 0)
	safeGo("actions.go:1418", func() {
		err := trimVideoLossless(src, dest, start, end)
		if a.Window == nil {
			return
		}
		a.update(func() {
			a.endWork(work)
			if err != nil {
				a.toast("无损裁剪失败。")
				return
			}
			applied := fileOperation{op: opCopy, created: []string{dest}}
			if inverse, ok := inverseOf(applied); ok {
				a.Undo.record(applied, inverse)
			}
			a.toastUndo("已导出 " + filepath.Base(dest))
			a.KeepLarge = true
			a.PendingLarge = src
			a.refresh()
		})
	})
}

func (a *appState) cropDialog(c *ui.Context) {
	if !a.CropAsk {
		return
	}
	ui.Modal(c, &a.CropAsk, func() {
		ui.Text(c, "裁剪视频尺寸").Bold().FontSize(14)
		ui.Text(c, "替换原文件。留空起点则居中，否则按左上角像素裁剪。").FontSize(12).TextColor(c.Theme().TextMuted)
		ui.Row(c).Gap(8).Children(func() {
			ui.TextInput(c, &a.CropW).Label("宽").Width(120)
			ui.TextInput(c, &a.CropH).Label("高").Width(120)
		})
		ui.Row(c).Gap(8).Children(func() {
			ui.TextInput(c, &a.CropX).Label("X").Width(120)
			ui.TextInput(c, &a.CropY).Label("Y").Width(120)
		})
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c.Key("crop-cancel"), "取消").Clicked() {
				a.CropAsk = false
			}
			if ui.Button(c.Key("crop-ok"), "裁剪并替换").Clicked() {
				a.commitCrop()
			}
		})
	})
}

func (a *appState) trimRangeSliders(c *ui.Context) {
	item := a.currentItem()
	if item == nil || !item.IsVideo {
		return
	}
	duration := item.Info.DurationSec
	if duration <= 0 {
		duration = a.videoDuration()
	}
	if duration <= 0 {
		return
	}
	start, end, err := parseTrimRange(a.TrimStart, a.TrimEnd)
	if err != nil {
		start, end = 0, duration
	}
	if end > duration {
		end = duration
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "入点").FontSize(11)
		if ui.Slider(c, &start, 0, duration).Width(200).Changed() {
			if start > end-0.05 {
				start = max(0, end-0.05)
			}
			a.TrimStart = strconv.FormatFloat(start, 'f', 3, 64)
			a.seekVideo(start)
		}
	})
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "出点").FontSize(11)
		if ui.Slider(c, &end, 0, duration).Width(200).Changed() {
			if end < start+0.05 {
				end = min(duration, start+0.05)
			}
			a.TrimEnd = strconv.FormatFloat(end, 'f', 3, 64)
			a.seekVideo(end)
		}
	})
}

func (a *appState) trimDialog(c *ui.Context) {
	if !a.TrimAsk {
		return
	}
	ui.Modal(c, &a.TrimAsk, func() {
		ui.Text(c, "无损裁剪片段").Bold().FontSize(14)
		ui.Text(c, "拖动范围会跳到对应画面，导出不重新编码，原视频保持不变。").FontSize(12).TextColor(c.Theme().TextMuted)
		a.trimRangeSliders(c)
		ui.Row(c).Gap(8).Children(func() {
			ui.TextInput(c, &a.TrimStart).Label("开始").Width(120)
			ui.TextInput(c, &a.TrimEnd).Label("结束").Width(120)
		})
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c.Key("trim-cancel"), "取消").Clicked() {
				a.TrimAsk = false
			}
			if ui.Button(c.Key("trim-ok"), "导出").Clicked() {
				a.commitTrim()
			}
		})
	})
}

func tagDots(tags []string) []string {
	var out []string
	for _, tag := range tags {
		if isRatingTag(tag) {
			continue
		}
		out = append(out, tag)
		if len(out) == 6 {
			break
		}
	}
	return out
}

func (a *appState) tagsOf(path string) []string {
	for i := range a.Items {
		if a.Items[i].Path == path {
			return a.Items[i].Tags
		}
	}
	return finderTags(path)
}

func (a *appState) sharedTags(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	counts := map[string]int{}
	for _, path := range paths {
		seen := map[string]bool{}
		for _, tag := range a.tagsOf(path) {
			if seen[tag] {
				continue
			}
			seen[tag] = true
			counts[tag]++
		}
	}
	var out []string
	for tag, n := range counts {
		if n == len(paths) {
			out = append(out, tag)
		}
	}
	return out
}

func (a *appState) rememberTags(paths []string) {
	refreshItemTags(a.Items, paths)
	a.applyFilterLocked()
}

func (a *appState) toggleTag(paths []string, name string) {
	added, err := toggleFinderTag(paths, name)
	if err != nil {
		a.toast("无法修改标签。")
		return
	}
	a.rememberTags(paths)
	if added {
		a.toast("已添加 " + name)
		return
	}
	a.toast("已移除 " + name)
}

func (a *appState) clearTags(paths []string) {
	if err := clearFinderTags(paths); err != nil {
		a.toast("无法移除标签。")
		return
	}
	a.rememberTags(paths)
	a.toast("已移除所有标签")
}

func (a *appState) tagMenu(m *ui.Menu, paths []string) {
	if len(paths) == 0 || !tagsSupported {
		return
	}
	active := a.sharedTags(paths)
	kept := append([]string(nil), paths...)
	m.Submenu(tagsLabel, func(sub *ui.Menu) {
		for _, name := range finderLabelNames() {
			name := name
			if sub.Item(name).Checked(containsString(active, name)).Chosen() {
				a.toggleTag(kept, name)
			}
		}
		sub.Separator()
		if sub.Item("移除所有标签").Chosen() {
			a.clearTags(kept)
		}
		if allImages(kept) {
			sub.Separator()
			a.ratingMenu(sub, kept)
		}
	})
}

func (a *appState) ratingMenu(m *ui.Menu, paths []string) {
	if !allImages(paths) {
		return
	}
	kept := append([]string(nil), paths...)
	m.Submenu("评级", func(sub *ui.Menu) {
		for rating := 5; rating >= 1; rating-- {
			rating := rating
			label := strings.Repeat("★", rating)
			if sub.Item(label).Chosen() {
				a.setRating(kept, rating)
			}
		}
		if sub.Item("无评级").Chosen() {
			a.setRating(kept, 0)
		}
	})
}

func (a *appState) setRating(paths []string, rating int) {
	if err := writeRating(paths, rating); err != nil {
		a.toast("无法写入评级。")
		return
	}
	a.rememberTags(paths)
	if rating == 0 {
		a.toast("已清除评级")
		return
	}
	a.toast("评级 " + strings.Repeat("★", rating))
}

func (a *appState) openWithMenu(m *ui.Menu, paths []string) {
	if len(paths) == 0 || allDirectories(paths) {
		return
	}
	apps := openerApps(paths[0])
	if len(apps) == 0 {
		return
	}
	kept := append([]string(nil), paths...)
	m.Submenu("打开方式", func(sub *ui.Menu) {
		for _, app := range apps {
			app := app
			if sub.Item(app.Name).Chosen() {
				if err := openWithApp(app.Path, kept); err != nil {
					a.toast("无法用 " + app.Name + " 打开。")
				}
			}
		}
	})
}

func (a *appState) showInfo() {
	text := describeSelection(a.operationPaths())
	if text == "" {
		return
	}
	a.InfoAsk = true
	a.InfoText = text
}

func (a *appState) infoDialog(c *ui.Context) {
	if !a.InfoAsk {
		return
	}
	ui.Modal(c, &a.InfoAsk, func() {
		title := "文件信息"
		if strings.HasPrefix(a.InfoText, "共 ") {
			title = "统计"
		}
		ui.Text(c, title).Bold().FontSize(14)
		ui.Text(c, a.InfoText).FontSize(12)
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c.Key("info-copy"), "复制").Clicked() {
				mygo.Clipboard.WriteText(a.InfoText)
				a.toast("已复制简介")
			}
			if ui.Button(c.Key("info-close"), "关闭").Clicked() {
				a.InfoAsk = false
			}
		})
	})
}

func (a *appState) duplicateSelection() {
	paths := a.operationPaths()
	if len(paths) == 0 {
		return
	}
	created, err := duplicateBeside(paths)
	if err != nil {
		a.toast(err.Error())
		return
	}
	applied := fileOperation{op: opCopy, created: created}
	if inverse, ok := inverseOf(applied); ok {
		a.Undo.record(applied, inverse)
	}
	a.PendingSelect = append([]string(nil), created...)
	a.toastUndo(fmt.Sprintf("已制作 %d 个副本", len(created)))
	a.refresh()
}

func (a *appState) showOriginalFolder() {
	paths := a.operationPaths()
	if len(paths) != 1 {
		return
	}
	parent := filepath.Dir(paths[0])
	if parent == "" || parent == paths[0] {
		return
	}
	a.PendingSelect = []string{paths[0]}
	a.openFolder(parent)
}
