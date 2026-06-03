package scip

// CanonicalizeDocument deterministically sorts and merges fields of the given document.
//
// Post-conditions:
//  1. The Occurrences field only contains those with well-formed ranges.
//  2. The Occurrences field is sorted in ascending order of ranges based on
//     Range.CompareStrict
//  3. The Symbols field is sorted in ascending order based on the symbol name,
//     and SymbolInformation values for the same name will have been merged.
func CanonicalizeDocument(document *Document) *Document {
	document.Occurrences = CanonicalizeOccurrences(document.Occurrences)
	document.Symbols = CanonicalizeSymbols(document.Symbols)
	return SanitizeDocument(document)
}

// CanonicalizeOccurrences deterministically re-orders the fields of the given occurrence slice.
func CanonicalizeOccurrences(occurrences []*Occurrence) []*Occurrence {
	canonicalized := make([]*Occurrence, 0, len(occurrences))
	for _, occurrence := range FlattenOccurrences(RemoveIllegalOccurrences(occurrences)) {
		canonicalized = append(canonicalized, CanonicalizeOccurrence(occurrence))
	}

	return SortOccurrences(canonicalized)
}

// RemoveIllegalOccurrences removes all occurrences that do not include a range. This is
// emitted by some indexers and will silently crash a downstream process, including further
// canonicalization, when trying to convert an empty slice into a valid range.
func RemoveIllegalOccurrences(occurrences []*Occurrence) []*Occurrence {
	filtered := occurrences[:0]
	for _, occurrence := range occurrences {
		if _, ok := OccurrenceRange(occurrence); !ok {
			continue
		}

		filtered = append(filtered, occurrence)
	}

	return filtered
}

// CanonicalizeOccurrence deterministically re-orders the fields of the given occurrence.
//
// It normalizes the occurrence's range encoding so that occurrences carrying
// only the deprecated `repeated int32 range` field are re-emitted with the
// most compact three-element form. Occurrences that carry a typed range are
// left unchanged.
func CanonicalizeOccurrence(occurrence *Occurrence) *Occurrence {
	if occurrence.GetTypedRange() == nil && len(occurrence.Range) > 0 {
		// Express ranges as three-components if possible
		occurrence.Range = NewRangeUnchecked(occurrence.Range).SCIPRange()
	}
	occurrence.Diagnostics = CanonicalizeDiagnostics(occurrence.Diagnostics)
	return occurrence
}

// CanonicalizeDiagnostics deterministically re-orders the fields of the given diagnostic slice.
func CanonicalizeDiagnostics(diagnostics []*Diagnostic) []*Diagnostic {
	canonicalized := make([]*Diagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		canonicalized = append(canonicalized, CanonicalizeDiagnostic(diagnostic))
	}

	return SortDiagnostics(canonicalized)
}

// CanonicalizeDiagnostic deterministically re-orders the fields of the given diagnostic.
func CanonicalizeDiagnostic(diagnostic *Diagnostic) *Diagnostic {
	diagnostic.Tags = SortDiagnosticTags(diagnostic.Tags)
	return diagnostic
}

// CanonicalizeSymbols deterministically re-orders the fields of the given symbols slice.
func CanonicalizeSymbols(symbols []*SymbolInformation) []*SymbolInformation {
	canonicalized := make([]*SymbolInformation, 0, len(symbols))
	for _, symbol := range FlattenSymbols(symbols) {
		canonicalized = append(canonicalized, CanonicalizeSymbol(symbol))
	}

	return SortSymbols(canonicalized)
}

// CanonicalizeSymbol deterministically re-orders the fields of the given symbol.
func CanonicalizeSymbol(symbol *SymbolInformation) *SymbolInformation {
	symbol.Relationships = CanonicalizeRelationships(symbol.Relationships)
	return symbol
}

// CanonicalizeRelationships deterministically re-orders the fields of the given relationship slice.
func CanonicalizeRelationships(relationships []*Relationship) []*Relationship {
	return SortRelationships(FlattenRelationship(relationships))
}
