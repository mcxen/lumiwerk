//go:build darwin

package main

import (
	"errors"
	"strings"
	"testing"
)

func TestTrashPermissionErrorFromOsascript(t *testing.T) {
	original := finderTrashCommand
	finderTrashCommand = func(string) ([]byte, error) {
		return []byte("execution error: Not authorized to send Apple events to Finder. (-1743)\n"), errors.New("exit status 1")
	}
	t.Cleanup(func() { finderTrashCommand = original })

	_, err := platformTrash("/tmp/lumiwerk-permission-test.txt")
	var perm *trashPermissionError
	if !errors.As(err, &perm) || !errors.Is(err, errTrashPermission) {
		t.Fatalf("permission error = %#v", err)
	}
	msg := err.Error()
	if strings.Contains(msg, "exit status") || !strings.Contains(msg, "自动化") || !strings.Contains(msg, "权限") {
		t.Fatalf("unclear user-facing error %q", msg)
	}
}
