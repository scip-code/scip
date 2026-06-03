package testutil

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/scip-code/scip/bindings/go/scip"
)

// FormatSnapshots renders the provided SCIP index into a pretty-printed text format
// that is suitable for snapshot testing.
func FormatSnapshots(
	index *scip.Index,
	commentSyntax string,
	symbolFormatter scip.SymbolFormatter,
	customProjectRoot string,
) ([]*scip.SourceFile, error) {
	var result []*scip.SourceFile
	projectRootUrl, err := url.Parse(index.Metadata.ProjectRoot)
	if err != nil {
		return nil, err
	}

	localSourcesRoot := projectRootUrl.Path
	if customProjectRoot != "" {
		localSourcesRoot = customProjectRoot
	} else if _, err := os.Stat(localSourcesRoot); errors.Is(err, os.ErrNotExist) {
		cwd, _ := os.Getwd()
		log.Printf("Project root [%s] doesn't exist, using current working directory [%s] instead. "+
			"To override this behaviour, use --project-root=<folder> option directly", projectRootUrl.Path, cwd)
		localSourcesRoot = cwd
	}

	var documentErrors error
	for _, document := range index.Documents {
		sourceFilePath := filepath.Join(localSourcesRoot, document.RelativePath)
		snapshot, err := FormatSnapshot(document, commentSyntax, symbolFormatter, sourceFilePath)
		if err = symbolFormatter.OnError(err); err != nil {
			documentErrors = errors.Join(
				documentErrors, fmt.Errorf("%s: %w", document.RelativePath, err))
		}
		sourceFile := scip.NewSourceFile(sourceFilePath,
			document.RelativePath,
			snapshot,
		)
		result = append(result, sourceFile)
	}
	if len(index.ExternalSymbols) > 0 {
		formatted, err := formatExternalSymbols(index.ExternalSymbols, symbolFormatter)
		if err = symbolFormatter.OnError(err); err != nil {
			documentErrors = errors.Join(
				documentErrors, fmt.Errorf("external_symbols: %w", err))
		}
		result = append(result, scip.NewSourceFile(
			filepath.Join(localSourcesRoot, "external_symbols.txt"),
			"external_symbols.txt",
			formatted,
		))
	}

	return result, documentErrors
}

// FormatSnapshot renders the provided SCIP index into a pretty-printed text format
// that is suitable for snapshot testing.
func FormatSnapshot(
	document *scip.Document,
	commentSyntax string,
	formatter scip.SymbolFormatter,
	sourceFilePath string,
) (string, error) {
	b := strings.Builder{}
	data, err := os.ReadFile(sourceFilePath)
	if err != nil {
		return "", err
	}
	symtab := document.SymbolTable()
	sort.SliceStable(document.Occurrences, func(i, j int) bool {
		ri, _ := document.Occurrences[i].SourceRange()
		rj, _ := document.Occurrences[j].SourceRange()
		return ri.LessStrict(rj)
	})
	var formattingError error
	formatSymbol := func(symbol string) string {
		formatted, err := formatter.Format(symbol)
		if err != nil {
			formattingError = errors.Join(formattingError, fmt.Errorf("%s: %w", symbol, err))
			return symbol
		}
		return formatted
	}
	enclosingRanges := enclosingRanges(document.Occurrences)
	enclosingByStartLine := enclosingRangesByStartLine(enclosingRanges)
	enclosingByEndLine := enclosingRangesByEndLine(enclosingRanges)
	i := 0
	for lineNumber, line := range strings.Split(string(data), "\n") {
		for _, er := range enclosingByStartLine[int32(lineNumber)] {
			b.WriteString(commentSyntax)
			for indent := int32(0); indent < er.Range.Start.Character; indent++ {
				b.WriteRune(' ')
			}
			b.WriteString("⌄ enclosing_range_start ")
			b.WriteString(formatSymbol(er.Symbol))
			b.WriteString("\n")
		}

		line = strings.TrimSuffix(line, "\r")
		b.WriteString(strings.Repeat(" ", len(commentSyntax)))
		b.WriteString(strings.ReplaceAll(line, "\t", " "))
		b.WriteString("\n")
		for i < len(document.Occurrences) {
			occ := document.Occurrences[i]
			pos, _ := occ.SourceRange()
			if pos.Start.Line != int32(lineNumber) {
				break
			}
			if !pos.IsSingleLine() {
				i++
				continue
			}
			b.WriteString(commentSyntax)
			for indent := int32(0); indent < pos.Start.Character; indent++ {
				b.WriteRune(' ')
			}
			length := pos.End.Character - pos.Start.Character
			for caret := int32(0); caret < length; caret++ {
				b.WriteRune('^')
			}
			b.WriteRune(' ')
			role := "reference"
			isDefinition := scip.SymbolRole_Definition.Matches(occ)
			if isDefinition {
				role = "definition"
			} else if scip.SymbolRole_ForwardDefinition.Matches(occ) {
				role = "forward_definition"
			}
			b.WriteString(role)
			b.WriteRune(' ')
			b.WriteString(formatSymbol(occ.Symbol))

			prefix := "\n" + commentSyntax + strings.Repeat(" ", int(pos.End.Character)+1)

			hasOverrideDocumentation := len(occ.OverrideDocumentation) > 0
			if hasOverrideDocumentation {
				documentation := occ.OverrideDocumentation[0]
				writeDocumentation(&b, documentation, prefix, true)
			}

			if info, ok := symtab[occ.Symbol]; ok && isDefinition {
				if info.Kind != scip.SymbolInformation_UnspecifiedKind {
					b.WriteString(prefix)
					b.WriteString("kind ")
					b.WriteString(info.Kind.String())
				}

				if info.DisplayName != "" {
					b.WriteString(prefix)
					b.WriteString("display_name ")
					b.WriteString(info.DisplayName)
				}

				if info.SignatureDocumentation != nil && info.SignatureDocumentation.Text != "" {
					b.WriteString(prefix)
					b.WriteString("signature_documentation")
					writeMultiline(&b, prefix, info.SignatureDocumentation.Text)
				}

				for _, documentation := range info.Documentation {
					// At least get the first line of documentation if there is leading whitespace
					documentation = strings.TrimSpace(documentation)
					writeDocumentation(&b, documentation, prefix, false)
				}

				writeRelationships(&b, info.Relationships, prefix, formatSymbol)
			}

			for _, diagnostic := range occ.Diagnostics {
				writeDiagnostic(&b, prefix, diagnostic)
			}

			b.WriteString("\n")
			i++
		}
		for _, er := range enclosingByEndLine[int32(lineNumber)] {
			b.WriteString(commentSyntax)
			for indent := int32(0); indent < er.Range.End.Character-1; indent++ {
				b.WriteRune(' ')
			}
			b.WriteString("⌃ enclosing_range_end ")
			b.WriteString(formatSymbol(er.Symbol))
			b.WriteString("\n")
		}
	}
	return b.String(), formattingError
}

func formatExternalSymbols(
	symbols []*scip.SymbolInformation, formatter scip.SymbolFormatter,
) (string, error) {
	var b strings.Builder
	var formattingError error

	sorted := make([]*scip.SymbolInformation, len(symbols))
	copy(sorted, symbols)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Symbol < sorted[j].Symbol
	})

	formatSymbol := func(symbol string) string {
		formatted, err := formatter.Format(symbol)
		if err != nil {
			formattingError = errors.Join(
				formattingError, fmt.Errorf("%s: %w", symbol, err))
			return symbol
		}
		return formatted
	}

	for i, sym := range sorted {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(formatSymbol(sym.Symbol))
		writeRelationships(&b, sym.Relationships, "\n  ", formatSymbol)
	}

	return b.String(), formattingError
}

func writeRelationships(
	b *strings.Builder,
	relationships []*scip.Relationship,
	prefix string,
	formatSymbol func(string) string,
) {
	sorted := make([]*scip.Relationship, len(relationships))
	copy(sorted, relationships)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Symbol < sorted[j].Symbol
	})
	for _, relationship := range sorted {
		b.WriteString(prefix)
		b.WriteString("relationship ")
		b.WriteString(formatSymbol(relationship.Symbol))
		if relationship.IsImplementation {
			b.WriteString(" implementation")
		}
		if relationship.IsReference {
			b.WriteString(" reference")
		}
		if relationship.IsTypeDefinition {
			b.WriteString(" type_definition")
		}
	}
}

func writeMultiline(b *strings.Builder, prefix string, paragraph string) {
	for _, s := range strings.Split(paragraph, "\n") {
		b.WriteString(prefix)
		b.WriteString("> ")
		b.WriteString(s)
	}
}

func writeDocumentation(b *strings.Builder, documentation string, prefix string, override bool) {
	b.WriteString(prefix)
	if override {
		b.WriteString("override_")
	}
	b.WriteString("documentation")

	writeMultiline(b, prefix, documentation)
}

func writeDiagnostic(b *strings.Builder, prefix string, diagnostic *scip.Diagnostic) {
	b.WriteString(prefix)
	b.WriteString("diagnostic ")
	b.WriteString(diagnostic.Severity.String())
	b.WriteRune(':')

	writeMultiline(b, prefix, diagnostic.Message)
}

type enclosingRange struct {
	Range  scip.Range
	Symbol string
}

func enclosingRanges(occurrences []*scip.Occurrence) []enclosingRange {
	var enclosingRanges []enclosingRange
	for _, occ := range occurrences {
		if r, ok := occ.EnclosingSourceRange(); ok {
			enclosingRanges = append(enclosingRanges, enclosingRange{
				Range:  r,
				Symbol: occ.Symbol,
			})
		}
	}
	return enclosingRanges
}

func enclosingRangesByStartLine(ranges []enclosingRange) map[int32][]enclosingRange {
	result := map[int32][]enclosingRange{}
	for _, r := range ranges {
		result[r.Range.Start.Line] = append(result[r.Range.Start.Line], r)
	}
	for _, ers := range result {
		sort.SliceStable(ers, func(i, j int) bool {
			return ers[i].Range.Start.Character < ers[j].Range.Start.Character
		})
	}
	return result
}

func enclosingRangesByEndLine(ranges []enclosingRange) map[int32][]enclosingRange {
	result := map[int32][]enclosingRange{}
	for _, r := range ranges {
		result[r.Range.End.Line] = append(result[r.Range.End.Line], r)
	}
	for _, ers := range result {
		sort.SliceStable(ers, func(i, j int) bool {
			return ers[i].Range.End.Character < ers[j].Range.End.Character
		})
	}
	return result
}
