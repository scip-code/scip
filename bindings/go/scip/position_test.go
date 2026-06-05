package scip

import (
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// TODO: Replace with cmp.Compare go 1.21 or newer.
func compare(i1 int, i2 int) int {
	if i1 < i2 {
		return -1
	}
	if i1 > i2 {
		return 1
	}
	return 0
}

func TestUnitComparePosition(t *testing.T) {
	positions := []Position{
		{0, 0},
		{0, 1},
		{1, 0},
		{1, 1},
		{1, 2},
		{2, 1},
		{2, 2},
	}

	type testCase struct {
		p1       Position
		p2       Position
		expected int
	}

	testCases := make([]testCase, len(positions)*len(positions))

	// There is a total ordering on these positions, so we just test all possible combinations
	for i1, pos1 := range positions {
		for i2, pos2 := range positions {
			testCases = append(testCases, testCase{pos1, pos2, compare(i1, i2)})
		}
	}

	for _, testCase := range testCases {
		if actual := testCase.p1.Compare(testCase.p2); actual != testCase.expected {
			t.Errorf("unexpected result. %+v.Compare(%+v) want=%d have=%d", testCase.p1, testCase.p2, testCase.expected, actual)
		}
	}
}

func TestUnitIntersect(t *testing.T) {
	r1 := Range{Position{0, 0}, Position{0, 10}}
	r2 := Range{Position{0, 5}, Position{1, 5}}
	r3 := Range{Position{1, 0}, Position{1, 10}}

	require.Truef(t, r1.Intersects(r2), "%+v.Intersects(%+v)", r1, r2)
	require.Truef(t, r2.Intersects(r3), "%+v.Intersects(%+v)", r2, r3)
	require.Falsef(t, r1.Intersects(r3), "%+v.Intersects(%+v)", r1, r3)
}

func genPosition() *rapid.Generator[Position] {
	return rapid.Custom(func(t *rapid.T) Position {
		return Position{
			Line:      rapid.Int32Range(0, 10).Draw(t, "Line"),
			Character: rapid.Int32Range(0, 10).Draw(t, "Character"),
		}
	})
}

func genRange() *rapid.Generator[Range] {
	return rapid.Custom(func(t *rapid.T) Range {
		posGen := genPosition()
		start := posGen.Draw(t, "start")
		end := posGen.Draw(t, "end")
		// A well formed range is always start <= end.
		if start.Compare(end) > 0 {
			start, end = end, start
		}
		return Range{
			Start: start,
			End:   end,
		}
	})
}

func TestComparePositionTransitive(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p1 := genPosition().Draw(t, "p1")
		p2 := genPosition().Draw(t, "p2")
		p3 := genPosition().Draw(t, "p3")

		if p1.Compare(p2) < 0 && p2.Compare(p3) < 0 {
			if !(p1.Compare(p3) < 0) {
				t.Errorf("%+v < %+v < %+v but !(%+v < %+v)", p1, p2, p3, p1, p3)
			}
		}
	})
}

func TestIntersects(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		r1 := genRange().Draw(t, "r1")
		r2 := genRange().Draw(t, "r2")

		if r1.Intersects(r2) {
			if !(r1.Contains(r2.Start) || r2.Contains(r1.Start)) {
				t.Errorf("%+v overlaps with %+v but neither contains the others start position", r1, r2)
			}
		}
	})
}

func TestLessCompareConsistent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		r1 := genRange().Draw(t, "r1")
		r2 := genRange().Draw(t, "r2")
		if r1.Compare(r2) < 0 {
			if !(r1.Less(r2)) {
				t.Errorf("%+v.Compare(%+v) < 0 but !(%+v < %+v)", r1, r2, r1, r2)
			}
		} else {
			if r1.Less(r2) {
				t.Errorf("%+v.Compare(%+v) >= 0 but  (%+v < %+v)", r1, r2, r1, r2)
			}
		}
	})

	rapid.Check(t, func(t *rapid.T) {
		r1 := genRange().Draw(t, "r1")
		r2 := genRange().Draw(t, "r2")
		if r1.CompareStrict(r2) < 0 {
			if !(r1.LessStrict(r2)) {
				t.Errorf("%+v.CompareStrict(%+v) < 0 but !(%+v < %+v)", r1, r2, r1, r2)
			}
		} else {
			if r1.LessStrict(r2) {
				t.Errorf("%+v.CompareStrict(%+v) >= 0 but  (%+v < %+v)", r1, r2, r1, r2)
			}
		}
	})
}

func TestCompareReverse(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		r1 := genRange().Draw(t, "r1")
		r2 := genRange().Draw(t, "r2")
		cmp1 := r1.Compare(r2)
		cmp2 := r2.Compare(r1)
		if cmp1 != -cmp2 {
			t.Errorf("%+v.Compare(%+v) = %d but %+v.Compare(%+v) = %d", r1, r2, cmp1, r2, r1, cmp2)
		}
	})
}

func TestNewRange_UnitTests(t *testing.T) {
	type result struct {
		range_ Range
		err    string
	}
	type testCase struct {
		input    []int32
		expected result
	}
	testCases := []testCase{
		{
			input:    nil,
			expected: result{err: "incorrect length"},
		},
		{
			input:    []int32{3},
			expected: result{err: "incorrect length"},
		},
		{
			input:    []int32{3, 3},
			expected: result{err: "incorrect length"},
		},
		{
			input: []int32{12, 0, 14},
			expected: result{range_: Range{
				Start: Position{Line: 12},
				End: Position{
					Line:      12,
					Character: 14,
				},
			}},
		},
		{
			input:    []int32{12, 14, 0},
			expected: result{err: "end before start"},
		},
		{
			input:    []int32{12, 14, 0},
			expected: result{err: "end before start"},
		},
		{
			input: []int32{12, 15, 14, 0},
			expected: result{range_: Range{
				Start: Position{
					Line:      12,
					Character: 15,
				},
				End: Position{Line: 14},
			}},
		},
		{
			input: []int32{12, 0, 12, 0},
			expected: result{range_: Range{
				Start: Position{Line: 12},
				End:   Position{Line: 12},
			}},
		},
		{
			input:    []int32{12, 0, 13, 1, 4},
			expected: result{err: "incorrect length"},
		},
		{
			input:    []int32{3, -1, 4},
			expected: result{err: "negative offsets"},
		},
	}

	for _, tc := range testCases {
		require.NotPanicsf(t, func() {
			r, err := NewRange(tc.input)
			errStr := ""
			if err != nil {
				errStr = err.Error()
			}
			require.Equal(t, tc.expected, result{range_: r, err: errStr})
		}, "panicked for input: %v", tc.input)
	}
}

func TestNewRange(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.SliceOfN(rapid.Int32Range(-2, 10), 0, 5).Draw(t, "input")
		var r1 Range
		var err error
		require.NotPanics(t, func() {
			r1, err = NewRange(input)
		})
		if err != nil {
			return
		}
		r2 := NewRangeUnchecked(input)
		if r1.CompareStrict(r2) != 0 {
			t.Fatalf("NewRange vs NewRangeUnchecked: %s %s", r1, r2)
		}
	})
}

func TestRange_Validate(t *testing.T) {
	require.NoError(t, Range{Start: Position{0, 0}, End: Position{0, 0}}.Validate())
	require.NoError(t, Range{Start: Position{1, 2}, End: Position{1, 2}}.Validate())
	require.NoError(t, Range{Start: Position{1, 2}, End: Position{3, 4}}.Validate())

	require.ErrorIs(t, Range{Start: Position{-1, 0}, End: Position{0, 0}}.Validate(), NegativeOffsetsRangeError)
	require.ErrorIs(t, Range{Start: Position{0, 0}, End: Position{0, -1}}.Validate(), NegativeOffsetsRangeError)

	require.ErrorIs(t, Range{Start: Position{3, 0}, End: Position{1, 0}}.Validate(), EndBeforeStartRangeError)
	require.ErrorIs(t, Range{Start: Position{1, 5}, End: Position{1, 2}}.Validate(), EndBeforeStartRangeError)
}
