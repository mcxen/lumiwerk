package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// pendingToast is a message waiting for the next frame, where showToasts
// hands it to mygo's c.Toast. undo adds a 撤销 button wired to a.undo.
type pendingToast struct {
	msg  string
	undo bool
}

// toast shows msg as a mygo toast. It may be called from any goroutine:
// the message waits in a queue the view drains.
func (a *appState) toast(msg string) { a.queueToast(pendingToast{msg: msg}) }

// toastUndo shows msg with a 撤销 button that undoes the newest operation.
func (a *appState) toastUndo(msg string) { a.queueToast(pendingToast{msg: msg, undo: true}) }

func (a *appState) queueToast(p pendingToast) {
	if p.msg == "" {
		return
	}
	a.toastMu.Lock()
	a.toasts = append(a.toasts, p)
	a.lastToast = p.msg
	a.toastMu.Unlock()
	if a.Window != nil {
		a.Window.Invalidate()
	}
}

// showToasts hands queued messages to mygo's toast viewport.
func (a *appState) showToasts(c *ui.Context) {
	a.toastMu.Lock()
	queued := a.toasts
	a.toasts = nil
	a.toastMu.Unlock()
	for _, p := range queued {
		if p.undo {
			c.ToastAction(p.msg, "撤销", a.undo)
		} else {
			c.Toast(p.msg)
		}
	}
}

// undoToast names an undoable operation just applied, for its 撤销 toast;
// "" for one that needs none.
func undoToast(op fileOperation) string {
	n := len(op.mappings)
	switch op.op {
	case opDelete:
		if n > 0 {
			return fmt.Sprintf("已移到废纸篓 %d 项", n)
		}
	case opMove:
		if n > 0 {
			return fmt.Sprintf("已移动 %d 项", n)
		}
	case opRename:
		if n > 0 {
			return fmt.Sprintf("已重命名 %d 项", n)
		}
	case opCompress, opExtract:
		return op.detail
	case opCopy:
		if len(op.created) > 0 {
			return fmt.Sprintf("已拷贝 %d 项", len(op.created))
		}
	}
	return ""
}

// recordUndo records an applied operation on the undo stack and, for a
// file change the user may want back, shows a toast with 撤销.
func (a *appState) recordUndo(applied, inverse fileOperation) {
	a.Undo.record(applied, inverse)
	if msg := undoToast(applied); msg != "" {
		a.toastUndo(msg)
	}
}
