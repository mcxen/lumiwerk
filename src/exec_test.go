package main

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestExecCommandKeepsArgs(t *testing.T) {
	cmd := execCommand("ffmpeg", "-hide_banner", "-version")
	if got := cmd.Args; len(got) != 3 || got[0] != "ffmpeg" || got[1] != "-hide_banner" || got[2] != "-version" {
		t.Fatalf("args = %v", got)
	}
}

func TestExecCommandContextKeepsArgs(t *testing.T) {
	cmd := execCommandContext(context.Background(), "ffprobe", "-v", "error")
	if got := cmd.Args; len(got) != 3 || got[0] != "ffprobe" || got[1] != "-v" || got[2] != "error" {
		t.Fatalf("args = %v", got)
	}
}

func TestExecAppCommandKeepsArgs(t *testing.T) {
	cmd := execAppCommand("open", "/tmp/a.png")
	if got := cmd.Args; len(got) != 2 || got[0] != "open" || got[1] != "/tmp/a.png" {
		t.Fatalf("args = %v", got)
	}
}

// TestNoDirectExecCommandInProduction ensures every production child process
// goes through execCommand / execCommandContext / execAppCommand so Windows
// builds never leak a console window from a raw os/exec call.
func TestNoDirectExecCommandInProduction(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		checked++
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		var currentFunc string
		ast.Inspect(file, func(n ast.Node) bool {
			if fn, ok := n.(*ast.FuncDecl); ok {
				currentFunc = fn.Name.Name
				return true
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "exec" {
				return true
			}
			if sel.Sel.Name != "Command" && sel.Sel.Name != "CommandContext" {
				return true
			}
			if name == "exec.go" {
				return true
			}
			pos := fset.Position(call.Pos())
			t.Errorf("%s:%d: direct exec.%s call in %s; use execCommand/execCommandContext/execAppCommand instead",
				filepath.Base(pos.Filename), pos.Line, sel.Sel.Name, currentFunc)
			return true
		})
	}
	if checked == 0 {
		t.Fatal("no production .go files scanned")
	}
}

// TestFFmpegCallsIncludeNoStdin ensures every ffmpeg command built in
// production code passes -nostdin so ffmpeg never tries to grab a console.
func TestFFmpegCallsIncludeNoStdin(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	ffmpegCalls := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		var currentFunc *ast.FuncDecl
		ast.Inspect(file, func(n ast.Node) bool {
			if fn, ok := n.(*ast.FuncDecl); ok {
				currentFunc = fn
				return true
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			ident, ok := call.Fun.(*ast.Ident)
			if !ok || (ident.Name != "execCommand" && ident.Name != "execCommandContext") {
				return true
			}
			argIdx := 0
			if ident.Name == "execCommandContext" {
				argIdx = 1
			}
			if len(call.Args) <= argIdx {
				return true
			}
			isFFmpeg := isToolPathCall(call.Args[argIdx], "ffmpeg") ||
				(currentFunc != nil && currentFunc.Name.Name == "ffmpegVideoThumb")
			if !isFFmpeg {
				return true
			}
			ffmpegCalls++
			if currentFunc != nil && funcMentionsNoStdinOrHelper(currentFunc) {
				return true
			}
			pos := fset.Position(call.Pos())
			t.Errorf("%s:%d: ffmpeg call in %s does not pass -nostdin",
				filepath.Base(pos.Filename), pos.Line, currentFunc.Name.Name)
			return true
		})
	}
	if ffmpegCalls == 0 {
		t.Fatal("no ffmpeg command calls found")
	}
}

func isToolPathCall(expr ast.Expr, tool string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	ident, ok := call.Fun.(*ast.Ident)
	if !ok || ident.Name != "toolPath" {
		return false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	return ok && lit.Kind == token.STRING && lit.Value == `"`+tool+`"`
}

func funcMentionsNoStdinOrHelper(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		switch v := n.(type) {
		case *ast.BasicLit:
			if v.Kind == token.STRING && v.Value == `"-nostdin"` {
				found = true
			}
		case *ast.Ident:
			switch v.Name {
			case "playbackArgsMapped", "soundtrackArgsMapped", "ffmpegArgsWithNoStdin":
				found = true
			}
		}
		return !found
	})
	return found
}

func TestFFmpegHelpersPassNoStdin(t *testing.T) {
	if !slices.Contains(playbackArgs("/tmp/a.mp4", 0, 0, true, 1920), "-nostdin") {
		t.Fatal("playbackArgs must include -nostdin")
	}
	if !slices.Contains(soundtrackArgs("/tmp/a.mp4", 0), "-nostdin") {
		t.Fatal("soundtrackArgs must include -nostdin")
	}
	got := ffmpegArgsWithNoStdin([]string{"-hide_banner", "-i", "/tmp/a.mp4"})
	if !slices.Contains(got, "-nostdin") {
		t.Fatalf("ffmpegArgsWithNoStdin = %v, want -nostdin", got)
	}
	dedup := ffmpegArgsWithNoStdin([]string{"-hide_banner", "-nostdin", "-i", "/tmp/a.mp4"})
	count := 0
	for _, arg := range dedup {
		if arg == "-nostdin" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("ffmpegArgsWithNoStdin duplicated -nostdin: %v", dedup)
	}
}
