package main

import "path/filepath"

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

func (a *appState) performTracked(op fileOperation) error {
	var inverse fileOperation
	switch op.op {
	case opMove:
		var dsts []string
		for _, src := range op.paths {
			dsts = append(dsts, filepath.Join(op.dest, filepath.Base(src)))
		}
		inverse = fileOperation{op: opMove, paths: dsts, dest: filepath.Dir(op.paths[0])}
	case opRename:
		src := op.paths[0]
		inverse = fileOperation{op: opRename, paths: []string{filepath.Join(filepath.Dir(src), op.newName)}, newName: filepath.Base(src)}
	}
	if err := perform(op); err != nil {
		return err
	}
	if inverse.op != 0 {
		a.Undo.record(op, inverse)
	}
	return nil
}

func (a *appState) undo() {
	e, ok := a.Undo.undo()
	if !ok || perform(e.inverse) != nil {
		return
	}
	a.Undo.future = append(a.Undo.future, e)
	a.refresh()
}

func (a *appState) redo() {
	e, ok := a.Undo.redo()
	if !ok || perform(e.forward) != nil {
		return
	}
	a.Undo.past = append(a.Undo.past, e)
	a.refresh()
}
