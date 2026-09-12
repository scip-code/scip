package main

import (
	"testing"

	"github.com/scip-code/scip/bindings/go/scip"
)

// Regression for https://github.com/scip-code/scip/issues/282.
//
// Before the fix, countLinesOfCode called url.Parse on Index.Metadata.ProjectRoot
// directly. A Windows ProjectRoot of the form "file://X:\..." was rejected by
// url.Parse with `invalid port ":\..." after host` (it parses "X:" as
// host:port), so `scip stats` errored before reading any document.
//
// Exercising countLinesOfCode keeps the externally visible seam covered:
// scip stats -> statsMain -> countStatistics -> countLinesOfCode.
func TestCountLinesOfCode_WindowsFileURI(t *testing.T) {
	idx := &scip.Index{
		Metadata: &scip.Metadata{ProjectRoot: `file://D:\dev\scip-issue-282-does-not-exist`},
	}
	if _, err := countLinesOfCode(idx, ""); err != nil {
		t.Fatalf("countLinesOfCode returned error for Windows file URI: %v", err)
	}
}

// Regression for the localSource bug surfaced in the #282 review: when
// customProjectRoot is empty and the parsed rootPath exists on disk,
// localSource was left empty and the subsequent os.Stat("") returned
// `stat : no such file or directory`. This drives the "rootPath exists"
// branch on both platforms by using t.TempDir() as the ProjectRoot:
//   - POSIX: "file:///tmp/xxx" is parsed by url.Parse.
//   - Windows: "file://C:\Users\..." is handled by ProjectRootToLocalPath.
func TestCountLinesOfCode_RootPathExists(t *testing.T) {
	tmp := t.TempDir()
	idx := &scip.Index{
		Metadata: &scip.Metadata{ProjectRoot: "file://" + tmp},
	}
	if _, err := countLinesOfCode(idx, ""); err != nil {
		t.Fatalf("countLinesOfCode returned error when rootPath exists: %v", err)
	}
}
