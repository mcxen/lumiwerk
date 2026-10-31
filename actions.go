package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	return []string{a.Items[a.Visible[a.Chosen]].Path}
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
	go func() {
		dest, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.Window, Title: title, Directory: true})
		if err != nil || len(dest) == 0 {
			return
		}
		a.runOperation(fileOperation{op: kind, paths: paths, dest: dest[0]})
	}()
}

func (a *appState) doTrash() {
	paths := a.selectedPaths()
	if a.InLargeView {
		if item := a.currentItem(); item != nil {
			paths = []string{item.Path}
		}
	}
	if len(paths) == 0 {
		return
	}
	_ = a.performTracked(fileOperation{op: opDelete, paths: paths})
}

func (a *appState) startRename() {
	paths := a.selectedPaths()
	if len(paths) == 0 {
		return
	}
	a.Renaming = true
	a.RenamePath = paths[0]
	a.RenameVal = filepath.Base(paths[0])
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
	if index >= 0 && index < len(a.Visible) {
		a.Chosen = index
		if !a.Selected.Has(a.Items[a.Visible[index]].Path) {
			a.Selected.Clear()
			a.Selected.Add(a.Items[a.Visible[index]].Path)
		}
	}
	a.fileMenu(m, true)
}

func (a *appState) fileContextMenu(m *ui.Menu) { a.fileMenu(m, false) }

func (a *appState) largeContextMenu(m *ui.Menu, item *mediaItem) {
	_ = item
	if m.Item("适应窗口").Chosen() {
		a.resetZoom()
	}
	if m.Item("实际大小").Chosen() {
		a.zoom100()
	}
	m.Separator()
	a.fileMenu(m, false)
}

func (a *appState) fileMenu(m *ui.Menu, canOpen bool) {
	paths := a.selectedPaths()
	none := len(paths) == 0
	if canOpen && m.Item("打开").Disabled(none).Chosen() {
		a.openLargeImage(a.Chosen)
	}
	if m.Item("在 Finder 中显示").Disabled(none).Chosen() {
		a.revealInFinder()
	}
	if m.Item("复制路径").Disabled(none).Chosen() {
		a.copyPath()
	}
	m.Separator()
	if m.Item("复制").Disabled(none).Chosen() {
		a.doCopy()
	}
	if m.Item("剪切").Disabled(none).Chosen() {
		a.doCut()
	}
	if m.Item("粘贴").Disabled(a.Root == "" || !a.canPaste()).Chosen() {
		a.doPaste(false)
	}
	if m.Item("移动到此").Disabled(a.Root == "" || !a.canPaste()).Chosen() {
		a.doPaste(true)
	}
	m.Separator()
	if m.Item("重命名…").Disabled(none).Chosen() {
		a.startRename()
	}
	if m.Item("批量重命名…").Disabled(len(paths) < 2).Chosen() {
		a.startBatchRename()
	}
	if m.Item("快速重命名当前目录").Disabled(a.Root == "").Chosen() {
		a.quickRenameFolder()
	}
	if m.Item("复制到…").Disabled(none).Chosen() {
		a.doCopyTo()
	}
	if m.Item("移动到…").Disabled(none).Chosen() {
		a.doMove()
	}
	if m.Item("复制到配置目录").Disabled(none).Chosen() {
		go a.copyToConfigured()
	}
	m.Separator()
	if m.Item("新建文件夹").Disabled(a.Root == "").Chosen() {
		a.newFolder()
	}
	if m.Item("用所选项目新建文件夹").Disabled(none).Chosen() {
		a.newFolderWithSelection()
	}
	if m.Item("移到废纸篓").Disabled(none).Chosen() {
		a.doTrash()
	}
}

// renameDialog shows a real editable field and commits on Enter.
func (a *appState) renameDialog(c *ui.Context) {
	if !a.Renaming {
		return
	}
	t := c.Theme()
	panel := ui.Column(c).Attach(ui.AnchorCenter, ui.AnchorCenter).Width(320).Padding(16).Gap(8).
		Background(t.Surface).Border(1, t.Border).Radius(8)
	panel.Children(func() {
		ui.Text(c, "重命名").Bold().FontSize(14)
		field := ui.TextInput(c, &a.RenameVal).Width(280)
		if field.Submitted() {
			a.commitRename()
		}
		row := ui.Row(c).Gap(8).Justify(ui.End)
		row.Children(func() {
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
		item.Info = loadMediaInfo(item.Path, item.IsVideo)
		item.InfoLoaded = true
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
	a.Batch.Rule = renameRule{Mode: "format", Pattern: "{name}_{num:3}", StartNumber: 1}
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
	t := c.Theme()
	panel := ui.Column(c).Attach(ui.AnchorCenter, ui.AnchorCenter).Width(520).Padding(16).Gap(10).
		Background(t.Surface).Border(1, t.Border).Radius(8)
	panel.Children(func() {
		ui.Text(c, "批量重命名").Bold().FontSize(16)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "模式").FontSize(12)
			mode := 0
			switch a.Batch.Rule.Mode {
			case "replace":
				mode = 1
			case "add":
				mode = 2
			}
			ui.Segmented(c, &mode, "模板", "替换", "添加").Changed()
			switch mode {
			case 0:
				a.Batch.Rule.Mode = "format"
			case 1:
				a.Batch.Rule.Mode = "replace"
			case 2:
				a.Batch.Rule.Mode = "add"
			}
		})
		if a.Batch.Rule.Mode == "format" {
			if ui.TextInput(c, &a.Batch.Rule.Pattern).Label("模板").Width(480).Changed() {
				a.updateBatchPreview()
			}
			ui.Text(c, "可用: {num:3} {date} {time} {name} {ext}").FontSize(11).TextColor(t.TextMuted)
		}
		if a.Batch.Rule.Mode == "replace" {
			if ui.TextInput(c, &a.Batch.Rule.Find).Label("查找").Width(230).Changed() {
				a.updateBatchPreview()
			}
			if ui.TextInput(c, &a.Batch.Rule.Replace).Label("替换").Width(230).Changed() {
				a.updateBatchPreview()
			}
		}
		if a.Batch.Rule.Mode == "add" {
			if ui.TextInput(c, &a.Batch.Rule.Pattern).Label("文本").Width(480).Changed() {
				a.updateBatchPreview()
			}
			pos := 0
			if a.Batch.Rule.AddPosition == "suffix" {
				pos = 1
			}
			ui.Segmented(c, &pos, "前缀", "后缀").Changed()
			if pos == 0 {
				a.Batch.Rule.AddPosition = "prefix"
			} else {
				a.Batch.Rule.AddPosition = "suffix"
			}
		}
		ui.Divider(c)
		ui.Text(c, "预览").Bold().FontSize(12)
		for _, src := range a.selectedPaths() {
			dst := a.Batch.Preview[src]
			if dst == "" {
				continue
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, filepath.Base(src)).FontSize(11).TextColor(t.TextMuted).MaxLines(1)
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
	a.Window.Update(func() {
		a.Profiles.Profiles[a.Profiles.Active].Destination = dest[0]
		savePrefs(a.Profiles)
	})
}

func (a *appState) copyToConfigured() {
	dest := a.currentDestination()
	if dest == "" || !isDir(dest) {
		return
	}
	paths := a.selectedPaths()
	if len(paths) == 0 {
		return
	}
	a.runOperation(fileOperation{op: opCopy, paths: paths, dest: dest})
}

func (a *appState) toast(msg string) {
	if msg == "" {
		return
	}
	a.HUD = msg
	a.HUDUntil = time.Now().Add(1600 * time.Millisecond)
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
	if a.Root == "" {
		return
	}
	paths, cut := a.filesForPaste(forceMove)
	if len(paths) == 0 {
		a.toast("剪贴板里没有文件")
		return
	}
	op := fileOperation{op: opCopy, paths: paths, dest: a.Root}
	if cut {
		op.op = opMove
		op.cut = true
	}
	a.runOperation(op)
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
	_ = exec.Command("open", "-R", paths[0]).Start()
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

func (a *appState) isCut(path string) bool {
	return a.Cut != nil && a.Cut[path]
}

func (a *appState) toastOverlay(c *ui.Context) {
	if a.HUD == "" || a.InLargeView {
		return
	}
	if time.Now().After(a.HUDUntil) {
		a.HUD = ""
		return
	}
	if d := time.Until(a.HUDUntil); d > 0 {
		c.After(d)
	}
	t := c.Theme()
	hud := ui.Row(c).Attach(ui.AnchorTop, ui.AnchorTop).Margin(8, 0).Padding(6, 12).Radius(6).Background(t.Surface).Border(1, t.Border)
	hud.Children(func() {
		ui.Text(c, a.HUD).FontSize(12).MaxLines(1)
	})
}

func (a *appState) folderDialog(c *ui.Context) {
	if !a.NewFolder {
		return
	}
	t := c.Theme()
	panel := ui.Column(c).Attach(ui.AnchorCenter, ui.AnchorCenter).Width(320).Padding(16).Gap(8).
		Background(t.Surface).Border(1, t.Border).Radius(8)
	panel.Children(func() {
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
}
