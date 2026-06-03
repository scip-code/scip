package scip

import "strings"

// SourceRange returns the source range of this occurrence and whether one is
// set. `typed_range` takes precedence over the deprecated `range` field.
// Malformed deprecated ranges (length != 3 or 4) are reported as missing; use
// `scip lint` to surface them.
func (occ *Occurrence) SourceRange() (Range, bool) {
	if r, ok := readTypedRange(occ.GetTypedRange()); ok {
		return r, true
	}
	if len(occ.Range) != 3 && len(occ.Range) != 4 {
		return Range{}, false
	}
	return NewRangeUnchecked(occ.Range), true
}

// EnclosingSourceRange returns the enclosing source range of this occurrence
// and whether one is set. `typed_enclosing_range` takes precedence over the
// deprecated `enclosing_range` field.
func (occ *Occurrence) EnclosingSourceRange() (Range, bool) {
	if r, ok := readTypedRange(occ.GetTypedEnclosingRange()); ok {
		return r, true
	}
	if len(occ.EnclosingRange) != 3 && len(occ.EnclosingRange) != 4 {
		return Range{}, false
	}
	return NewRangeUnchecked(occ.EnclosingRange), true
}

// SetSourceRange sets the source range using the typed `typed_range` encoding
// (SingleLineRange when r fits on one line, MultiLineRange otherwise) and
// clears the deprecated `range` field.
func (occ *Occurrence) SetSourceRange(r Range) {
	occ.Range = nil
	if r.IsSingleLine() {
		occ.TypedRange = &Occurrence_SingleLineRange{SingleLineRange: r.ToSingleLineRange()}
		return
	}
	occ.TypedRange = &Occurrence_MultiLineRange{MultiLineRange: r.ToMultiLineRange()}
}

// SetEnclosingSourceRange is the counterpart of SetSourceRange for the
// enclosing range.
func (occ *Occurrence) SetEnclosingSourceRange(r Range) {
	occ.EnclosingRange = nil
	if r.IsSingleLine() {
		occ.TypedEnclosingRange = &Occurrence_SingleLineEnclosingRange{SingleLineEnclosingRange: r.ToSingleLineRange()}
		return
	}
	occ.TypedEnclosingRange = &Occurrence_MultiLineEnclosingRange{MultiLineEnclosingRange: r.ToMultiLineRange()}
}

// Compare orders occurrences in the canonical SCIP ordering: ascending by
// source range, with symbol name as a tiebreaker. Returns -1, 0, or +1.
//
// Occurrences missing a source range compare as if their range were the
// zero range; such occurrences are illegal per the SCIP spec and should be
// surfaced via `scip lint`.
func (occ *Occurrence) Compare(other *Occurrence) int {
	r1, _ := occ.SourceRange()
	r2, _ := other.SourceRange()
	if c := r1.CompareStrict(r2); c != 0 {
		return c
	}
	return strings.Compare(occ.Symbol, other.Symbol)
}

// Contains reports whether the source range of this occurrence contains the
// given position. Returns false if the occurrence has no range.
func (occ *Occurrence) Contains(pos Position) bool {
	r, ok := occ.SourceRange()
	return ok && r.Contains(pos)
}

// ToRange returns this single-line range as a Range.
func (sr *SingleLineRange) ToRange() Range {
	return Range{
		Start: Position{Line: sr.Line, Character: sr.StartCharacter},
		End:   Position{Line: sr.Line, Character: sr.EndCharacter},
	}
}

// ToRange returns this multi-line range as a Range.
func (mr *MultiLineRange) ToRange() Range {
	return Range{
		Start: Position{Line: mr.StartLine, Character: mr.StartCharacter},
		End:   Position{Line: mr.EndLine, Character: mr.EndCharacter},
	}
}

// readTypedRange decodes any of the four typed-range oneof variants on
// Occurrence into a Range. Returns (Range{}, false) when typed is nil.
func readTypedRange(typed any) (Range, bool) {
	switch tr := typed.(type) {
	case *Occurrence_SingleLineRange:
		return tr.SingleLineRange.ToRange(), true
	case *Occurrence_MultiLineRange:
		return tr.MultiLineRange.ToRange(), true
	case *Occurrence_SingleLineEnclosingRange:
		return tr.SingleLineEnclosingRange.ToRange(), true
	case *Occurrence_MultiLineEnclosingRange:
		return tr.MultiLineEnclosingRange.ToRange(), true
	}
	return Range{}, false
}
