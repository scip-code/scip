package main

import (
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/scip-code/scip/bindings/go/scip"
)

func TestMerge_PlanPaths(t *testing.T) {
	cases := []struct {
		name         string
		roots        []string
		override     string
		wantRoot     string
		wantPrefixes []string
		wantErr      bool
	}{
		{
			name:         "identical roots",
			roots:        []string{"file:///repo", "file:///repo"},
			wantRoot:     "file:///repo",
			wantPrefixes: []string{"", ""},
		},
		{
			name:         "sibling subdirs",
			roots:        []string{"file:///repo/frontend", "file:///repo/backend"},
			wantRoot:     "file:///repo",
			wantPrefixes: []string{"frontend", "backend"},
		},
		{
			name:         "one is ancestor of the other",
			roots:        []string{"file:///repo", "file:///repo/sub"},
			wantRoot:     "file:///repo",
			wantPrefixes: []string{"", "sub"},
		},
		{
			name:         "deep nesting",
			roots:        []string{"file:///repo/a/b/c", "file:///repo/a/b/d", "file:///repo/a/e"},
			wantRoot:     "file:///repo/a",
			wantPrefixes: []string{"b/c", "b/d", "e"},
		},
		{
			name:         "common component is filesystem root",
			roots:        []string{"file:///alpha/x", "file:///beta/y"},
			wantRoot:     "file:///",
			wantPrefixes: []string{"alpha/x", "beta/y"},
		},
		{
			name:         "trailing slashes normalized",
			roots:        []string{"file:///repo/", "file:///repo/sub"},
			wantRoot:     "file:///repo",
			wantPrefixes: []string{"", "sub"},
		},
		{
			name:         "prefix-like names don't share parent",
			roots:        []string{"file:///repo/frontend", "file:///repo/frontend-v2"},
			wantRoot:     "file:///repo",
			wantPrefixes: []string{"frontend", "frontend-v2"},
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
			name:         "override at a common ancestor",
			roots:        []string{"file:///workspace/repo/frontend", "file:///workspace/repo/backend"},
			override:     "file:///workspace",
			wantRoot:     "file:///workspace",
			wantPrefixes: []string{"repo/frontend", "repo/backend"},
		},
		{
			name:     "override must be ancestor of all inputs",
			roots:    []string{"file:///repo/frontend", "file:///elsewhere/backend"},
			override: "file:///repo",
			wantErr:  true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			parsed := make([]*url.URL, len(c.roots))
			for i, r := range c.roots {
				u, err := parseRootURI(r)
				require.NoError(t, err)
				parsed[i] = u
			}
			gotURL, gotPrefixes, err := planPaths(parsed, c.override)
			if c.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.wantRoot, gotURL.String())
			require.Equal(t, c.wantPrefixes, gotPrefixes)
		})
	}
}

func TestMerge_RelativeTo(t *testing.T) {
	cases := []struct {
		name    string
		parent  string
		child   string
		want    string
		wantErr bool
	}{
		{"same", "/repo", "/repo", "", false},
		{"direct child", "/repo", "/repo/sub", "sub", false},
		{"deep child", "/repo", "/repo/a/b/c", "a/b/c", false},
		{"parent is filesystem root", "/", "/alpha/x", "alpha/x", false},
		{"parent equals root and child equals root", "/", "/", "", false},
		{"input is ancestor (error)", "/repo/sub", "/repo", "", true},
		{"siblings (error)", "/repo/frontend", "/repo/backend", "", true},
		{"prefix-like but not ancestor", "/repo/frontend", "/repo/frontend-v2", "", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := relativeTo(c.parent, c.child)
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

func TestMergeIndexes_IncompatibleProtocolVersion(t *testing.T) {
	a := &scip.Index{
		Metadata: &scip.Metadata{
			ProjectRoot:          "file:///repo",
			TextDocumentEncoding: scip.TextEncoding_UTF8,
			Version:              scip.ProtocolVersion_UnspecifiedProtocolVersion,
		},
	}
	b := &scip.Index{
		Metadata: &scip.Metadata{
			ProjectRoot:          "file:///repo",
			TextDocumentEncoding: scip.TextEncoding_UTF8,
			Version:              scip.ProtocolVersion(42), // imaginary future version
		},
	}

	_, err := mergeIndexes([]*scip.Index{a, b}, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "protocol version")
}

func TestMergeIndexes_MissingMetadata(t *testing.T) {
	a := &scip.Index{
		Metadata: &scip.Metadata{
			ProjectRoot:          "file:///repo",
			TextDocumentEncoding: scip.TextEncoding_UTF8,
		},
	}
	b := &scip.Index{Metadata: nil}

	_, err := mergeIndexes([]*scip.Index{a, b}, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "metadata")
}

func TestMergeIndexes_EmptyRelativePathGetsPrefix(t *testing.T) {
	a := &scip.Index{
		Metadata: &scip.Metadata{
			ProjectRoot:          "file:///repo/frontend",
			TextDocumentEncoding: scip.TextEncoding_UTF8,
		},
		Documents: []*scip.Document{
			{RelativePath: ""}, // empty -- represents the project root itself
		},
	}
	b := &scip.Index{
		Metadata: &scip.Metadata{
			ProjectRoot:          "file:///repo/backend",
			TextDocumentEncoding: scip.TextEncoding_UTF8,
		},
		Documents: []*scip.Document{
			{RelativePath: "main.go"},
		},
	}

	merged, err := mergeIndexes([]*scip.Index{a, b}, "")
	require.NoError(t, err)
	require.Equal(t, "file:///repo", merged.Metadata.ProjectRoot)
	require.ElementsMatch(t,
		[]string{"frontend", "backend/main.go"},
		docPaths(merged))
}

func TestMergeIndexes_UnspecifiedAndConcreteEncodingMismatch(t *testing.T) {
	// Unspecified is treated as a distinct value, not a wildcard: mixing it
	// with a concrete encoding must error rather than silently labeling the
	// merged output with the concrete encoding (which could mislabel the
	// Unspecified index's source files).
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

	_, err := mergeIndexes([]*scip.Index{a, b}, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "text encoding")
}

func TestMergeIndexes_AllUnspecifiedEncodingPreserved(t *testing.T) {
	// When every input has the same encoding (including Unspecified), the
	// merged output preserves it.
	a := &scip.Index{
		Metadata: &scip.Metadata{
			ProjectRoot:          "file:///repo",
			TextDocumentEncoding: scip.TextEncoding_UnspecifiedTextEncoding,
		},
	}
	b := &scip.Index{
		Metadata: &scip.Metadata{
			ProjectRoot:          "file:///repo",
			TextDocumentEncoding: scip.TextEncoding_UnspecifiedTextEncoding,
		},
	}

	merged, err := mergeIndexes([]*scip.Index{a, b}, "")
	require.NoError(t, err)
	require.Equal(t, scip.TextEncoding_UnspecifiedTextEncoding, merged.Metadata.TextDocumentEncoding)
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
