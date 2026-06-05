package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/scip-code/scip/bindings/go/scip"
)

type stringMap = map[string][]string

var symbolRegexp = regexp.MustCompile(`([0-9a-zA-Z]*)(.*)`)
var relationsRegexp = regexp.MustCompile(`~([0-9a-zA-Z]*)#([rdit]*)`)

// parseSymbolInfo allows compactly specifying relationships based on the grammar:
//
//	  <kind> ::= 'r' | 'd' | 'i' | 't'
//	  <name> ::= [0-9a-zA-Z]*
//	  <symbol> = <name> ('~' <name> '#' ( <kind> )* )*
//	  ^ symbol name                        ^ relationship
//		                      ^ related symbol name
func parseSymbolInfo(symInfoStr string) *scip.SymbolInformation {
	matches := symbolRegexp.FindStringSubmatch(symInfoStr)
	if matches == nil {
		panic(fmt.Sprintf("failed to parse symbol info string %s", symInfoStr))
	}
	sym := matches[1]
	relText := matches[2]
	var rels []*scip.Relationship
	if relText != "" {
		for _, match := range relationsRegexp.FindAllStringSubmatch(relText, -1) {
			rel := scip.Relationship{Symbol: match[1]}
			relFields := match[2]
			if strings.ContainsRune(relFields, 'r') {
				rel.IsReference = true
			}
			if strings.ContainsRune(relFields, 'd') {
				rel.IsDefinition = true
			}
			if strings.ContainsRune(relFields, 'i') {
				rel.IsImplementation = true
			}
			if strings.ContainsRune(relFields, 't') {
				rel.IsTypeDefinition = true
			}
			rels = append(rels, &rel)
		}
	}
	return &scip.SymbolInformation{Symbol: sym, Relationships: rels}
}

var placeholderRange = []int32{0, 0, 0}

const placeholderRole int32 = 0

func makeIndex(extSyms []string, docSyms stringMap, docOccs stringMap) *scip.Index {
	scipExtSyms := make([]*scip.SymbolInformation, 0, len(extSyms))
	for _, s := range extSyms {
		scipExtSyms = append(scipExtSyms, parseSymbolInfo(s))
	}
	if docOccs == nil {
		docOccs = map[string][]string{}
	}
	docs := make([]*scip.Document, 0, len(docSyms))
	for p, ss := range docSyms {
		syms := make([]*scip.SymbolInformation, 0, len(ss))
		for _, s := range ss {
			syms = append(syms, parseSymbolInfo(s))
		}
		var occs []*scip.Occurrence
		if foundOccs, ok := docOccs[p]; ok {
			for _, s := range foundOccs {
				occs = append(occs, &scip.Occurrence{Symbol: s, Range: placeholderRange, SymbolRoles: placeholderRole})
			}
			delete(docOccs, p)
		}
		docs = append(docs, &scip.Document{RelativePath: p, Symbols: syms, Occurrences: occs})
	}
	for p, ss := range docOccs {
		occs := make([]*scip.Occurrence, 0, len(ss))
		for _, s := range ss {
			occs = append(occs, &scip.Occurrence{Symbol: s, Range: placeholderRange, SymbolRoles: placeholderRole})
		}
		delete(docOccs, p)
		docs = append(docs, &scip.Document{RelativePath: p, Symbols: nil, Occurrences: occs})
	}
	return &scip.Index{Documents: docs, ExternalSymbols: scipExtSyms}
}

type testCase struct {
	name           string
	index          *scip.Index
	expectedErrors []error
}

func TestErrors(t *testing.T) {
	SkipLintSymbolParseTest = true
	var testCases []testCase

	testCases = []testCase{
		{"duplicateSymbolInfo",
			makeIndex(
				nil,
				stringMap{"myfile": {"dupSym", "dupSym"}},
				nil),
			[]error{
				duplicateSymbolInfoWarning{"dupSym", "myfile"},
			}},
		{
			"emptyString",
			makeIndex([]string{"", "k~#r"}, stringMap{"": nil, "a": {"", "b", "h~#r"}}, stringMap{"c": {""}}),
			[]error{
				emptyStringError{"symbol", "external symbols"},
				emptyStringError{"document", "index"},
				emptyStringError{"symbol", "symbols for document a"},
				emptyStringError{"symbol", "occurrence at c @ 0:0-0:0"},
				emptyStringError{"symbol", "relationships for h"},
				emptyStringError{"symbol", "relationships for external symbol k"},
			},
		},
		{
			"nonCanonicalSymbol",
			makeIndex([]string{}, stringMap{}, stringMap{"c": {". . . . `simple`#"}}),
			[]error{
				nonCanonicalSymbolError{". . . . `simple`#", ". . . . simple#", ""},
			},
		},
		{
			"duplicateDocument",
			&scip.Index{Documents: []*scip.Document{{RelativePath: "dup"}, {RelativePath: "dup"}}},
			[]error{
				duplicateDocumentWarning{"dup"},
			},
		},
		{
			"bothLocalAndExternalSymbol",
			makeIndex([]string{"foo"}, stringMap{"x": {"foo"}}, nil),
			[]error{
				bothLocalAndExternalSymbolError{"foo", "x"},
			},
		},
		{
			"missingRelationshipFlag",
			makeIndex([]string{"b", "a~b#"}, stringMap{"f": {"x~a#"}}, nil),
			[]error{
				missingRelationshipFlagError{"a", "external symbols"},
				missingRelationshipFlagError{"x", "symbols for file f"},
			},
		},
		{
			"missingSymbolInRelationship",
			makeIndex([]string{"a~b#r"}, stringMap{"f": {"x~y#r"}}, nil),
			[]error{
				missingSymbolInRelationshipError{
					"a", "external symbols",
					"b", "external symbols",
				},
				missingSymbolInRelationshipError{
					"x", "symbols for file f",
					"y", "external symbols or some other document",
				},
			},
		},
		{
			"multipleRelationship",
			makeIndex([]string{"b", "a~b#r", "a~b#d"}, stringMap{"f": {"x~a#r", "x~a#d"}}, nil),
			[]error{
				multipleRelationshipWarning{"a", "b"},
				multipleRelationshipWarning{"x", "a"},
			},
		},
		{
			"missingSymbolForOccurrence",
			makeIndex(nil, nil, stringMap{"f": {"a"}}),
			[]error{
				missingSymbolForOccurrenceError{"a", "f", scip.NewRangeUnchecked(placeholderRange)},
			},
		},
		{
			"duplicateOccurrence",
			makeIndex([]string{"a"}, stringMap{"f": {"b"}}, stringMap{"f": {"a", "a", "b", "b"}}),
			[]error{
				duplicateOccurrenceWarning{"a", "f", scip.NewRangeUnchecked(placeholderRange), placeholderRole},
				duplicateOccurrenceWarning{"b", "f", scip.NewRangeUnchecked(placeholderRange), placeholderRole},
			},
		},
		{
			"invalidOccurrenceRange",
			&scip.Index{Documents: []*scip.Document{
				{RelativePath: "f", Occurrences: []*scip.Occurrence{
					{Symbol: "a", Range: []int32{1, 2}}, // wrong length
				}},
			}},
			[]error{
				invalidOccurrenceRangeError{"a", "f"},
			},
		},
	}

	for _, testCase := range testCases {
		errMsgSet := errorMessages(lintMainPure(testCase.index))
		for _, expectErr := range testCase.expectedErrors {
			if !errMsgSet.Contains(expectErr.Error()) {
				t.Errorf("test %s failure: expected\n%s\nbut it wasn't present in error set:\n%s",
					testCase.name, expectErr.Error(), setToString(errMsgSet, func(t string) string { return t + "\n" }))
			}
		}
	}
}

func errorMessages(e errorSet) set[string] {
	out := newSet[string]()
	for _, e := range e.data {
		out.Add(e.Error())
	}
	return out
}
