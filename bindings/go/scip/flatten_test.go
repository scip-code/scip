package scip

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFlattenDocuments(t *testing.T) {
	s1 := []*SymbolInformation{{Symbol: "foo"}, {Symbol: "bar"}}
	o1 := []*Occurrence{{Range: []int32{1, 2, 3}, Symbol: "foo"}, {Range: []int32{2, 3, 4}, Symbol: "bar"}}
	s2 := []*SymbolInformation{{Symbol: "baz"}, {Symbol: "bonk"}}
	o2 := []*Occurrence{{Range: []int32{3, 4, 5}, Symbol: "baz"}, {Range: []int32{4, 5, 6}, Symbol: "bonk"}}
	s3 := []*SymbolInformation{{Symbol: "quux"}}
	o3 := []*Occurrence{{Range: []int32{5, 6, 7}, Symbol: "quux"}}

	documents := []*Document{
		{RelativePath: "foo.go", Symbols: s1, Occurrences: o1},
		{RelativePath: "bar.go", Symbols: s2, Occurrences: o2},
		{RelativePath: "foo.go", Symbols: s3, Occurrences: o3},
	}
	flattenedDocuments := SortDocuments(FlattenDocuments(documents))

	expectedDocuments := []*Document{
		{RelativePath: "bar.go", Symbols: s2, Occurrences: o2},
		{RelativePath: "foo.go", Symbols: append(s1, s3...), Occurrences: append(o1, o3...)},
	}
	if diff := cmp.Diff(expectedDocuments, flattenedDocuments, compareDocuments); diff != "" {
		t.Fatalf("unexpected documents (-want +got):\n%s", diff)
	}
}

var compareDocuments = cmp.Comparer(func(a, b *Document) bool {
	if a.Language != b.Language || a.RelativePath != b.RelativePath || a.Text != b.Text {
		return false
	}
	if !compareSlices(a.Symbols, b.Symbols, compareSymbol) {
		return false
	}
	if !compareSlices(a.Occurrences, b.Occurrences, compareOccurrence) {
		return false
	}

	return true
})

func compareSlices[T any](as, bs []T, f func(a, b T) bool) bool {
	if len(as) != len(bs) {
		return false
	}

	for i, a := range as {
		if !f(a, bs[i]) {
			return false
		}
	}

	return true
}

func compareSymbol(a, b *SymbolInformation) bool {
	if a.Symbol != b.Symbol {
		return false
	}
	if a.Kind != b.Kind {
		return false
	}
	if a.Visibility != b.Visibility {
		return false
	}
	if a.DisplayName != b.DisplayName {
		return false
	}
	if a.SignatureDocumentation != b.SignatureDocumentation {
		return false
	}
	if a.EnclosingSymbol != b.EnclosingSymbol {
		return false
	}

	if !compareSlices(a.Documentation, b.Documentation, compareValue[string]) {
		return false
	}
	if !compareSlices(a.Relationships, b.Relationships, compareRelationship) {
		return false
	}

	return true
}

func compareOccurrence(a, b *Occurrence) bool {
	if a.Symbol != b.Symbol {
		return false
	}
	if a.SymbolRoles != b.SymbolRoles {
		return false
	}
	if a.SyntaxKind != b.SyntaxKind {
		return false
	}

	if !compareSlices(a.Range, b.Range, compareValue[int32]) {
		return false
	}
	if !compareSlices(a.OverrideDocumentation, b.OverrideDocumentation, compareValue[string]) {
		return false
	}
	if !compareSlices(a.Diagnostics, b.Diagnostics, compareDiagnostic) {
		return false
	}
	if !compareSlices(a.EnclosingRange, b.EnclosingRange, compareValue[int32]) {
		return false
	}

	return true
}

func compareRelationship(a, b *Relationship) bool {
	if a.Symbol != b.Symbol {
		return false
	}
	if a.IsReference != b.IsReference {
		return false
	}
	if a.IsImplementation != b.IsImplementation {
		return false
	}
	if a.IsTypeDefinition != b.IsTypeDefinition {
		return false
	}
	if a.IsDefinition != b.IsDefinition {
		return false
	}

	return true
}

func compareDiagnostic(a, b *Diagnostic) bool {
	if a.Severity != b.Severity {
		return false
	}
	if a.Code != b.Code {
		return false
	}
	if a.Message != b.Message {
		return false
	}
	if a.Source != b.Source {
		return false
	}
	if !compareSlices(a.Tags, b.Tags, compareValue[DiagnosticTag]) {
		return false
	}

	return true
}

func compareValue[T comparable](a, b T) bool {
	return a == b
}
