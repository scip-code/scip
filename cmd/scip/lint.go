package main

import (
	"context"
	stderrors "errors"
	"fmt"
	"sort"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/scip-code/scip/bindings/go/scip"
)

func lintCommand() cli.Command {
	snapshot := cli.Command{
		Name:  "lint",
		Usage: "Flag potential issues with a SCIP index",
		Description: "Example usage:\n\n  scip lint /path/to/index.scip\n\n" +
			"You may want to filter the output using `grep -v <pattern>`\n" +
			"to narrow down on certain classes of errors.",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			indexPath := cmd.Args().Get(0)
			if indexPath == "" {
				return stderrors.New("missing argument for path to SCIP index")
			}
			return lintMain(indexPath)
		},
	}
	return snapshot
}

func lintMain(indexPath string) error {
	scipIndex, err := readFromOption(indexPath)
	if err != nil {
		return err
	}
	return stderrors.Join(lintMainPure(scipIndex).data...)
}

func lintMainPure(scipIndex *scip.Index) errorSet {
	symTable := newSymbolTable()

	var errs errorSet

	for _, extSym := range scipIndex.ExternalSymbols {
		if extSym.Symbol == "" {
			errs.Add(emptyStringError{what: "symbol", context: "external symbols"})
		} else if _, ok := symTable.extSyms[extSym.Symbol]; ok {
			errs.Add(duplicateSymbolInfoWarning{symbol: extSym.Symbol})
		} else {
			symTable.extSyms[extSym.Symbol] = fileOccurrenceMap{}
		}
	}

	docMap := map[string]*scip.Document{}
	for _, doc := range scipIndex.Documents {
		if doc.RelativePath == "" {
			errs.Add(emptyStringError{what: "document", context: "index"})
			errs.Add(note{"empty documents will be skipped for diagnostics"})
			continue
		}
		if _, ok := docMap[doc.RelativePath]; ok {
			errs.Add(duplicateDocumentWarning{path: doc.RelativePath})
			errs.Add(note{"out of documents with identical paths, all but one will be skipped for diagnostics"})
		} else {
			docMap[doc.RelativePath] = doc
		}
	}

	for path, doc := range docMap {
		for _, sym := range doc.Symbols {
			if err := symTable.addFileForSymbol(sym.Symbol, path); err != nil {
				errs.Add(err)
			}
		}
		for _, sym := range doc.Symbols {
			for _, rel := range sym.Relationships {
				// This has to run in a second pass so that all symbols are first populated.
				if err := symTable.addRelationship(sym.Symbol, path, rel); err != nil {
					errs.Add(err)
				}
			}
		}
	}

	for _, extSym := range scipIndex.ExternalSymbols {
		for _, rel := range extSym.Relationships {
			if err := symTable.addRelationshipForExternalSymbol(extSym.Symbol, rel); err != nil {
				errs.Add(err)
			}
		}
	}

	for path, doc := range docMap {
		for _, occ := range doc.Occurrences {
			if err := symTable.addOccurrence(path, occ); err != nil {
				errs.Add(err)
			}
		}
	}

	return errs
}

// --- Main types ---

type occurrenceKey struct {
	loc   scip.Range
	roles int32
}

func scipOccurrenceKey(occ *scip.Occurrence) occurrenceKey {
	return occurrenceKey{scip.NewRangeUnchecked(occ.Range), occ.SymbolRoles}
}

type occurrenceMap = map[occurrenceKey]*scip.Occurrence

type fileOccurrenceMap = map[string]occurrenceMap

type symbolTable struct {
	extSyms      map[string]fileOccurrenceMap
	localSymsMap map[string]fileOccurrenceMap
	relsMap      set[pair[string]]
}

func newSymbolTable() symbolTable {
	return symbolTable{
		map[string]fileOccurrenceMap{},
		map[string]fileOccurrenceMap{},
		newSet[pair[string]](),
	}
}

func (st *symbolTable) addFileForSymbol(sym string, path string) error {
	if sym == "" {
		return emptyStringError{what: "symbol", context: fmt.Sprintf("symbols for document %s", path)}
	}
	if _, ok := st.extSyms[sym]; ok {
		return bothLocalAndExternalSymbolError{sym, path}
	}
	if occsForSym, ok := st.localSymsMap[sym]; ok {
		if _, ok := occsForSym[path]; ok {
			return duplicateSymbolInfoWarning{symbol: sym, path: path}
		} else {
			occsForSym[path] = occurrenceMap{}
		}
	} else {
		st.localSymsMap[sym] = fileOccurrenceMap{path: {}}
	}
	return nil
}

func (st *symbolTable) addRelationshipForExternalSymbol(sym string, rel *scip.Relationship) error {
	if sym == "" { // Errors for this are emitted earlier
		return nil
	}
	if rel.Symbol == "" {
		return emptyStringError{what: "symbol", context: fmt.Sprintf("relationships for external symbol %s", sym)}
	}
	if !rel.IsDefinition && !rel.IsReference && !rel.IsImplementation && !rel.IsTypeDefinition {
		return missingRelationshipFlagError{sym, "external symbols"}
	}
	if _, ok := st.extSyms[rel.Symbol]; !ok {
		return missingSymbolInRelationshipError{sym, "external symbols", rel.Symbol, "external symbols"}
	}
	relPair := pair[string]{sym, rel.Symbol}
	if st.relsMap.Contains(relPair) {
		return multipleRelationshipWarning{sym, rel.Symbol}
	}
	st.relsMap.Add(relPair)
	return nil
}

func (st *symbolTable) addRelationship(sym string, path string, rel *scip.Relationship) error {
	if sym == "" { // Errors for this are emitted earlier
		return nil
	}
	if rel.Symbol == "" {
		return emptyStringError{what: "symbol", context: fmt.Sprintf("relationships for %s", sym)}
	}
	if !rel.IsDefinition && !rel.IsReference && !rel.IsImplementation && !rel.IsTypeDefinition {
		return missingRelationshipFlagError{sym, fmt.Sprintf("symbols for file %s", path)}
	}
	if _, ok := st.extSyms[rel.Symbol]; !ok {
		if _, ok := st.localSymsMap[rel.Symbol]; !ok {
			return missingSymbolInRelationshipError{
				sym, fmt.Sprintf("symbols for file %s", path),
				rel.Symbol, "external symbols or some other document",
			}
		}
	}
	relPair := pair[string]{sym, rel.Symbol}
	if st.relsMap.Contains(relPair) {
		return multipleRelationshipWarning{sym, rel.Symbol}
	}
	st.relsMap.Add(relPair)
	return nil
}

func (st *symbolTable) addOccurrence(path string, occ *scip.Occurrence) error {
	err := lintSymbolString(
		occ.Symbol,
		fmt.Sprintf("occurrence at %s @ %s", path, scipRangeToString(scip.NewRangeUnchecked(occ.Range))),
	)
	if err != nil {
		return err
	}
	if scip.SymbolRole_Definition.Matches(occ) && scip.SymbolRole_ForwardDefinition.Matches(occ) {
		return forwardDefIsDefinitionError{occ.Symbol, path, scip.NewRangeUnchecked(occ.Range)}
	}
	tryInsertOccurrence := func(occMap fileOccurrenceMap) error {
		occKey := scipOccurrenceKey(occ)
		if fileOccs, ok := occMap[path]; ok {
			if _, ok := fileOccs[occKey]; ok {
				return duplicateOccurrenceWarning{occ.Symbol, path, scip.NewRangeUnchecked(occ.Range), occ.SymbolRoles}
			} else {
				fileOccs[occKey] = occ
			}
		} else {
			// We're seeing an occurrence for a symbol in a file
			// other than the one it was defined in.
			occMap[path] = occurrenceMap{occKey: occ}
		}
		return nil
	}
	if occMap, ok := st.extSyms[occ.Symbol]; ok {
		if err := tryInsertOccurrence(occMap); err != nil {
			return err
		}
	} else if occMap, ok := st.localSymsMap[occ.Symbol]; ok {
		if err := tryInsertOccurrence(occMap); err != nil {
			return err
		}
	} else {
		return missingSymbolForOccurrenceError{occ.Symbol, path, scip.NewRangeUnchecked(occ.Range)}
	}
	return nil
}

// SkipLintSymbolParseTest is only set to true in the tests to allow using more succinct symbols
var SkipLintSymbolParseTest = false

func lintSymbolString(symbol string, context string) error {
	if symbol == "" {
		return emptyStringError{what: "symbol", context: context}
	}
	sym, err := scip.ParseSymbol(symbol)
	if err != nil {
		if SkipLintSymbolParseTest {
			return nil
		}
		return err
	}
	formatted := scip.VerboseSymbolFormatter.FormatSymbol(sym)
	if symbol != formatted {
		return nonCanonicalSymbolError{symbol, formatted, context}
	}
	return nil
}

// --- SCIP related utilities ---

func scipRangeToString(r scip.Range) string {
	return fmt.Sprintf("%d:%d-%d:%d", r.Start.Line, r.Start.Character, r.End.Line, r.End.Character)
}

// --- All possible errors ---

type duplicateSymbolInfoWarning struct {
	symbol string
	// path may be empty for external symbols
	path string
}

func (e duplicateSymbolInfoWarning) Error() string {
	if e.path == "" {
		return fmt.Sprintf("warning: found repeated SymbolInformation for '%s' in '%s'", e.symbol, e.path)
	}
	return fmt.Sprintf("warning: found repeated SymbolInformation for external symbol '%s'", e.symbol)
}

type emptyStringError struct {
	what    string
	context string
}

func (e emptyStringError) Error() string {
	return fmt.Sprintf("error: found empty %s in %s", e.what, e.context)
}

type nonCanonicalSymbolError struct {
	original  string
	canonical string
	context   string
}

func (e nonCanonicalSymbolError) Error() string {
	return fmt.Sprintf("error: found non-canonical symbol '%s' should be formatted as '%s'", e.original, e.canonical)
}

type duplicateDocumentWarning struct {
	path string
}

func (e duplicateDocumentWarning) Error() string {
	return fmt.Sprintf("warning: found multiple documents with path '%s' in index", e.path)
}

type bothLocalAndExternalSymbolError struct {
	symbol string
	path   string
}

func (e bothLocalAndExternalSymbolError) Error() string {
	return fmt.Sprintf("error: SymbolInformation for '%s'"+
		" is present in both external symbols and document '%s'", e.symbol, e.path)
}

type missingRelationshipFlagError struct {
	symbol  string
	context string
}

func (e missingRelationshipFlagError) Error() string {
	return fmt.Sprintf("error: at least one of is_definition,"+
		" is_reference, is_type_definition or is_implementation should"+
		" be set for symbol '%s' in %s", e.symbol, e.context)
}

type missingSymbolInRelationshipError struct {
	symbol                       string
	symbolContext                string
	relatedSymbol                string
	expectedRelatedSymbolContext string
}

func (e missingSymbolInRelationshipError) Error() string {
	return fmt.Sprintf("error: symbol '%s' (#1) (in %s) has"+
		" a relationship to '%s' (#2), but couldn't find #2 in %s",
		e.symbol, e.symbolContext, e.relatedSymbol, e.expectedRelatedSymbolContext)
}

type multipleRelationshipWarning struct {
	symbol        string
	relatedSymbol string
}

func (e multipleRelationshipWarning) Error() string {
	return fmt.Sprintf("warning: found multiple relationships from"+
		" '%s' to '%s', which could optimized into a single relationship", e.symbol, e.relatedSymbol)
}

type missingSymbolForOccurrenceError struct {
	symbol string
	path   string
	occ    scip.Range
}

func (e missingSymbolForOccurrenceError) Error() string {
	return fmt.Sprintf("error: found occurrence at %s @ %s"+
		" for symbol %s, but there is no matching SymbolInformation"+
		" in external symbols or any document", e.path, scipRangeToString(e.occ), e.symbol)
}

type forwardDefIsDefinitionError struct {
	symbol string
	path   string
	range_ scip.Range
}

func (e forwardDefIsDefinitionError) Error() string {
	return fmt.Sprintf("error: forward declaration for %v at %v @ %v was marked as definition",
		e.symbol, e.path, scipRangeToString(e.range_))
}

type duplicateOccurrenceWarning struct {
	symbol      string
	path        string
	range_      scip.Range
	symbolRoles int32
}

func (e duplicateOccurrenceWarning) Error() string {
	return fmt.Sprintf("warning: found duplicate occurrence for %s at %s @ %s with role %d",
		e.symbol, e.path, scipRangeToString(e.range_), e.symbolRoles)
}

type note struct {
	message string
}

func (n note) Error() string {
	return fmt.Sprintf("note: %s", n.message)
}

// --- Miscellaneous utility types ---

type errorSet struct {
	data []error
}

func (e *errorSet) Add(err error) {
	e.data = append(e.data, err)
}

func (e *errorSet) Unique() []error {
	m := map[string]error{}
	for _, e := range e.data {
		m[e.Error()] = e
	}
	var out []error
	for _, e := range m {
		out = append(out, e)
	}
	sort.Sort(mySlice[error]{out, func(e1 error, e2 error) bool {
		return e1.Error() < e2.Error()
	}})
	return out
}

type mySlice[T any] struct {
	data []T
	cmp  func(T, T) bool
}

func (s mySlice[T]) Len() int {
	return len(s.data)
}
func (s mySlice[T]) Swap(i int, j int) {
	s.data[i], s.data[j] = s.data[j], s.data[i]
}
func (s mySlice[T]) Less(i int, j int) bool {
	return s.cmp(s.data[i], s.data[j])
}

type set[T comparable] struct {
	impl map[T]struct{}
}

func newSet[T comparable]() set[T] {
	return set[T]{impl: map[T]struct{}{}}
}

func (s *set[T]) Add(t T) bool {
	if _, ok := s.impl[t]; ok {
		return false
	}
	s.impl[t] = struct{}{}
	return true
}

func (s *set[T]) Contains(t T) bool {
	_, ok := s.impl[t]
	return ok
}

func setToString[T comparable](s set[T], toString func(T) string) string {
	var out strings.Builder
	out.WriteRune('{')
	slice := make([]string, 0, len(s.impl))
	for x := range s.impl {
		slice = append(slice, toString(x))
	}
	sort.Slice(slice, func(i, j int) bool { return slice[i] < slice[j] })
	for i, x := range slice {
		if i != 0 {
			out.WriteString(", ")
		}
		out.WriteString(x)
	}
	out.WriteRune('}')
	return out.String()
}

type pair[T any] struct {
	first  T
	second T
}
