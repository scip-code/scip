package scip

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOccurrence_SourceRange_TypedSingleLine(t *testing.T) {
	occ := &Occurrence{
		TypedRange: &Occurrence_SingleLineRange{
			SingleLineRange: &SingleLineRange{Line: 5, StartCharacter: 2, EndCharacter: 7},
		},
	}
	r, ok := occ.SourceRange()
	require.True(t, ok)
	require.Equal(t, Range{Start: Position{5, 2}, End: Position{5, 7}}, r)
}

func TestOccurrence_SourceRange_TypedMultiLine(t *testing.T) {
	occ := &Occurrence{
		TypedRange: &Occurrence_MultiLineRange{
			MultiLineRange: &MultiLineRange{StartLine: 1, StartCharacter: 2, EndLine: 3, EndCharacter: 4},
		},
	}
	r, ok := occ.SourceRange()
	require.True(t, ok)
	require.Equal(t, Range{Start: Position{1, 2}, End: Position{3, 4}}, r)
}

func TestOccurrence_SourceRange_DeprecatedFallback(t *testing.T) {
	occ := &Occurrence{Range: []int32{2, 3, 5}}
	r, ok := occ.SourceRange()
	require.True(t, ok)
	require.Equal(t, Range{Start: Position{2, 3}, End: Position{2, 5}}, r)
}

func TestOccurrence_SourceRange_TypedTakesPrecedenceOverDeprecated(t *testing.T) {
	// Per scip.proto, when both encodings are present the typed form wins.
	occ := &Occurrence{
		Range: []int32{100, 100, 100}, // deliberately disagrees
		TypedRange: &Occurrence_SingleLineRange{
			SingleLineRange: &SingleLineRange{Line: 5, StartCharacter: 2, EndCharacter: 7},
		},
	}
	r, ok := occ.SourceRange()
	require.True(t, ok)
	require.Equal(t, Range{Start: Position{5, 2}, End: Position{5, 7}}, r)
}

func TestOccurrence_SourceRange_Missing(t *testing.T) {
	occ := &Occurrence{}
	r, ok := occ.SourceRange()
	require.False(t, ok)
	require.Equal(t, Range{}, r)
}

func TestOccurrence_SourceRange_DeprecatedMalformed(t *testing.T) {
	occ := &Occurrence{Range: []int32{1, 2}} // wrong length
	r, ok := occ.SourceRange()
	require.False(t, ok)
	require.Equal(t, Range{}, r)
}

func TestOccurrence_EnclosingSourceRange_TypedSingleLine(t *testing.T) {
	occ := &Occurrence{
		TypedEnclosingRange: &Occurrence_SingleLineEnclosingRange{
			SingleLineEnclosingRange: &SingleLineRange{Line: 5, StartCharacter: 0, EndCharacter: 10},
		},
	}
	r, ok := occ.EnclosingSourceRange()
	require.True(t, ok)
	require.Equal(t, Range{Start: Position{5, 0}, End: Position{5, 10}}, r)
}

func TestOccurrence_EnclosingSourceRange_TypedMultiLine(t *testing.T) {
	occ := &Occurrence{
		TypedEnclosingRange: &Occurrence_MultiLineEnclosingRange{
			MultiLineEnclosingRange: &MultiLineRange{StartLine: 1, StartCharacter: 0, EndLine: 9, EndCharacter: 1},
		},
	}
	r, ok := occ.EnclosingSourceRange()
	require.True(t, ok)
	require.Equal(t, Range{Start: Position{1, 0}, End: Position{9, 1}}, r)
}

func TestOccurrence_EnclosingSourceRange_DeprecatedFallback(t *testing.T) {
	occ := &Occurrence{EnclosingRange: []int32{2, 0, 5, 1}}
	r, ok := occ.EnclosingSourceRange()
	require.True(t, ok)
	require.Equal(t, Range{Start: Position{2, 0}, End: Position{5, 1}}, r)
}

func TestOccurrence_EnclosingSourceRange_TypedTakesPrecedence(t *testing.T) {
	occ := &Occurrence{
		EnclosingRange: []int32{100, 100, 100},
		TypedEnclosingRange: &Occurrence_SingleLineEnclosingRange{
			SingleLineEnclosingRange: &SingleLineRange{Line: 5, StartCharacter: 0, EndCharacter: 10},
		},
	}
	r, ok := occ.EnclosingSourceRange()
	require.True(t, ok)
	require.Equal(t, Range{Start: Position{5, 0}, End: Position{5, 10}}, r)
}

func TestOccurrence_EnclosingSourceRange_Missing(t *testing.T) {
	occ := &Occurrence{}
	r, ok := occ.EnclosingSourceRange()
	require.False(t, ok)
	require.Equal(t, Range{}, r)
}

func TestOccurrence_SetSourceRange_SingleLine(t *testing.T) {
	occ := &Occurrence{Range: []int32{99, 99, 99}}
	occ.SetSourceRange(Range{Start: Position{3, 1}, End: Position{3, 8}})
	require.Nil(t, occ.Range)
	tr, ok := occ.TypedRange.(*Occurrence_SingleLineRange)
	require.True(t, ok)
	require.Equal(t, &SingleLineRange{Line: 3, StartCharacter: 1, EndCharacter: 8}, tr.SingleLineRange)

	r, ok := occ.SourceRange()
	require.True(t, ok)
	require.Equal(t, Range{Start: Position{3, 1}, End: Position{3, 8}}, r)
}

func TestOccurrence_SetSourceRange_MultiLine(t *testing.T) {
	occ := &Occurrence{}
	occ.SetSourceRange(Range{Start: Position{1, 0}, End: Position{4, 2}})
	tr, ok := occ.TypedRange.(*Occurrence_MultiLineRange)
	require.True(t, ok)
	require.Equal(t, &MultiLineRange{StartLine: 1, StartCharacter: 0, EndLine: 4, EndCharacter: 2}, tr.MultiLineRange)
}

func TestOccurrence_SetEnclosingSourceRange(t *testing.T) {
	occ := &Occurrence{EnclosingRange: []int32{99, 99, 99}}
	occ.SetEnclosingSourceRange(Range{Start: Position{1, 0}, End: Position{5, 1}})
	require.Nil(t, occ.EnclosingRange)
	tr, ok := occ.TypedEnclosingRange.(*Occurrence_MultiLineEnclosingRange)
	require.True(t, ok)
	require.Equal(t, &MultiLineRange{StartLine: 1, StartCharacter: 0, EndLine: 5, EndCharacter: 1}, tr.MultiLineEnclosingRange)
}

func TestOccurrence_Compare(t *testing.T) {
	mkOcc := func(r []int32, sym string) *Occurrence {
		return &Occurrence{Range: r, Symbol: sym}
	}
	tests := []struct {
		name string
		a, b *Occurrence
		want int
	}{
		{"equal", mkOcc([]int32{1, 0, 5}, "x"), mkOcc([]int32{1, 0, 5}, "x"), 0},
		{"earlier range", mkOcc([]int32{0, 0, 5}, "x"), mkOcc([]int32{1, 0, 5}, "x"), -1},
		{"later range", mkOcc([]int32{2, 0, 5}, "x"), mkOcc([]int32{1, 0, 5}, "x"), 1},
		{"same range, earlier symbol", mkOcc([]int32{1, 0, 5}, "a"), mkOcc([]int32{1, 0, 5}, "b"), -1},
		{"same range, later symbol", mkOcc([]int32{1, 0, 5}, "b"), mkOcc([]int32{1, 0, 5}, "a"), 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.a.Compare(tc.b))
		})
	}
}

func TestOccurrence_Contains(t *testing.T) {
	occ := &Occurrence{Range: []int32{2, 5, 10}}
	require.True(t, occ.Contains(Position{2, 5}))
	require.True(t, occ.Contains(Position{2, 9}))
	require.False(t, occ.Contains(Position{2, 4}))
	require.False(t, occ.Contains(Position{2, 10}))
	require.False(t, occ.Contains(Position{3, 0}))

	empty := &Occurrence{}
	require.False(t, empty.Contains(Position{0, 0}))
}
