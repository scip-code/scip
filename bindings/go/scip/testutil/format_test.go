package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/scip-code/scip/bindings/go/scip"
)

func rangeAt(startLine, startChar, endLine, endChar int32) scip.Range {
	return scip.Range{
		Start: scip.Position{Line: startLine, Character: startChar},
		End:   scip.Position{Line: endLine, Character: endChar},
	}
}

// TestFormatSnapshotFeatures exercises enclosing_symbol, multiline occurrences
// and synthetic_definition rendering, plus the zero-width range edge case.
func TestFormatSnapshotFeatures(t *testing.T) {
	source := "package foo\nfunc bar() {\n  return\n}\n"
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "test.go")
	require.NoError(t, os.WriteFile(sourcePath, []byte(source), 0644))

	document := &scip.Document{
		RelativePath: "test.go",
		Occurrences: []*scip.Occurrence{
			{
				Symbol:      "local foo",
				SymbolRoles: int32(scip.SymbolRole_Definition),
				TypedRange:  rangeAt(0, 8, 0, 11).AsTypedRange(),
			},
			{
				// Multiline reference spanning lines 1..3.
				Symbol:     "local bar",
				TypedRange: rangeAt(1, 5, 3, 1).AsTypedRange(),
			},
			{
				// Zero-width definition: still renders a single marker.
				Symbol:      "local zero",
				SymbolRoles: int32(scip.SymbolRole_Definition),
				TypedRange:  rangeAt(2, 2, 2, 2).AsTypedRange(),
			},
		},
		Symbols: []*scip.SymbolInformation{
			{
				Symbol:          "local foo",
				Kind:            scip.SymbolInformation_Namespace,
				DisplayName:     "foo",
				EnclosingSymbol: "local root",
			},
			{
				// Defined at "local foo"'s definition site; rendered as a
				// synthetic_definition under it.
				Symbol:        "local child",
				Kind:          scip.SymbolInformation_Field,
				DisplayName:   "child",
				Relationships: []*scip.Relationship{{Symbol: "local foo", IsDefinition: true}},
			},
		},
	}

	snapshot, err := FormatSnapshot(document, "//", scip.DescriptorOnlyFormatter, sourcePath)
	require.NoError(t, err)

	// enclosing_symbol is rendered through the symbol formatter.
	require.Contains(t, snapshot, "//        ^^^ definition local foo")
	require.Contains(t, snapshot, "enclosing_symbol local root")

	// Synthetic definition: distinct child symbol, underscore markers, and the
	// is_definition relationship rendered as "definition".
	require.Contains(t, snapshot, "//        ___ synthetic_definition local child")
	require.Contains(t, snapshot, "relationship local foo definition")

	// Multiline occurrence: markers extend to end of the start line and a
	// "<lineDelta>:<endCharacter>" suffix reports the end position.
	require.Contains(t, snapshot, "//     ^^^^^^^ reference local bar 2:1")

	// Zero-width range still renders exactly one marker.
	require.Contains(t, snapshot, "//  ^ definition local zero")
}
