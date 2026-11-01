package main

import (
	"errors"
	"path/filepath"
	"strings"
)

// undoEntry records an applied operation and its direct inverse.
type undoEntry struct {
	forward fileOperation
	inverse fileOperation
}

type undoStack struct {
	past   []undoEntry
	future []undoEntry
}

func (s *undoStack) record(op, inverse fileOperation) {
	s.past = append(s.past, undoEntry{forward: op, inverse: inverse})
	s.future = s.future[:0]
}

func (s *undoStack) undo() (undoEntry, bool) {
	if len(s.past) == 0 {
		return undoEntry{}, false
	}
	i := len(s.past) - 1
	e := s.past[i]
	s.past = s.past[:i]
	return e, true
}

func (s *undoStack) redo() (undoEntry, bool) {
	if len(s.future) == 0 {
		return undoEntry{}, false
	}
	i := len(s.future) - 1
	e := s.future[i]
	s.future = s.future[:i]
	return e, true
}

func inverseOf(op fileOperation) (fileOperation, bool) {
	if op.replaced || op.permanent {
		return fileOperation{}, false
	}
	switch op.op {
	case opCopy:
		if len(op.created) == 0 {
			return fileOperation{}, false
		}
		return fileOperation{op: opDelete, paths: append([]string(nil), op.created...)}, true
	case opMove:
		swapped := swappedMappings(op.mappings)
		if len(swapped) == 0 {
			return fileOperation{}, false
		}
		return fileOperation{op: opMove, mappings: swapped}, true
	case opRename:
		swapped := swappedMappings(op.mappings)
		if len(swapped) == 0 {
			return fileOperation{}, false
		}
		return fileOperation{op: opRename, mappings: swapped}, true
	case opCompress, opExtract:
		if op.dropSource || len(op.created) == 0 {
			return fileOperation{}, false
		}
		return fileOperation{op: opDelete, paths: append([]string(nil), op.created...)}, true
	case opDelete:
		if op.permanent || len(op.mappings) == 0 {
			return fileOperation{}, false
		}
		swapped := swappedMappings(op.mappings)
		if len(swapped) == 0 {
			return fileOperation{}, false
		}
		return fileOperation{op: opMove, mappings: swapped}, true
	default:
		return fileOperation{}, false
	}
}

var errOperationBusy = errors.New("busy")

func (a *appState) performTracked(op fileOperation) error {
	if a.busy() {
		return errOperationBusy
	}
	applied, err := perform(op)
	if err != nil {
		logErrorf("file operation %d failed: paths=%d dest=%s err=%v",
			int(op.op), len(op.paths), op.dest, err)
		a.toast(err.Error())
		return err
	}
	if inverse, ok := inverseOf(applied); ok {
		a.Undo.record(applied, inverse)
	}
	a.finishOperation(applied)
	return nil
}

func (a *appState) startArchive(op fileOperation) {
	if len(op.paths) == 0 {
		return
	}
	if a.Window == nil {
		applied, err := perform(op)
		if err != nil {
			if errors.Is(err, errNeedZipPassword) && op.op == opExtract {
				a.promptZip("extract", op.paths, op.dropSource)
				return
			}
			a.toast(err.Error())
			return
		}
		if inverse, ok := inverseOf(applied); ok {
			a.Undo.record(applied, inverse)
		}
		a.finishOperation(applied)
		return
	}
	safeGo("undo.go:119", func() {
		applied, err := perform(op)
		if a.Window == nil {
			return
		}
		a.update(func() {
			if err != nil && errors.Is(err, errNeedZipPassword) && op.op == opExtract {
				a.promptZip("extract", op.paths, op.dropSource)
				return
			}
			if err != nil {
				a.toast(err.Error())
				return
			}
			if inverse, ok := inverseOf(applied); ok {
				a.Undo.record(applied, inverse)
			}
			a.finishOperation(applied)
		})
	})
}

func (a *appState) runOperation(op fileOperation) {
	if a.busy() {
		return
	}
	if a.headless() {
		_ = a.performTracked(op)
		return
	}
	a.opBusy = true
	safeGo("undo.go:142", func() {
		applied, err := perform(op)
		a.update(func() {
			a.opBusy = false
			if err != nil {
				a.toast(err.Error())
				return
			}
			if inverse, ok := inverseOf(applied); ok {
				a.Undo.record(applied, inverse)
			}
			a.finishOperation(applied)
		})
	})
}

// undo and redo do their file work in the background, one operation at a
// time: the entry is taken off its stack at once, so a second press while
// one runs is ignored instead of racing it, and the stacks and selection are
// updated on the main thread when the work is done.
func (a *appState) undo() {
	if a.busy() {
		return
	}
	e, ok := a.Undo.undo()
	if !ok {
		return
	}
	if a.headless() {
		applied, err := perform(e.inverse)
		if err != nil {
			a.Undo.past = append(a.Undo.past, e)
			return
		}
		a.Undo.future = append(a.Undo.future, undoEntry{forward: e.forward, inverse: applied})
		a.finishOperation(applied)
		return
	}
	a.opBusy = true
	safeGo("undo", func() {
		applied, err := perform(e.inverse)
		a.updateOrDrop(func() {
			a.opBusy = false
			if err != nil {
				a.Undo.past = append(a.Undo.past, e)
				a.toast(err.Error())
				return
			}
			a.Undo.future = append(a.Undo.future, undoEntry{forward: e.forward, inverse: applied})
			a.finishOperation(applied)
		})
	})
}

func (a *appState) redo() {
	if a.busy() {
		return
	}
	e, ok := a.Undo.redo()
	if !ok {
		return
	}
	if a.headless() {
		applied, err := perform(e.forward)
		if err != nil {
			a.Undo.future = append(a.Undo.future, e)
			return
		}
		a.Undo.past = append(a.Undo.past, undoEntry{forward: applied, inverse: e.inverse})
		a.finishOperation(applied)
		return
	}
	a.opBusy = true
	safeGo("redo", func() {
		applied, err := perform(e.forward)
		a.updateOrDrop(func() {
			a.opBusy = false
			if err != nil {
				a.Undo.future = append(a.Undo.future, e)
				a.toast(err.Error())
				return
			}
			a.Undo.past = append(a.Undo.past, undoEntry{forward: applied, inverse: e.inverse})
			a.finishOperation(applied)
		})
	})
}

// updateOrDrop runs fn on the main thread through update.
func (a *appState) updateOrDrop(fn func()) { a.update(fn) }

// headless reports a run with no window and no test hook, where background
// results could never be applied: operations then run inline as before.
func (a *appState) headless() bool {
	return a.Window == nil && headlessUpdate.Load() == nil
}

// busy refuses a file operation while another one runs in the background,
// so operations apply in the order they were asked for.
func (a *appState) busy() bool {
	if a.opBusy {
		a.toast("正在处理上一个操作…")
		return true
	}
	return false
}

func (a *appState) finishOperation(op fileOperation) {
	// Each operation owns the pending selection. A leftover from a prior
	// operation whose scan got superseded (refresh bumps scanSeq) would
	// otherwise be applied by this operation's scan and hijack the
	// selection — e.g. redo's target landing after a later delete.
	a.PendingSelect = nil
	if op.cut {
		a.Cut = nil
		a.Clip = nil
	}
	switch op.op {
	case opRename:
		a.retargetPaths(op.mappings)
		a.PendingSelect = mappingTargets(op.mappings)
		a.noteLargeTarget(op.mappings)
	case opMove:
		a.retargetPaths(op.mappings)
		a.PendingSelect = mappingTargets(op.mappings)
		a.noteLargeTarget(op.mappings)
	case opCopy:
		a.PendingSelect = append([]string(nil), op.created...)
	case opDelete:
		a.noteDeleted(op.paths)
	case opCompress, opExtract:
		if op.detail != "" {
			a.toast(op.detail)
		}
		if op.dropSource {
			a.noteDeleted(op.paths)
		}
		if op.op == opExtract && len(op.created) == 1 && !a.Recursive && isDir(op.created[0]) {
			a.openFolder(op.created[0])
			return
		}
	}
	a.refresh()
}

func mappingTargets(mappings []pathPair) []string {
	out := make([]string, 0, len(mappings))
	for _, mapping := range mappings {
		if mapping.From != mapping.To {
			out = append(out, mapping.To)
		}
	}
	return out
}

func (a *appState) noteLargeTarget(mappings []pathPair) {
	if !a.InLargeView {
		return
	}
	item := a.currentItem()
	if item == nil {
		return
	}
	a.KeepLarge = true
	a.PendingLarge = remapPath(item.Path, mappings)
}

func (a *appState) noteDeleted(paths []string) {
	gone := map[string]struct{}{}
	for _, path := range paths {
		gone[path] = struct{}{}
	}
	if a.InLargeView {
		item := a.currentItem()
		if item != nil {
			if _, ok := gone[item.Path]; ok {
				a.KeepLarge = true
				a.PendingLarge = ""
				if next := a.visibleItem(a.LargeIndex + 1); next != nil {
					nextPath := next.Path
					if _, drop := gone[nextPath]; !drop {
						a.PendingLarge = nextPath
					}
				}
				if a.PendingLarge == "" && a.LargeIndex > 0 {
					if prev := a.visibleItem(a.LargeIndex - 1); prev != nil {
						prevPath := prev.Path
						if _, drop := gone[prevPath]; !drop {
							a.PendingLarge = prevPath
						}
					}
				}
			} else {
				a.KeepLarge = true
				a.PendingLarge = item.Path
			}
		}
	}
	if a.Video != nil {
		if _, ok := gone[a.Video.path]; ok {
			a.closeVideo()
		}
	}
}

func (a *appState) retargetPaths(mappings []pathPair) {
	if len(mappings) == 0 {
		return
	}
	// Background decodes read item paths under a.mu (thumbAbandoned), so the
	// rewrites here hold it too. Window work happens after the lock.
	retitled := ""
	a.mu.Lock()
	if a.Root != "" {
		next := remapPath(a.Root, mappings)
		if next != a.Root && isDir(next) {
			a.Root = next
			retitled = next
		}
	}
	for i := range a.Items {
		next := remapPath(a.Items[i].Path, mappings)
		if next == a.Items[i].Path {
			continue
		}
		a.Items[i].Path = next
		a.Items[i].Name = filepath.Base(next)
		a.Items[i].Ext = extOf(next)
	}
	var selected []string
	for path := range a.Selected.All() {
		selected = append(selected, remapPath(path, mappings))
	}
	a.Selected.Clear()
	for _, path := range selected {
		a.Selected.Add(path)
	}
	if a.Video != nil {
		next := remapPath(a.Video.path, mappings)
		a.Video.path = next
		if a.Video.dec != nil {
			a.Video.dec.path = next
		}
	}
	if len(a.Cut) > 0 {
		next := map[string]bool{}
		for path := range a.Cut {
			next[remapPath(path, mappings)] = true
		}
		a.Cut = next
	}
	for i, path := range a.Clip {
		a.Clip[i] = remapPath(path, mappings)
	}
	a.mu.Unlock()
	if retitled != "" {
		a.setTitle(appName + " — " + pathName(retitled))
	}
}

func extOf(path string) string {
	ext := filepath.Ext(path)
	if len(ext) > 0 {
		ext = ext[1:]
	}
	return strings.ToLower(ext)
}
