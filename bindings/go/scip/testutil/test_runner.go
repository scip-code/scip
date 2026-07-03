package testutil

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/fatih/color"

	"github.com/scip-code/scip/bindings/go/scip"
)

// RunTests validates whether the SCIP data present in an index matches that
// specified in human-readable test files. This is the core logic used by the
// 'scip test' CLI command.
func RunTests(
	directory string,
	fileFilters []string,
	checkDocuments bool,
	index *scip.Index,
	commentSyntax string,
	output io.Writer,
) error {
	hasFailure := false

	fileFilterSet := map[string]struct{}{}
	for _, file := range fileFilters {
		fileFilterSet[file] = struct{}{}
	}

	allTestFilesSet := map[string]struct{}{}
	if err := filepath.WalkDir(directory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if filepath.Ext(path) == ".scip" {
			return nil
		}
		if len(fileFilterSet) > 0 {
			if _, ok := fileFilterSet[path]; ok {
				allTestFilesSet[path] = struct{}{}
			}
		} else if !d.IsDir() {
			allTestFilesSet[path] = struct{}{}
		}
		return nil
	}); err != nil {
		return err
	}

	for _, document := range index.Documents {
		sourceFilePath := filepath.Join(directory, document.RelativePath)

		if len(fileFilters) > 0 {
			if !slices.Contains(fileFilters, document.RelativePath) {
				continue
			}
		}

		data, err := os.ReadFile(sourceFilePath)
		if err != nil {
			return err
		}
		delete(allTestFilesSet, sourceFilePath)

		failures := []string{}
		successCount := 0

		syntheticDefinitions := syntheticDefinitionsByParent(document.Symbols)

		lines := strings.Split(string(data), "\n")
		for lineNumber := 0; lineNumber < len(lines); lineNumber++ {
			testCasesAtLine, usedLines := testCasesForLine(lineNumber, lines, commentSyntax)

			// if the test file contains no test lines, skip it. Only test the lines
			// that the test file dictates should be tested
			if len(testCasesAtLine) == 0 {
				continue
			}

			attributes := attributesForOccurrencesAtLine(
				lineNumber,
				renderLine(lines[lineNumber]),
				document.Occurrences,
				syntheticDefinitions,
			)
			for _, testCase := range testCasesAtLine {
				filteredAttrs := filterAttributesForTestCase(testCase, attributes)
				if !testCase.checkAll(filteredAttrs) {
					failures = append(failures, formatFailure(lineNumber, testCase, filteredAttrs))
				} else {
					successCount++
				}
			}

			lineNumber += usedLines
		}

		if len(failures) > 0 {
			hasFailure = true
			red := color.New(color.FgRed)
			red.Fprintf(output, "✗ %s\n", document.RelativePath)

			for _, failure := range failures {
				fmt.Fprintf(output, "%s\n", indent(failure, 4))
			}
		} else {
			green := color.New(color.FgGreen)
			green.Fprintf(output, "✓ %s (%d assertions)\n", document.RelativePath, successCount)
		}
	}

	if checkDocuments && len(allTestFilesSet) > 0 {
		sortedFiles := []string{}
		for f, _ := range allTestFilesSet {
			sortedFiles = append(sortedFiles, f)
		}
		slices.Sort(sortedFiles)
		red := color.New(color.FgRed)
		red.Fprintf(output, "✗ Missing documents in SCIP index\n")
		for _, path := range sortedFiles {
			fmt.Fprintf(output, "    %s\n", path)
		}
		hasFailure = true
	}

	if hasFailure {
		return errors.New("test failed")
	}

	return nil
}

// symbolAttribute refers to a single attribute of a symbol.
// This can be a definition, reference, documentation, or diagnostic
type symbolAttribute struct {
	// the column number where this symbol starts
	start int

	// the length of the symbol's name
	length int

	// the type of attribute that this is
	kind symbolAttributeKind

	// contextual information about the attribute, as determined
	// by the [kind]
	data string

	// any additional information necessary to represent this attribute
	// each line should be considered a "newline", and is used for multiline
	// comments (in documentation), and diagnostic information
	additionalData []string

	// hasEndPosition is true when this attribute describes a multiline
	// occurrence and the end position fields below are populated.
	hasEndPosition bool

	// endLineDelta is the number of lines between the occurrence's start and end
	// line (always > 0 when hasEndPosition is true).
	endLineDelta int

	// endCharacter is the occurrence's end character on its end line.
	endCharacter int
}

// displayData renders the attribute's data for failure messages, appending the
// `<lineDelta>:<endCharacter>` suffix for multiline occurrences so the output
// matches the `scip snapshot` representation.
func (a symbolAttribute) displayData() string {
	if a.hasEndPosition {
		return fmt.Sprintf("%s %d:%d", a.data, a.endLineDelta, a.endCharacter)
	}
	return a.data
}

type symbolAttributeKind string

const (
	definitionAttrKind          symbolAttributeKind = "definition"
	referenceAttrKind           symbolAttributeKind = "reference"
	forwardDefinitionAttrKind   symbolAttributeKind = "forward_definition"
	syntheticDefinitionAttrKind symbolAttributeKind = "synthetic_definition"
	diagnosticAttrKind          symbolAttributeKind = "diagnostic"
)

func symbolAttributeKindFromStr(str string) symbolAttributeKind {
	switch str {
	case "definition":
		return definitionAttrKind
	case "reference":
		return referenceAttrKind
	case "forward_definition":
		return forwardDefinitionAttrKind
	case "synthetic_definition":
		return syntheticDefinitionAttrKind
	case "diagnostic":
		return diagnosticAttrKind
	default:
		panic(fmt.Sprintf("Unknown symbolAttributeKind: %s", str))
	}
}

// symbolAttributeTestCase refers to metadata used to validate
// [symbolAttributes]
type symbolAttributeTestCase struct {
	attribute     symbolAttribute
	enforceLength bool
}

// commentsForLine returns the list of lines, after a provided [lineNumber], which are
// classified as comment. The comment type can be configured using [commentSyntax].
//
// Returns the list of symbolAttributeTestCase(s) for the provided line, and the number of
// of lines that were "consumed" by the cases on this line
func testCasesForLine(lineNumber int, lines []string, commentSyntax string) ([]symbolAttributeTestCase, int) {
	testCases := []symbolAttributeTestCase{}

	// if the specified lineNumber is outside the bounds of lines
	// return an empty array
	if lineNumber >= len(lines)-1 {
		return testCases, 0
	}

	testLines := []symbolAttributeTestCase{}
	usedLines := 0
	for i := lineNumber + 1; i < len(lines); i++ {
		line := lines[i]

		if !strings.HasPrefix(strings.TrimSpace(line), commentSyntax) {
			break
		}
		testCase, ok := parseTestCase(line, lines[i+1:], commentSyntax)
		if !ok {
			// Non-assertion comment line (e.g. snapshot metadata): consume and
			// ignore it, but keep scanning the rest of the comment block.
			usedLines += 1
			continue
		}

		testLines = append(testLines, testCase)
		i += len(testCase.attribute.additionalData)
		usedLines += 1 + len(testCase.attribute.additionalData)
	}

	return testLines, usedLines
}

func filterAttributesForTestCase(testCase symbolAttributeTestCase, attributes []symbolAttribute) []symbolAttribute {
	filteredAttrs := []symbolAttribute{}
	for _, attr := range attributes {
		if testCase.attribute.start >= attr.start && testCase.attribute.start <= (attr.start+attr.length)-1 {
			filteredAttrs = append(filteredAttrs, attr)
		}
	}
	return filteredAttrs
}

// attributeForRange builds a symbolAttribute for the given occurrence range,
// computing the marker length (and end position for multiline ranges) the same
// way the snapshot formatter does.
func attributeForRange(pos scip.Range, renderedLine string, kind symbolAttributeKind, data string) symbolAttribute {
	attr := symbolAttribute{
		start:          int(pos.Start.Character),
		length:         int(markerLength(pos, renderedLine)),
		kind:           kind,
		data:           data,
		additionalData: []string{},
	}
	if !pos.IsSingleLine() {
		attr.hasEndPosition = true
		attr.endLineDelta = int(pos.End.Line - pos.Start.Line)
		attr.endCharacter = int(pos.End.Character)
	}
	return attr
}

func attributesForOccurrencesAtLine(
	lineNumber int,
	renderedLine string,
	occurrences []*scip.Occurrence,
	syntheticDefinitions map[string][]*scip.SymbolInformation,
) []symbolAttribute {
	result := []symbolAttribute{}
	for _, occ := range occurrences {
		pos, ok := occ.SourceRange()
		if !ok {
			continue
		}
		if pos.Start.Line != int32(lineNumber) {
			continue
		}

		isDefinition := scip.SymbolRole_Definition.Matches(occ)
		kind := referenceAttrKind
		if isDefinition {
			kind = definitionAttrKind
		} else if scip.SymbolRole_ForwardDefinition.Matches(occ) {
			kind = forwardDefinitionAttrKind
		}
		result = append(result, attributeForRange(pos, renderedLine, kind, occ.Symbol))

		for _, diagnostic := range occ.Diagnostics {
			diag := attributeForRange(pos, renderedLine, diagnosticAttrKind, diagnostic.Severity.String())
			diag.additionalData = []string{diagnostic.Message}
			result = append(result, diag)
		}

		// Synthetic definitions are projected onto a real definition's range,
		// mirroring the `scip snapshot` output.
		if isDefinition {
			for _, synthetic := range syntheticDefinitions[occ.Symbol] {
				result = append(result, attributeForRange(pos, renderedLine, syntheticDefinitionAttrKind, synthetic.Symbol))
			}
		}
	}
	return result
}

type rangeMarker byte

const (
	caretMarker      rangeMarker = '^'
	underscoreMarker rangeMarker = '_'
)

type rangeSelection struct {
	start         int
	length        int
	enforceLength bool
	marker        rangeMarker
	// content is the test line with the comment prefix and range markers removed.
	content string
}

// parseRangeSelection extracts the range-selection portion of a test comment
// line. Ranges are selected with `<-`, a run of `^` (caret), or a run of `_`
// (underscore, used by `synthetic_definition`). It returns ok=false for comment
// lines that contain no range selection, such as snapshot metadata detail lines
// (`kind`, `display_name`, `enclosing_symbol`, `relationship`, `>` ...), which
// the test runner ignores rather than asserts.
func parseRangeSelection(line, commentSyntax string) (rangeSelection, bool) {
	commentIdx := strings.Index(line, commentSyntax)
	if commentIdx < 0 {
		return rangeSelection{}, false
	}
	body := line[commentIdx+len(commentSyntax):]
	trimmed := strings.TrimLeft(body, " \t")
	leading := len(body) - len(trimmed)

	// `<-` selects the character above the first comment character, similar to
	// Sublime Text syntax tests.
	if strings.HasPrefix(trimmed, "<-") {
		return rangeSelection{
			start:   commentIdx,
			marker:  caretMarker,
			content: strings.TrimPrefix(trimmed, "<-"),
		}, true
	}

	if trimmed == "" {
		return rangeSelection{}, false
	}
	marker := rangeMarker(trimmed[0])
	if marker != caretMarker && marker != underscoreMarker {
		return rangeSelection{}, false
	}

	// Count only the contiguous marker run. This matters because `_` also
	// appears inside kinds like `synthetic_definition` and `forward_definition`.
	runLen := 0
	for runLen < len(trimmed) && rangeMarker(trimmed[runLen]) == marker {
		runLen++
	}

	sel := rangeSelection{
		start:   commentIdx + len(commentSyntax) + leading,
		marker:  marker,
		content: trimmed[runLen:],
	}
	// A single marker ignores length; two or more enforce it.
	if runLen >= 2 {
		sel.enforceLength = true
		sel.length = runLen
	}
	return sel, true
}

// parseMultilineSuffix splits a trailing `<lineDelta>:<endCharacter>` token off
// of the test data. The suffix is only recognized when lineDelta > 0, matching
// the multiline occurrences emitted by `scip snapshot`.
func parseMultilineSuffix(data string) (rest string, endLineDelta int, endCharacter int, ok bool) {
	fields := strings.Fields(data)
	if len(fields) == 0 {
		return data, 0, 0, false
	}
	last := fields[len(fields)-1]
	colon := strings.IndexByte(last, ':')
	if colon <= 0 || colon == len(last)-1 {
		return data, 0, 0, false
	}
	lineDelta, err1 := strconv.Atoi(last[:colon])
	endChar, err2 := strconv.Atoi(last[colon+1:])
	if err1 != nil || err2 != nil || lineDelta <= 0 || endChar < 0 {
		return data, 0, 0, false
	}
	rest = strings.TrimSpace(strings.TrimSuffix(data, last))
	return rest, lineDelta, endChar, true
}

// parseTestCase parses a single test comment line. It returns ok=false for
// comment lines that are not assertions (e.g. snapshot metadata detail lines),
// which the caller skips.
func parseTestCase(line string, leadingLines []string, commentSyntax string) (symbolAttributeTestCase, bool) {
	sel, ok := parseRangeSelection(line, commentSyntax)
	if !ok {
		return symbolAttributeTestCase{}, false
	}

	content := strings.TrimSpace(sel.content)
	fields := strings.Fields(content)
	if len(fields) == 0 {
		// A range marker with no kind is not a valid test case; ignore it.
		return symbolAttributeTestCase{}, false
	}

	// the type of the symbol is the first word, e.g. "definition", "reference",
	// "synthetic_definition", "forward_definition", "diagnostic".
	kindStr := fields[0]
	kind := symbolAttributeKindFromStr(kindStr)

	if sel.marker == underscoreMarker && kind != syntheticDefinitionAttrKind {
		panic(fmt.Sprintf("'_' range marker is only valid for synthetic_definition, got %q", kindStr))
	}

	// the data is everything after the type, minus any multiline suffix
	data := strings.TrimSpace(strings.TrimPrefix(content, kindStr))
	data, endLineDelta, endCharacter, hasEndPosition := parseMultilineSuffix(data)

	additionalData := []string{}
	for i := range leadingLines {
		leadingLine := leadingLines[i]
		// if the leadingLine is not a comment line, we're outside of the test case block.
		if !strings.HasPrefix(strings.TrimSpace(leadingLine), commentSyntax) {
			break
		}

		// remove the comment character(s)
		leadingLine = strings.Replace(leadingLine, commentSyntax, "", 1)

		// if the leading line doesn't start with '>', its not a multiline case
		if !strings.HasPrefix(strings.TrimSpace(leadingLine), ">") {
			break
		}

		// remove the '>' character
		leadingLine = strings.Replace(leadingLine, ">", "", 1)
		additionalData = append(additionalData, strings.TrimSpace(leadingLine))
	}

	return symbolAttributeTestCase{
		attribute: symbolAttribute{
			kind:           kind,
			start:          sel.start,
			length:         sel.length,
			data:           data,
			additionalData: additionalData,
			hasEndPosition: hasEndPosition,
			endLineDelta:   endLineDelta,
			endCharacter:   endCharacter,
		},
		enforceLength: sel.enforceLength,
	}, true
}

func (s symbolAttributeTestCase) checkAll(attributes []symbolAttribute) bool {
	for _, attr := range attributes {
		if s.check(attr) {
			return true
		}
	}
	return false
}

func (s symbolAttributeTestCase) check(attr symbolAttribute) bool {
	if s.enforceLength {
		if s.attribute.length != attr.length || s.attribute.start != attr.start {
			return false
		}
	} else {
		if s.attribute.start < attr.start || s.attribute.start > (attr.start+attr.length)-1 {
			return false
		}
	}

	if s.attribute.kind != attr.kind {
		return false
	}

	// check if symbols are equal, a `.` character in the testCaseSymbol is considered
	// a wildcard, and matches the correlating group
	testCaseSymbolParts := strings.Fields(s.attribute.data)
	attrSymbolParts := strings.Fields(attr.data)
	if len(testCaseSymbolParts) != len(attrSymbolParts) {
		return false
	}
	for i, testCaseSymbolPart := range testCaseSymbolParts {
		if testCaseSymbolPart == "." {
			continue
		}
		if testCaseSymbolPart != attrSymbolParts[i] {
			return false
		}
	}

	// when the test specifies a multiline end position (`<lineDelta>:<endChar>`),
	// require the occurrence to match it. Tests that omit the suffix do not
	// constrain the end position.
	if s.attribute.hasEndPosition {
		if !attr.hasEndPosition ||
			s.attribute.endLineDelta != attr.endLineDelta ||
			s.attribute.endCharacter != attr.endCharacter {
			return false
		}
	}

	// only validate additionalData if the testCases provides one
	// otherwise, ignore what the attribute specifies
	if len(s.attribute.additionalData) > 0 {
		if !slices.Equal(s.attribute.additionalData, attr.additionalData) {
			return false
		}
	}

	return true
}

func formatFailure(lineNumber int, testCase symbolAttributeTestCase, attributesAtLine []symbolAttribute) string {
	failureDesc := []string{
		fmt.Sprintf("Failure [Ln: %d, Col: %d]", lineNumber+1, testCase.attribute.start),
		fmt.Sprintf("  Expected: '%s %s'", testCase.attribute.kind, testCase.attribute.displayData()),
	}
	for _, add := range testCase.attribute.additionalData {
		failureDesc = append(failureDesc, indent(fmt.Sprintf("'%s'", add), 12))
	}

	if (len(attributesAtLine)) == 0 {
		failureDesc = append(failureDesc, "    Actual: <no attributes found>")
	} else {
		for i, attr := range attributesAtLine {
			prefix := "       "
			if i == 0 {
				prefix = "Actual:"
			}

			if len(attributesAtLine) > 1 {
				prefix += " *"
			} else {
				prefix += "  "
			}

			failureDesc = append(failureDesc, fmt.Sprintf("  %s '%s %s'", prefix, attr.kind, attr.displayData()))
			for _, add := range attr.additionalData {
				failureDesc = append(failureDesc, indent(fmt.Sprintf("'%s'", add), 12))
			}
		}
	}
	return strings.Join(failureDesc, "\n")
}

// --------------------------------- Utils ---------------------------------

func indent(str string, count int) string {
	lines := strings.Split(str, "\n")
	newLines := []string{}
	for _, line := range lines {
		newLines = append(newLines, strings.Repeat(" ", count)+line)
	}
	return strings.Join(newLines, "\n")
}
