package scip

import (
	"errors"
	"fmt"
)

// Range represents [start, end) between two offset positions.
type Range struct {
	Start Position
	End   Position
}

// Position represents an offset position.
type Position struct {
	Line      int32
	Character int32
}

func (p Position) Compare(other Position) int {
	if p.Line < other.Line {
		return -1
	}
	if p.Line > other.Line {
		return 1
	}
	if p.Character < other.Character {
		return -1
	}
	if p.Character > other.Character {
		return 1
	}
	return 0
}

func (p Position) Less(other Position) bool {
	if p.Line < other.Line {
		return true
	}
	if p.Line > other.Line {
		return false
	}
	return p.Character < other.Character
}

//go:noinline
func makeNewRangeError(startLine, endLine, startChar, endChar int32) (Range, error) {
	if startLine < 0 || endLine < 0 || startChar < 0 || endChar < 0 {
		return Range{}, NegativeOffsetsRangeError
	}
	if startLine > endLine || (startLine == endLine && startChar > endChar) {
		return Range{}, EndBeforeStartRangeError
	}
	panic("unreachable")
}

// NewRange constructs a Range while checking if the input is valid.
func NewRange(scipRange []int32) (Range, error) {
	// N.B. This function is kept small so that it can be inlined easily.
	// See also: https://github.com/golang/go/issues/17566
	var startLine, endLine, startChar, endChar int32
	switch len(scipRange) {
	case 3:
		startLine = scipRange[0]
		endLine = startLine
		startChar = scipRange[1]
		endChar = scipRange[2]
		if startLine >= 0 && startChar >= 0 && endChar >= startChar {
			break
		}
		return makeNewRangeError(startLine, endLine, startChar, endChar)
	case 4:
		startLine = scipRange[0]
		startChar = scipRange[1]
		endLine = scipRange[2]
		endChar = scipRange[3]
		if startLine >= 0 && startChar >= 0 &&
			((endLine > startLine && endChar >= 0) || (endLine == startLine && endChar >= startChar)) {
			break
		}
		return makeNewRangeError(startLine, endLine, startChar, endChar)
	default:
		return Range{}, IncorrectLengthRangeError
	}
	return Range{Start: Position{Line: startLine, Character: startChar}, End: Position{Line: endLine, Character: endChar}}, nil
}

type RangeError int32

const (
	IncorrectLengthRangeError RangeError = iota
	NegativeOffsetsRangeError
	EndBeforeStartRangeError
)

var _ error = RangeError(0)

func (e RangeError) Error() string {
	switch e {
	case IncorrectLengthRangeError:
		return "incorrect length"
	case NegativeOffsetsRangeError:
		return "negative offsets"
	case EndBeforeStartRangeError:
		return "end before start"
	}
	panic("unhandled range error")
}

// NewRangeUnchecked converts an SCIP range into `Range`
//
// Pre-condition: The input slice must follow the SCIP range encoding.
// https://sourcegraph.com/github.com/sourcegraph/scip/-/blob/scip.proto?L646:18-646:23
func NewRangeUnchecked(scipRange []int32) Range {
	// Single-line case is most common
	endCharacter := scipRange[2]
	endLine := scipRange[0]
	if len(scipRange) == 4 { // multi-line
		endCharacter = scipRange[3]
		endLine = scipRange[2]
	}
	return Range{
		Start: Position{
			Line:      scipRange[0],
			Character: scipRange[1],
		},
		End: Position{
			Line:      endLine,
			Character: endCharacter,
		},
	}
}

func (r Range) IsSingleLine() bool {
	return r.Start.Line == r.End.Line
}

func (r Range) SCIPRange() []int32 {
	if r.Start.Line == r.End.Line {
		return []int32{r.Start.Line, r.Start.Character, r.End.Character}
	}
	return []int32{r.Start.Line, r.Start.Character, r.End.Line, r.End.Character}
}

// Contains checks if position is within the range
func (r Range) Contains(position Position) bool {
	return !position.Less(r.Start) && position.Less(r.End)
}

// Intersects checks if two ranges intersect.
//
// case 1: r1.Start >= other.Start && r1.Start < other.End
// case 2: r2.Start >= r1.Start && r2.Start < r1.End
func (r Range) Intersects(other Range) bool {
	return r.Start.Less(other.End) && other.Start.Less(r.End)
}

// Compare compares two ranges.
//
// Returns 0 if the ranges intersect (not just if they're equal).
func (r Range) Compare(other Range) int {
	if r.Intersects(other) {
		return 0
	}
	return r.Start.Compare(other.Start)
}

// Less compares two ranges, consistent with Compare.
//
// r.Compare(other) < 0 iff r.Less(other).
func (r Range) Less(other Range) bool {
	if r.Intersects(other) {
		return false
	}
	return r.Start.Less(other.Start)
}

// CompareStrict compares two ranges.
//
// Returns 0 iff the ranges are exactly equal.
func (r Range) CompareStrict(other Range) int {
	if ret := r.Start.Compare(other.Start); ret != 0 {
		return ret
	}
	return r.End.Compare(other.End)
}

// LessStrict compares two ranges, consistent with CompareStrict.
//
// r.CompareStrict(other) < 0 iff r.LessStrict(other).
func (r Range) LessStrict(other Range) bool {
	if ret := r.Start.Compare(other.Start); ret != 0 {
		return ret < 0
	}
	return r.End.Less(other.End)
}

func (r Range) String() string {
	return fmt.Sprintf("%d:%d-%d:%d", r.Start.Line, r.Start.Character, r.End.Line, r.End.Character)
}

// ToSingleLineRange returns this range as a SingleLineRange.
//
// Pre-condition: r.IsSingleLine() must be true.
func (r Range) ToSingleLineRange() *SingleLineRange {
	return &SingleLineRange{
		Line:           r.Start.Line,
		StartCharacter: r.Start.Character,
		EndCharacter:   r.End.Character,
	}
}

// ToMultiLineRange returns this range as a MultiLineRange.
func (r Range) ToMultiLineRange() *MultiLineRange {
	return &MultiLineRange{
		StartLine:      r.Start.Line,
		StartCharacter: r.Start.Character,
		EndLine:        r.End.Line,
		EndCharacter:   r.End.Character,
	}
}

// rangeFromSingleLine constructs a Range from a SingleLineRange without
// validation. Used by Occurrence helpers where the typed message has already
// been produced by a proto decoder.
func rangeFromSingleLine(sr *SingleLineRange) Range {
	return Range{
		Start: Position{Line: sr.Line, Character: sr.StartCharacter},
		End:   Position{Line: sr.Line, Character: sr.EndCharacter},
	}
}

// rangeFromMultiLine constructs a Range from a MultiLineRange without
// validation.
func rangeFromMultiLine(mr *MultiLineRange) Range {
	return Range{
		Start: Position{Line: mr.StartLine, Character: mr.StartCharacter},
		End:   Position{Line: mr.EndLine, Character: mr.EndCharacter},
	}
}

// ErrMissingRange is returned by OccurrenceRange when an occurrence has
// neither typed_range nor the deprecated repeated-int32 range field set.
var ErrMissingRange = errors.New("occurrence has no range")

// OccurrenceRange returns the source range of an occurrence. It reads
// `typed_range` if present (taking precedence per the schema), and otherwise
// falls back to the deprecated `repeated int32 range` field.
//
// Returns ErrMissingRange if neither encoding is set, or a RangeError if the
// deprecated field is present but malformed.
func OccurrenceRange(occ *Occurrence) (Range, error) {
	switch tr := occ.GetTypedRange().(type) {
	case *Occurrence_SingleLineRange:
		return rangeFromSingleLine(tr.SingleLineRange), nil
	case *Occurrence_MultiLineRange:
		return rangeFromMultiLine(tr.MultiLineRange), nil
	}
	if len(occ.Range) == 0 {
		return Range{}, ErrMissingRange
	}
	return NewRange(occ.Range)
}

// OccurrenceRangeUnchecked returns the source range of an occurrence without
// validating the input. Like OccurrenceRange, `typed_range` takes precedence
// over the deprecated `range` field.
//
// Pre-condition: the occurrence must carry a range in one of the two
// encodings. Calling this on an occurrence with no range will return a
// zero-valued Range.
func OccurrenceRangeUnchecked(occ *Occurrence) Range {
	switch tr := occ.GetTypedRange().(type) {
	case *Occurrence_SingleLineRange:
		return rangeFromSingleLine(tr.SingleLineRange)
	case *Occurrence_MultiLineRange:
		return rangeFromMultiLine(tr.MultiLineRange)
	}
	if len(occ.Range) == 0 {
		return Range{}
	}
	return NewRangeUnchecked(occ.Range)
}

// OccurrenceEnclosingRange returns the enclosing range of an occurrence, if
// any. Returns (zero, false, nil) when no enclosing range is set, (range,
// true, nil) on success, and (zero, true, err) when a deprecated
// repeated-int32 enclosing range is present but malformed.
//
// `typed_enclosing_range` takes precedence over the deprecated
// `enclosing_range` field.
func OccurrenceEnclosingRange(occ *Occurrence) (Range, bool, error) {
	switch tr := occ.GetTypedEnclosingRange().(type) {
	case *Occurrence_SingleLineEnclosingRange:
		return rangeFromSingleLine(tr.SingleLineEnclosingRange), true, nil
	case *Occurrence_MultiLineEnclosingRange:
		return rangeFromMultiLine(tr.MultiLineEnclosingRange), true, nil
	}
	if len(occ.EnclosingRange) == 0 {
		return Range{}, false, nil
	}
	r, err := NewRange(occ.EnclosingRange)
	if err != nil {
		return Range{}, true, err
	}
	return r, true, nil
}

// HasOccurrenceRange reports whether an occurrence carries a source range in
// either encoding.
func HasOccurrenceRange(occ *Occurrence) bool {
	if occ.GetTypedRange() != nil {
		return true
	}
	return len(occ.Range) == 3 || len(occ.Range) == 4
}

// SetOccurrenceRange stores r on occ using the typed `typed_range` encoding,
// choosing SingleLineRange when r is single-line and MultiLineRange
// otherwise. The deprecated `range` field is cleared.
func SetOccurrenceRange(occ *Occurrence, r Range) {
	occ.Range = nil
	if r.IsSingleLine() {
		occ.TypedRange = &Occurrence_SingleLineRange{SingleLineRange: r.ToSingleLineRange()}
		return
	}
	occ.TypedRange = &Occurrence_MultiLineRange{MultiLineRange: r.ToMultiLineRange()}
}

// SetOccurrenceEnclosingRange stores r on occ using the typed
// `typed_enclosing_range` encoding. The deprecated `enclosing_range` field is
// cleared.
func SetOccurrenceEnclosingRange(occ *Occurrence, r Range) {
	occ.EnclosingRange = nil
	if r.IsSingleLine() {
		occ.TypedEnclosingRange = &Occurrence_SingleLineEnclosingRange{SingleLineEnclosingRange: r.ToSingleLineRange()}
		return
	}
	occ.TypedEnclosingRange = &Occurrence_MultiLineEnclosingRange{MultiLineEnclosingRange: r.ToMultiLineRange()}
}
