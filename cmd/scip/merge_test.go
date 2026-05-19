package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/scip-code/scip/bindings/go/scip"
)

func TestMerge_CommonAncestorURI(t *testing.T) {
	cases := []struct {
		name    string
		roots   []string
		want    string
		wantErr bool
	}{
		{
			name:  "identical roots",
			roots: []string{"file:///repo", "file:///repo"},
			want:  "file:///repo",
		},
		{
			name:  "sibling subdirs",
			roots: []string{"file:///repo/frontend", "file:///repo/backend"},
			want:  "file:///repo",
		},
		{
			name:  "one is ancestor of the other",
			roots: []string{"file:///repo", "file:///repo/sub"},
			want:  "file:///repo",
		},
		{
			name:  "deep nesting",
			roots: []string{"file:///repo/a/b/c", "file:///repo/a/b/d", "file:///repo/a/e"},
			want:  "file:///repo/a",
		},
		{
			name:  "common component is filesystem root",
			roots: []string{"file:///alpha/x", "file:///beta/y"},
			want:  "file:///",
		},
		{
			name:  "trailing slashes normalized",
			roots: []string{"file:///repo/", "file:///repo/sub"},
			want:  "file:///repo",
		},
		{
			name:    "different schemes error",
			roots:   []string{"file:///repo", "https://example.com/repo"},
			wantErr: true,
		},
		{
			name:    "different hosts error",
			roots:   []string{"https://a.com/repo", "https://b.com/repo"},
			wantErr: true,
		},
		{
			name:  "non-overlapping but same scheme is ancestor=/",
			roots: []string{"file:///repo/frontend", "file:///different-name/backend"},
			want:  "file:///",
		},
		{
			name:  "prefix-like names don't share parent",
			roots: []string{"file:///repo/frontend", "file:///repo/frontend-v2"},
			want:  "file:///repo",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := commonAncestorURI(c.roots)
			if c.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.want, got)
		})
	}
}

func TestMerge_RelativePrefix(t *testing.T) {
	cases := []struct {
		name       string
		outputRoot string
		inputRoot  string
		want       string
		wantErr    bool
	}{
		{
			name:       "same root",
			outputRoot: "file:///repo",
			inputRoot:  "file:///repo",
			want:       "",
		},
		{
			name:       "direct child",
			outputRoot: "file:///repo",
			inputRoot:  "file:///repo/sub",
			want:       "sub",
		},
		{
			name:       "deep child",
			outputRoot: "file:///repo",
			inputRoot:  "file:///repo/a/b/c",
			want:       "a/b/c",
		},
		{
			name:       "input is ancestor (error)",
			outputRoot: "file:///repo/sub",
			inputRoot:  "file:///repo",
			wantErr:    true,
		},
		{
			name:       "siblings (error)",
			outputRoot: "file:///repo/frontend",
			inputRoot:  "file:///repo/backend",
			wantErr:    true,
		},
		{
			name:       "prefix-like but not actual ancestor",
			outputRoot: "file:///repo/frontend",
			inputRoot:  "file:///repo/frontend-v2",
			wantErr:    true,
		},
		{
			name:       "output is filesystem root",
			outputRoot: "file:///",
			inputRoot:  "file:///alpha/x",
			want:       "alpha/x",
		},
		{
			name:       "trailing slash on input",
			outputRoot: "file:///repo",
			inputRoot:  "file:///repo/sub/",
			want:       "sub",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := relativePrefix(c.outputRoot, c.inputRoot)
			if c.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.want, got)
		})
	}
}

func TestMergeIndexes_AutoInferredRoot(t *testing.T) {
	a := &scip.Index{
		Metadata: &scip.Metadata{
			ProjectRoot:          "file:///repo/frontend",
			TextDocumentEncoding: scip.TextEncoding_UTF8,
		},
		Documents: []*scip.Document{
			{
				RelativePath: "src/a.ts",
				Occurrences: []*scip.Occurrence{
					{Symbol: "scip-ts npm pkg-a 1.0 A#", Range: []int32{0, 0, 1}, SymbolRoles: int32(scip.SymbolRole_Definition)},
				},
				Symbols: []*scip.SymbolInformation{{Symbol: "scip-ts npm pkg-a 1.0 A#"}},
			},
		},
		ExternalSymbols: []*scip.SymbolInformation{
			{Symbol: "scip-ts npm shared 1.0 Util#"},
		},
	}
	b := &scip.Index{
		Metadata: &scip.Metadata{
			ProjectRoot:          "file:///repo/backend",
			TextDocumentEncoding: scip.TextEncoding_UTF8,
		},
		Documents: []*scip.Document{
			{
				RelativePath: "src/b.go",
				Occurrences: []*scip.Occurrence{
					{Symbol: "scip-go go . . pkg/B#", Range: []int32{0, 0, 1}, SymbolRoles: int32(scip.SymbolRole_Definition)},
				},
				Symbols: []*scip.SymbolInformation{{Symbol: "scip-go go . . pkg/B#"}},
			},
		},
		ExternalSymbols: []*scip.SymbolInformation{
			{Symbol: "scip-ts npm shared 1.0 Util#"},
		},
	}

	merged, err := mergeIndexes([]*scip.Index{a, b}, "")
	require.NoError(t, err)

	require.NotNil(t, merged.Metadata)
	require.Equal(t, "file:///repo", merged.Metadata.ProjectRoot)
	require.Equal(t, scip.TextEncoding_UTF8, merged.Metadata.TextDocumentEncoding)
	require.NotNil(t, merged.Metadata.ToolInfo)
	require.Equal(t, "scip", merged.Metadata.ToolInfo.Name)

	paths := docPaths(merged)
	require.ElementsMatch(t, []string{"frontend/src/a.ts", "backend/src/b.go"}, paths)

	// External symbols should be deduped.
	require.Len(t, merged.ExternalSymbols, 1)
	require.Equal(t, "scip-ts npm shared 1.0 Util#", merged.ExternalSymbols[0].Symbol)
}

func TestMergeIndexes_SameRootDeduplicatesDocuments(t *testing.T) {
	a := &scip.Index{
		Metadata: &scip.Metadata{
			ProjectRoot:          "file:///repo",
			TextDocumentEncoding: scip.TextEncoding_UTF8,
		},
		Documents: []*scip.Document{
			{
				RelativePath: "src/shared.go",
				Symbols:      []*scip.SymbolInformation{{Symbol: "shared.A"}},
				Occurrences: []*scip.Occurrence{
					{Symbol: "shared.A", Range: []int32{0, 0, 1}, SymbolRoles: int32(scip.SymbolRole_Definition)},
				},
			},
		},
	}
	b := &scip.Index{
		Metadata: &scip.Metadata{
			ProjectRoot:          "file:///repo",
			TextDocumentEncoding: scip.TextEncoding_UTF8,
		},
		Documents: []*scip.Document{
			{
				RelativePath: "src/shared.go",
				Symbols:      []*scip.SymbolInformation{{Symbol: "shared.B"}},
				Occurrences: []*scip.Occurrence{
					{Symbol: "shared.B", Range: []int32{1, 0, 1}, SymbolRoles: int32(scip.SymbolRole_Definition)},
				},
			},
		},
	}

	merged, err := mergeIndexes([]*scip.Index{a, b}, "")
	require.NoError(t, err)

	require.Equal(t, "file:///repo", merged.Metadata.ProjectRoot)
	require.Len(t, merged.Documents, 1, "documents with the same relative_path should be merged")
	doc := merged.Documents[0]
	require.Equal(t, "src/shared.go", doc.RelativePath)
	require.Len(t, doc.Symbols, 2)
	require.Len(t, doc.Occurrences, 2)
}

func TestMergeIndexes_OverrideProjectRoot(t *testing.T) {
	a := &scip.Index{
		Metadata: &scip.Metadata{
			ProjectRoot:          "file:///workspace/repo/frontend",
			TextDocumentEncoding: scip.TextEncoding_UTF8,
		},
		Documents: []*scip.Document{
			{RelativePath: "x.ts"},
		},
	}
	b := &scip.Index{
		Metadata: &scip.Metadata{
			ProjectRoot:          "file:///workspace/repo/backend",
			TextDocumentEncoding: scip.TextEncoding_UTF8,
		},
		Documents: []*scip.Document{
			{RelativePath: "y.go"},
		},
	}

	merged, err := mergeIndexes([]*scip.Index{a, b}, "file:///workspace")
	require.NoError(t, err)

	require.Equal(t, "file:///workspace", merged.Metadata.ProjectRoot)
	paths := docPaths(merged)
	require.ElementsMatch(t, []string{"repo/frontend/x.ts", "repo/backend/y.go"}, paths)
}

func TestMergeIndexes_OverrideMustBeAncestor(t *testing.T) {
	a := &scip.Index{
		Metadata: &scip.Metadata{
			ProjectRoot:          "file:///repo/frontend",
			TextDocumentEncoding: scip.TextEncoding_UTF8,
		},
	}
	b := &scip.Index{
		Metadata: &scip.Metadata{
			ProjectRoot:          "file:///elsewhere/backend",
			TextDocumentEncoding: scip.TextEncoding_UTF8,
		},
	}

	_, err := mergeIndexes([]*scip.Index{a, b}, "file:///repo")
	require.Error(t, err)
}

func TestMergeIndexes_IncompatibleEncoding(t *testing.T) {
	a := &scip.Index{
		Metadata: &scip.Metadata{
			ProjectRoot:          "file:///repo",
			TextDocumentEncoding: scip.TextEncoding_UTF8,
		},
	}
	b := &scip.Index{
		Metadata: &scip.Metadata{
			ProjectRoot:          "file:///repo",
			TextDocumentEncoding: scip.TextEncoding_UTF16,
		},
	}

	_, err := mergeIndexes([]*scip.Index{a, b}, "")
	require.Error(t, err)
}

func TestMergeIndexes_UnspecifiedEncodingPromotes(t *testing.T) {
	a := &scip.Index{
		Metadata: &scip.Metadata{
			ProjectRoot:          "file:///repo",
			TextDocumentEncoding: scip.TextEncoding_UnspecifiedTextEncoding,
		},
	}
	b := &scip.Index{
		Metadata: &scip.Metadata{
			ProjectRoot:          "file:///repo",
			TextDocumentEncoding: scip.TextEncoding_UTF8,
		},
	}

	merged, err := mergeIndexes([]*scip.Index{a, b}, "")
	require.NoError(t, err)
	require.Equal(t, scip.TextEncoding_UTF8, merged.Metadata.TextDocumentEncoding)
}

func TestMergeMain_EndToEnd(t *testing.T) {
	tempDir := t.TempDir()

	write := func(name string, idx *scip.Index) string {
		p := filepath.Join(tempDir, name)
		data, err := proto.Marshal(idx)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(p, data, 0o644))
		return p
	}

	aPath := write("a.scip", &scip.Index{
		Metadata: &scip.Metadata{
			ProjectRoot:          "file:///repo/frontend",
			TextDocumentEncoding: scip.TextEncoding_UTF8,
		},
		Documents: []*scip.Document{{RelativePath: "src/a.ts"}},
	})
	bPath := write("b.scip", &scip.Index{
		Metadata: &scip.Metadata{
			ProjectRoot:          "file:///repo/backend",
			TextDocumentEncoding: scip.TextEncoding_UTF8,
		},
		Documents: []*scip.Document{{RelativePath: "src/b.go"}},
	})

	outPath := filepath.Join(tempDir, "merged.scip")
	err := mergeMain(
		[]string{aPath, bPath},
		mergeFlags{output: outPath},
	)
	require.NoError(t, err)

	merged, err := readFromOption(outPath)
	require.NoError(t, err)

	require.Equal(t, "file:///repo", merged.Metadata.ProjectRoot)
	require.ElementsMatch(t,
		[]string{"frontend/src/a.ts", "backend/src/b.go"},
		docPaths(merged))
}

func docPaths(idx *scip.Index) []string {
	out := make([]string, 0, len(idx.Documents))
	for _, d := range idx.Documents {
		out = append(out, d.RelativePath)
	}
	sort.Strings(out)
	return out
}
