package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
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

func (a *appState) doCopy() {
	paths := a.selectedPaths()
	if len(paths) == 0 {
		return
	}
	go func() {
		dest, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.Window, Title: "复制到目录", Directory: true})
		if err != nil || len(dest) == 0 {
			return
		}
		if err := a.performTracked(fileOperation{op: opCopy, paths: paths, dest: dest[0]}); err != nil {
			return
		}
		a.Window.Update(func() { a.refresh() })
	}()
}

func (a *appState) doMove() {
	paths := a.selectedPaths()
	if len(paths) == 0 {
		return
	}
	go func() {
		dest, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.Window, Title: "移动到目录", Directory: true})
		if err != nil || len(dest) == 0 {
			return
		}
		if err := a.performTracked(fileOperation{op: opMove, paths: paths, dest: dest[0]}); err != nil {
			return
		}
		a.Window.Update(func() { a.refresh() })
	}()
}

func (a *appState) doTrash() {
	paths := a.selectedPaths()
	if len(paths) == 0 {
		return
	}
	if err := a.performTracked(fileOperation{op: opDelete, paths: paths}); err != nil {
		return
	}
	a.refresh()
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
		a.Renaming = false
		return
	}
	a.Renaming = false
	a.RenamePath = ""
	a.refresh()
}

func (a *appState) newFolder() {
	if a.Root == "" {
		return
	}
	name := time.Now().Format("新建文件夹 2006-01-02 15-04-05")
	if err := os.Mkdir(filepath.Join(a.Root, name), 0o755); err != nil {
		return
	}
	a.refresh()
}

func (a *appState) mediaContextMenu(m *ui.Menu, index int) {
	if index >= 0 && index < len(a.Visible) {
		a.Chosen = index
	}
	paths := a.selectedPaths()
	none := len(paths) == 0
	if m.Item("打开").Disabled(none).Chosen() {
		a.openLargeImage(a.Chosen)
	}
	m.Separator()
	if m.Item("重命名…").Disabled(none).Chosen() {
		a.startRename()
	}
	if m.Item("批量重命名…").Disabled(none).Chosen() {
		a.startBatchRename()
	}
	if m.Item("复制到…").Disabled(none).Chosen() {
		a.doCopy()
	}
	if m.Item("移动到…").Disabled(none).Chosen() {
		a.doMove()
	}
	if m.Item("移到废纸篓").Disabled(none).Chosen() {
		a.doTrash()
	}
	m.Separator()
	if m.Item("新建文件夹").Disabled(a.Root == "").Chosen() {
		a.newFolder()
	}
}

func (a *appState) fileContextMenu(m *ui.Menu) {
	paths := a.selectedPaths()
	none := len(paths) == 0
	if m.Item("重命名…").Disabled(none).Chosen() {
		a.startRename()
	}
	if m.Item("批量重命名…").Disabled(none).Chosen() {
		a.startBatchRename()
	}
	if m.Item("复制到…").Disabled(none).Chosen() {
		a.doCopy()
	}
	if m.Item("移动到…").Disabled(none).Chosen() {
		a.doMove()
	}
	if m.Item("移到废纸篓").Disabled(none).Chosen() {
		a.doTrash()
	}
	m.Separator()
	if m.Item("新建文件夹").Disabled(a.Root == "").Chosen() {
		a.newFolder()
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
	if err := applyRenames(a.Batch.Preview); err != nil {
		a.Batch.Open = false
		return
	}
	a.Batch.Open = false
	a.Batch.Preview = nil
	a.Selected.Clear()
	a.refresh()
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
	if err := a.performTracked(fileOperation{op: opCopy, paths: paths, dest: dest}); err != nil {
		return
	}
	a.Window.Update(func() { a.refresh() })
}
