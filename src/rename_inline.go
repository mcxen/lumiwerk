package main

import (
	"errors"
	"github.com/egoist/mygo"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// renameSlowClick is how long after a click on the name of the item
// already chosen the name turns into a field, as Finder's slow second click:
// longer than a double click.
const renameSlowClick = 600 * time.Millisecond

// inlineRenameIndex is the visible index the rename of path edits in
// place, or -1 where the layout shows no name to edit (large view, 无字,
// waterfall/justified): those keep the dialog.
func (a *appState) inlineRenameIndex(path string) int {
	if a.InLargeView || a.galleryActive() || a.Root == "" {
		return -1
	}
	switch {
	case a.LayoutMode == 1:
	case a.LayoutMode == 0 && flowLabel > 0:
	default:
		return -1
	}
	if i := a.Chosen; i >= 0 {
		if it := a.visibleItem(i); it != nil && it.Path == path {
			return i
		}
	}
	for i := range a.Visible {
		if it := a.visibleItem(i); it != nil && it.Path == path {
			return i
		}
	}
	return -1
}

// renamingInline reports whether item i shows the inline name field.
func (a *appState) renamingInline(i int) bool {
	if !a.Renaming || !a.RenameInline {
		return false
	}
	it := a.visibleItem(i)
	return it != nil && it.Path == a.RenamePath
}

// inlineRenameField edits a.RenameVal in place of an item's name: the
// name before its extension selected, Enter keeps it, Escape goes back,
// and moving the focus away keeps it, as Finder (and mygo's EditableText).
func (a *appState) inlineRenameField(c *ui.Context, size float32, center bool) {
	t := c.Theme()
	in := ui.TextInput(c.Key("inline-rename"), &a.RenameVal).AutoFocus().FontSize(size).
		MinWidth(40).Padding(0, 3).Radius(3).Border(1, t.Accent).Background(t.Background)
	if center {
		// Under a grid thumbnail: as wide as the card, one line high.
		in.TextAlign(ui.Center).FillWidth().Shrink(0)
	} else {
		in.Grow(1)
	}
	switch {
	case a.renameFresh > 0:
		// The field takes the focus as it appears; select once it has it.
		if in.Focused() || a.renameFresh == 1 {
			in.SetTextSelection(0, baseNameEnd(a.RenameVal))
			a.renameFresh = 0
		} else {
			a.renameFresh--
		}
		c.Invalidate()
	case in.Submitted():
		a.commitInlineRename()
	case in.Shortcut(0, ui.KeyEscape):
		a.cancelRename()
	case !in.Focused():
		if a.commitInlineRename() != nil {
			a.cancelRename()
		}
	}
}

// baseNameEnd is the rune offset where name's extension starts, its end
// without one; a leading dot is not an extension.
func baseNameEnd(name string) int {
	if dot := strings.LastIndexByte(name, '.'); dot > 0 {
		return len([]rune(name[:dot]))
	}
	return len([]rune(name))
}

// checkRename validates a new name for path: not empty, no characters a
// file name cannot hold, and no other item of that name beside it.
func checkRename(path, name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("名称不能为空")
	}
	if err := validBaseName(name); err != nil {
		return err
	}
	if strings.ContainsAny(name, ":") {
		return errors.New("名称不能包含 :")
	}
	if name == filepath.Base(path) {
		return nil
	}
	if _, err := os.Lstat(filepath.Join(filepath.Dir(path), name)); err == nil &&
		!strings.EqualFold(name, filepath.Base(path)) {
		return errors.New("已存在同名项目：" + name)
	}
	return nil
}

// commitInlineRename validates and renames through the tracked path (undo
// and the 撤销 toast); an invalid name keeps the field open with a toast.
func (a *appState) commitInlineRename() error {
	if err := checkRename(a.RenamePath, a.RenameVal); err != nil {
		a.toast(err.Error())
		a.renameFresh = 2
		return err
	}
	a.commitRename()
	if a.Renaming {
		return errors.New("rename failed")
	}
	a.RenameInline = false
	return nil
}

func (a *appState) cancelRename() {
	a.Renaming, a.RenameInline, a.RenamePath, a.renameFresh = false, false, "", 0
}

// armSlowRename notes a click on item i's name: when i was already the
// only item chosen, a rename starts once a double click is no longer
// possible.
func (a *appState) armSlowRename(c *ui.Context, i int, wasChosen bool) {
	it := a.visibleItem(i)
	if it == nil || !wasChosen || a.Selected.Len() > 1 || a.Renaming {
		a.renameArm = ""
		return
	}
	a.renameArm, a.renameArmAt = it.Path, time.Now().Add(renameSlowClick)
	c.Invalidate()
}

// slowRename starts the rename armed by a slow second click, unless the
// choice moved or a double click opened the item in the meantime.
func (a *appState) slowRename(c *ui.Context) {
	if a.renameArm == "" {
		return
	}
	if time.Now().Before(a.renameArmAt) {
		c.Invalidate()
		return
	}
	path := a.renameArm
	a.renameArm = ""
	if it := a.visibleItem(a.Chosen); it != nil && it.Path == path && !a.InLargeView && !a.Renaming {
		a.startRename()
	}
}

// pointerOver reports whether the pointer over card is on its name; asked
// only of the chosen item, so other cards take no frame as it moves.
func pointerOver(card, name ui.Element) bool {
	x, y, over := card.PointerPosition()
	if !over {
		return false
	}
	cb, nb := card.LayoutBounds(), name.LayoutBounds()
	x, y = x+cb.X, y+cb.Y
	return x >= nb.X && x < nb.X+nb.W && y >= nb.Y-2 && y < nb.Y+nb.H+2
}

// menuKey is b for a menu item's Shortcut, none while a name is typed in
// place for a key without Cmd, Ctrl or Alt: the menu would take the letter.
func (a *appState) menuKey(b keyBinding) (ui.Modifiers, ui.Key) {
	if a.Renaming && a.RenameInline && b.Mods&^ui.Shift == 0 {
		return 0, 0
	}
	return b.Mods, b.Key
}

// syncMenuKeys turns off the app menu's items whose accelerator is a bare
// key (T, 5…) while a name is typed in place, as the menu would take
// those letters before the field, and back on after.
func (a *appState) syncMenuKeys() {
	typing := a.Renaming && a.RenameInline
	switch {
	case typing && a.menuKeysOff == nil && a.appMenu != nil:
		a.menuKeysOff = []*mygo.MenuItem{}
		var walk func([]*mygo.MenuItem)
		walk = func(items []*mygo.MenuItem) {
			for _, it := range items {
				if it == nil {
					continue
				}
				if acc := it.Accelerator; acc != "" && !strings.Contains(acc, "+") && it.IsEnabled() {
					it.SetEnabled(false)
					a.menuKeysOff = append(a.menuKeysOff, it)
				}
				walk(it.Submenu)
			}
		}
		walk(a.appMenu.Items())
	case !typing && a.menuKeysOff != nil:
		for _, it := range a.menuKeysOff {
			it.SetEnabled(true)
		}
		a.menuKeysOff = nil
	}
}
