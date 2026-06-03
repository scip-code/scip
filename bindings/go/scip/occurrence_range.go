package scip

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
		occ.TypedRange = &Occurrence_SingleLineRange{SingleLineRange: rangeToSingleLine(r)}
		return
	}
	occ.TypedRange = &Occurrence_MultiLineRange{MultiLineRange: rangeToMultiLine(r)}
}

// SetEnclosingSourceRange is the counterpart of SetSourceRange for the
// enclosing range.
func (occ *Occurrence) SetEnclosingSourceRange(r Range) {
	occ.EnclosingRange = nil
	if r.IsSingleLine() {
		occ.TypedEnclosingRange = &Occurrence_SingleLineEnclosingRange{SingleLineEnclosingRange: rangeToSingleLine(r)}
		return
	}
	occ.TypedEnclosingRange = &Occurrence_MultiLineEnclosingRange{MultiLineEnclosingRange: rangeToMultiLine(r)}
}

// readTypedRange decodes any of the four typed-range oneof variants on
// Occurrence into a Range. Returns (Range{}, false) when typed is nil.
func readTypedRange(typed any) (Range, bool) {
	switch tr := typed.(type) {
	case *Occurrence_SingleLineRange:
		return singleLineToRange(tr.SingleLineRange), true
	case *Occurrence_MultiLineRange:
		return multiLineToRange(tr.MultiLineRange), true
	case *Occurrence_SingleLineEnclosingRange:
		return singleLineToRange(tr.SingleLineEnclosingRange), true
	case *Occurrence_MultiLineEnclosingRange:
		return multiLineToRange(tr.MultiLineEnclosingRange), true
	}
	return Range{}, false
}

func singleLineToRange(sr *SingleLineRange) Range {
	return Range{
		Start: Position{Line: sr.Line, Character: sr.StartCharacter},
		End:   Position{Line: sr.Line, Character: sr.EndCharacter},
	}
}

func multiLineToRange(mr *MultiLineRange) Range {
	return Range{
		Start: Position{Line: mr.StartLine, Character: mr.StartCharacter},
		End:   Position{Line: mr.EndLine, Character: mr.EndCharacter},
	}
}

func rangeToSingleLine(r Range) *SingleLineRange {
	return &SingleLineRange{
		Line:           r.Start.Line,
		StartCharacter: r.Start.Character,
		EndCharacter:   r.End.Character,
	}
}

func rangeToMultiLine(r Range) *MultiLineRange {
	return &MultiLineRange{
		StartLine:      r.Start.Line,
		StartCharacter: r.Start.Character,
		EndLine:        r.End.Line,
		EndCharacter:   r.End.Character,
	}
}
