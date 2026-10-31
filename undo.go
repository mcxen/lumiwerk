package main

import (
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
	default:
		return fileOperation{}, false
	}
}

func (a *appState) performTracked(op fileOperation) error {
	applied, err := perform(op)
	if err != nil {
		a.toast(err.Error())
		return err
	}
	if inverse, ok := inverseOf(applied); ok {
		a.Undo.record(applied, inverse)
	}
	a.finishOperation(applied)
	return nil
}

func (a *appState) runOperation(op fileOperation) {
	go func() {
		applied, err := perform(op)
		if a.Window == nil {
			return
		}
		a.Window.Update(func() {
			if err != nil {
				a.toast(err.Error())
				return
			}
			if inverse, ok := inverseOf(applied); ok {
				a.Undo.record(applied, inverse)
			}
			a.finishOperation(applied)
		})
	}()
}

func (a *appState) undo() {
	e, ok := a.Undo.undo()
	if !ok {
		return
	}
	applied, err := perform(e.inverse)
	if err != nil {
		a.Undo.past = append(a.Undo.past, e)
		a.toast(err.Error())
		return
	}
	a.Undo.future = append(a.Undo.future, undoEntry{forward: e.forward, inverse: applied})
	a.finishOperation(applied)
}

func (a *appState) redo() {
	e, ok := a.Undo.redo()
	if !ok {
		return
	}
	applied, err := perform(e.forward)
	if err != nil {
		a.Undo.future = append(a.Undo.future, e)
		a.toast(err.Error())
		return
	}
	a.Undo.past = append(a.Undo.past, undoEntry{forward: applied, inverse: e.inverse})
	a.finishOperation(applied)
}

func (a *appState) finishOperation(op fileOperation) {
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
				if a.LargeIndex+1 < len(a.Visible) {
					next := a.Items[a.Visible[a.LargeIndex+1]].Path
					if _, drop := gone[next]; !drop {
						a.PendingLarge = next
					}
				}
				if a.PendingLarge == "" && a.LargeIndex > 0 {
					prev := a.Items[a.Visible[a.LargeIndex-1]].Path
					if _, drop := gone[prev]; !drop {
						a.PendingLarge = prev
					}
				}
			} else {
				a.KeepLarge = true
				a.PendingLarge = item.Path
			}
		}
	}
	if a.Video != nil {
		if _, ok := gone[a.Video.path]; ok && a.Video.player != nil {
			a.Video.player.Close()
			a.Video = nil
		}
	}
}

func (a *appState) retargetPaths(mappings []pathPair) {
	if len(mappings) == 0 {
		return
	}
	if a.Root != "" {
		next := remapPath(a.Root, mappings)
		if next != a.Root && isDir(next) {
			a.Root = next
			if a.Window != nil {
				a.Window.SetTitle(appName + " — " + pathName(next))
			}
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
}

func extOf(path string) string {
	ext := filepath.Ext(path)
	if len(ext) > 0 {
		ext = ext[1:]
	}
	return strings.ToLower(ext)
}
