package scip

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	fuzz "github.com/google/gofuzz"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestEmpty(t *testing.T) {
	index := Index{}
	checkRoundtrip(t, &index)
}

func TestFuzz(t *testing.T) {
	pat := regexp.MustCompile("^(state|sizeCache|unknownFields|SignatureDocumentation|TypedRange|TypedEnclosingRange)$")
	f := fuzz.New().NumElements(0, 2).SkipFieldsWithPattern(pat)
	for i := 0; i < 100; i++ {
		index := Index{}
		f.Fuzz(&index)

		checkRoundtrip(t, &index)
	}
}

func TestLargeDocuments(t *testing.T) {
	// Regression test: the streaming parser must handle documents
	// large enough that a single io.Reader.Read call may not fill
	// the buffer completely.
	const textSize = 128 * 1024
	const docCount = 10
	const occurrencesPerDoc = 200
	index := Index{
		Metadata: &Metadata{
			Version:              ProtocolVersion_UnspecifiedProtocolVersion,
			ToolInfo:             &ToolInfo{Name: "test-indexer", Version: "1.0.0"},
			ProjectRoot:          "file:///test/project",
			TextDocumentEncoding: TextEncoding_UTF8,
		},
	}
	for i := 0; i < docCount; i++ {
		doc := Document{
			RelativePath: fmt.Sprintf("src/file%d.ts", i),
			Text:         strings.Repeat(fmt.Sprintf("%d", i), textSize),
		}
		for j := 0; j < occurrencesPerDoc; j++ {
			doc.Occurrences = append(doc.Occurrences, &Occurrence{
				Range:  []int32{int32(j), 0, 10},
				Symbol: fmt.Sprintf("test . pkg v1 File%d#method%d().", i, j),
			})
		}
		index.Documents = append(index.Documents, &doc)
	}
	index.ExternalSymbols = append(index.ExternalSymbols, &SymbolInformation{
		Symbol:        "test . dep v2 ExternalType#",
		Documentation: []string{"An external type"},
	})
	indexBytes, err := proto.Marshal(&index)
	require.NoError(t, err)

	parsed := parseStreaming(t, bytes.NewReader(indexBytes))
	checkIndexEqual(t, &index, parsed)
}

func TestDocumentsOnly(t *testing.T) {
	pat := regexp.MustCompile("^(state|sizeCache|unknownFields|SignatureDocumentation|TypedRange|TypedEnclosingRange)$")
	f := fuzz.New().NumElements(0, 2).SkipFieldsWithPattern(pat)
	for i := 0; i < 100; i++ {
		index := Index{}
		f.Fuzz(&index)

		parsedIndex := Index{}

		indexVisitor := IndexVisitor{
			VisitDocument: func(_ context.Context, document *Document) error {
				parsedIndex.Documents = append(parsedIndex.Documents, document)
				return nil
			},
		}

		indexBytes, err := proto.Marshal(&index)
		require.NoError(t, err)
		bytesReader := bytes.NewReader(indexBytes)

		if err := indexVisitor.ParseStreaming(context.Background(), bytesReader); err != nil {
			t.Fatalf("got error parsing index %v", err)
		}

		onlyDocumentsIndex := Index{}
		onlyDocumentsIndex.Documents = index.Documents

		checkIndexEqual(t, &onlyDocumentsIndex, &parsedIndex)
	}
}

func checkIndexEqual(t *testing.T, expected *Index, got *Index) {
	if !proto.Equal(expected, got) {
		want := protojson.MarshalOptions{Multiline: true}.Format(expected)
		got := protojson.MarshalOptions{Multiline: true}.Format(got)
		diff := cmp.Diff(want, got)
		require.NotEqual(t, diff, "")
		t.Fatalf("index (-want, +got): %s", diff)
	}
}

func checkRoundtrip(t *testing.T, index *Index) {
	indexBytes, err := proto.Marshal(index)
	require.NoError(t, err)
	bytesReader := bytes.NewReader(indexBytes)

	parsedIndex := parseStreaming(t, bytesReader)

	checkIndexEqual(t, index, parsedIndex)
}

func parseStreaming(t *testing.T, reader io.Reader) *Index {
	parsedIndex := Index{}

	indexVisitor := IndexVisitor{
		VisitMetadata: func(_ context.Context, metadata *Metadata) error {
			parsedIndex.Metadata = metadata
			return nil
		},
		VisitDocument: func(_ context.Context, document *Document) error {
			parsedIndex.Documents = append(parsedIndex.Documents, document)
			return nil
		},
		VisitExternalSymbol: func(_ context.Context, extSym *SymbolInformation) error {
			parsedIndex.ExternalSymbols = append(parsedIndex.ExternalSymbols, extSym)
			return nil
		},
	}

	if err := indexVisitor.ParseStreaming(context.Background(), reader); err != nil {
		t.Fatalf("got error parsing index %v", err)
	}
	return &parsedIndex
}
