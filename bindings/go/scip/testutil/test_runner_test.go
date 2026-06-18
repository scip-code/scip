package testutil

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/scip-code/scip/bindings/go/scip"
)

func TestTestCasesForLine(t *testing.T) {
	actual, linesUsed := testCasesForLine(
		2,
		[]string{
			"void main() {",
			" //   ^ definition <skipped line>",
			"  print(foo)",
			"  //    ^^^ reference <with enforce length>",
			"  //     ^ reference <single line>",
			"        // <- reference <start of comment>",
			"  //    ^ diagnostic Warning",
			"  //    > with multiline",
			"  //    > additional data",
			"  final test = 4",
			"         ^ definition <skipped line>",
			"}",
		},
		"//",
	)

	// total of 4 test cases on the specified line
	require.Equal(t, 4, len(actual))
	require.Equal(t, 6, linesUsed)

	// only the first test case has enforceLength = true
	require.True(t, actual[0].enforceLength)
	require.Equal(t, 3, actual[0].attribute.length)
	require.False(t, actual[1].enforceLength)
	require.False(t, actual[2].enforceLength)
	require.False(t, actual[3].enforceLength)

	// all test cases should have the same start
	require.Equal(t, 8, actual[0].attribute.start)
	require.Equal(t, 9, actual[1].attribute.start) // different start, same symbol
	require.Equal(t, 8, actual[2].attribute.start)
	require.Equal(t, 8, actual[3].attribute.start)

	// validate kind on each test case
	require.Equal(t, referenceAttrKind, actual[0].attribute.kind)
	require.Equal(t, referenceAttrKind, actual[1].attribute.kind)
	require.Equal(t, referenceAttrKind, actual[2].attribute.kind)
	require.Equal(t, diagnosticAttrKind, actual[3].attribute.kind)

	// validate that additionalData is correctly parsed
	require.True(
		t,
		slices.Equal(
			actual[3].attribute.additionalData,
			[]string{"with multiline", "additional data"},
		),
	)
}

func TestParseTestCaseSyntheticDefinition(t *testing.T) {
	tc, ok := parseTestCase("//   ___ synthetic_definition local child", []string{}, "//")
	require.True(t, ok)
	require.Equal(t, syntheticDefinitionAttrKind, tc.attribute.kind)
	require.Equal(t, "local child", tc.attribute.data)
	require.True(t, tc.enforceLength)
	// only the contiguous run of 3 underscores counts, not the '_' inside the
	// "synthetic_definition" kind.
	require.Equal(t, 3, tc.attribute.length)
	require.Equal(t, 5, tc.attribute.start)
	require.False(t, tc.attribute.hasEndPosition)
}

func TestParseTestCaseMultilineSuffix(t *testing.T) {
	tc, ok := parseTestCase("// ^^^^^ reference local bar 2:1", []string{}, "//")
	require.True(t, ok)
	require.Equal(t, referenceAttrKind, tc.attribute.kind)
	require.Equal(t, "local bar", tc.attribute.data)
	require.True(t, tc.attribute.hasEndPosition)
	require.Equal(t, 2, tc.attribute.endLineDelta)
	require.Equal(t, 1, tc.attribute.endCharacter)
}

func TestParseTestCaseUnderscoreRejectsNonSynthetic(t *testing.T) {
	require.Panics(t, func() {
		parseTestCase("// ___ reference local child", []string{}, "//")
	})
}

func TestParseTestCaseIgnoresMetadataLines(t *testing.T) {
	for _, line := range []string{
		"//            kind Namespace",
		"//            display_name foo",
		"//            enclosing_symbol local root",
		"//            relationship local foo definition",
		"//            > documentation line",
	} {
		_, ok := parseTestCase(line, []string{}, "//")
		require.Falsef(t, ok, "expected %q to be ignored", line)
	}
}

func TestCheckSymbolPartCountMismatch(t *testing.T) {
	tc := symbolAttributeTestCase{
		attribute: symbolAttribute{
			kind:   definitionAttrKind,
			start:  0,
			length: 3,
			data:   "scheme manager name 1.0.0 Foo#",
		},
		enforceLength: true,
	}
	attr := symbolAttribute{
		kind:   definitionAttrKind,
		start:  0,
		length: 3,
		data:   "local 0",
	}
	// Must not panic, and must not match (different number of symbol parts).
	require.False(t, tc.check(attr))
}

func TestRunTestsSyntheticAndMultiline(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	index := &scip.Index{
		Documents: []*scip.Document{{
			RelativePath: "test.txt",
			Occurrences: []*scip.Occurrence{
				{
					Symbol:      "local foo",
					SymbolRoles: int32(scip.SymbolRole_Definition),
					TypedRange:  rangeAt(0, 3, 0, 6).AsTypedRange(),
				},
				{
					Symbol:     "local bar",
					TypedRange: rangeAt(3, 3, 5, 1).AsTypedRange(),
				},
			},
			Symbols: []*scip.SymbolInformation{
				{Symbol: "local foo"},
				{
					Symbol:        "local child",
					Relationships: []*scip.Relationship{{Symbol: "local foo", IsDefinition: true}},
				},
			},
		}},
	}

	passContent := strings.Join([]string{
		"   foo",
		"// ^^^ definition local foo",
		"// ___ synthetic_definition local child",
		"   barbar",
		"// ^^^^^^ reference local bar 2:1",
		"mid",
		"x",
	}, "\n")

	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte(passContent), 0644))

	var passOut bytes.Buffer
	require.NoError(t, RunTests(dir, nil, false, index, "//", &passOut))
	require.Contains(t, passOut.String(), "✓ test.txt (3 assertions)")

	// A wrong multiline end position must fail and report the actual suffix.
	failContent := strings.Replace(passContent,
		"reference local bar 2:1", "reference local bar 2:2", 1)
	require.NoError(t, os.WriteFile(testFile, []byte(failContent), 0644))

	var failOut bytes.Buffer
	require.Error(t, RunTests(dir, nil, false, index, "//", &failOut))
	require.Contains(t, failOut.String(), "reference local bar 2:2") // expected
	require.Contains(t, failOut.String(), "reference local bar 2:1") // actual
}
