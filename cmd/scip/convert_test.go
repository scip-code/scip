package main

import (
	"bytes"
	"cmp"
	"path/filepath"
	"slices"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"

	"github.com/scip-code/scip/bindings/go/scip"
)

func TestConvert_SmokeTest(t *testing.T) {
	// Create temp directory for test DB
	tempDir := t.TempDir()

	sqliteDBPath := filepath.Join(tempDir, "index.db")

	index := testIndex1()

	db, err := createSQLiteDatabase(sqliteDBPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()

	writer, err := zstd.NewWriter(nil)
	require.NoError(t, err)
	converter := NewConverter(db, chunkSizeHint, writer)
	err = converter.Convert(index)
	require.NoError(t, err)

	checks := []struct {
		name string
		fn   func(*testing.T, *scip.Index, *sqlite.Conn)
	}{
		{"documents", checkDocuments},
		{"symbols", checkSymbols},
		{"occurrences", checkOccurrences},
		{"relationships", checkRelationships},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			check.fn(t, index, db)
		})
	}
}

func testIndex1() *scip.Index {
	pkg1S1Sym := "scip-go go . . pkg1/S1#"
	pkg1I1Sym := "scip-go go . . pkg1/I1#"
	return &scip.Index{
		Documents: []*scip.Document{
			{
				RelativePath: "a.go",
				Occurrences: []*scip.Occurrence{
					{Symbol: pkg1S1Sym, Range: []int32{10, 3, 6}, SymbolRoles: int32(scip.SymbolRole_Definition)},
					{Symbol: pkg1I1Sym, Range: []int32{20, 3, 6}, SymbolRoles: int32(scip.SymbolRole_Definition)},
				},
				Symbols: []*scip.SymbolInformation{
					// S1 implements I1 — exercises the relationships column.
					{
						Symbol: pkg1S1Sym,
						Relationships: []*scip.Relationship{
							{Symbol: pkg1I1Sym, IsImplementation: true},
						},
					},
					{Symbol: pkg1I1Sym},
				},
			},
			{
				RelativePath: "b.go",
				Occurrences: []*scip.Occurrence{
					{Symbol: pkg1S1Sym, Range: []int32{15, 9, 12}},
				},
			},
		},
	}
}

func checkDocuments(t *testing.T, index *scip.Index, db *sqlite.Conn) {
	query := "SELECT relative_path FROM documents"
	var dbPaths []string
	err := sqlitex.ExecuteTransient(db, query, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			dbPaths = append(dbPaths, stmt.ColumnText(0))
			return nil
		},
	})
	require.NoError(t, err)
	var expectedPaths []string
	for _, doc := range index.Documents {
		expectedPaths = append(expectedPaths, doc.RelativePath)
	}
	slices.Sort(expectedPaths)
	expectedPaths = slices.Compact(expectedPaths)
	slices.Sort(dbPaths)

	require.Equal(t, expectedPaths, dbPaths)
}

func checkSymbols(t *testing.T, index *scip.Index, db *sqlite.Conn) {
	query := "SELECT symbol FROM global_symbols"
	var dbSymbols []string
	err := sqlitex.ExecuteTransient(db, query, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			dbSymbols = append(dbSymbols, stmt.ColumnText(0))
			return nil
		},
	})
	require.NoError(t, err)

	var expectedSymbols []string
	for _, doc := range index.Documents {
		for _, occ := range doc.Occurrences {
			expectedSymbols = append(expectedSymbols, occ.Symbol)
		}
		for _, sym := range doc.Symbols {
			expectedSymbols = append(expectedSymbols, sym.Symbol)
		}
	}
	slices.Sort(expectedSymbols)
	expectedSymbols = slices.Compact(expectedSymbols)
	slices.Sort(dbSymbols)

	require.Equal(t, expectedSymbols, dbSymbols)
}

func checkOccurrences(t *testing.T, index *scip.Index, db *sqlite.Conn) {
	zstdReader, err := zstd.NewReader(bytes.NewBuffer(nil))
	require.NoError(t, err)

	query := `SELECT d.relative_path, occurrences
				  FROM documents d
				  JOIN chunks c ON c.document_id = d.id`
	dbOccurrences := []occurrenceData{}
	err = sqlitex.ExecuteTransient(db, query, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			var c Chunk
			err = c.fromDBFormat(stmt.ColumnReader(1), zstdReader)
			require.NoError(t, err)
			for _, occ := range c.Occurrences {
				dbOccurrences = append(dbOccurrences, occurrenceData{
					DocumentPath: stmt.ColumnText(0),
					Symbol:       occ.Symbol,
					Role:         occ.SymbolRoles,
					Range:        scip.NewRangeUnchecked(occ.Range),
				})
			}
			return nil
		},
	})
	require.NoError(t, err)
	cmpFn := func(a, b occurrenceData) int {
		return cmp.Or(cmp.Compare(a.DocumentPath, b.DocumentPath),
			a.Range.CompareStrict(b.Range))
	}
	slices.SortFunc(dbOccurrences, cmpFn)

	var expectedOccurrences []occurrenceData
	for _, doc := range index.Documents {
		for _, occ := range doc.Occurrences {
			expectedOccurrences = append(expectedOccurrences, occurrenceData{
				DocumentPath: doc.RelativePath,
				Symbol:       occ.Symbol,
				Role:         occ.SymbolRoles,
				Range:        scip.NewRangeUnchecked(occ.Range),
			})
		}
	}
	slices.SortFunc(expectedOccurrences, cmpFn)

	require.Equal(t, expectedOccurrences, dbOccurrences)
}

type occurrenceData struct {
	DocumentPath string
	Symbol       string
	Role         int32
	Range        scip.Range
}

// checkRelationships asserts SymbolInformation.relationships survives the
// round-trip into global_symbols.relationships, and that symbols without
// relationships store NULL rather than a compressed empty message. The column
// is declared in the schema, so a converter that never writes it yields a
// database that queries cleanly while reporting every symbol as having no
// relationships.
func checkRelationships(t *testing.T, index *scip.Index, db *sqlite.Conn) {
	expected := map[string][]*scip.Relationship{}
	for _, doc := range index.Documents {
		for _, sym := range doc.Symbols {
			if len(sym.Relationships) > 0 {
				expected[sym.Symbol] = sym.Relationships
			}
		}
	}
	require.NotEmpty(t, expected, "fixture must exercise at least one relationship")

	decoder, err := zstd.NewReader(nil)
	require.NoError(t, err)
	defer decoder.Close()

	found := map[string][]*scip.Relationship{}
	query := "SELECT symbol, relationships FROM global_symbols WHERE relationships IS NOT NULL"
	err = sqlitex.ExecuteTransient(db, query, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			symbol := stmt.ColumnText(0)
			blob := make([]byte, stmt.ColumnLen(1))
			stmt.ColumnBytes(1, blob)

			raw, err := decoder.DecodeAll(blob, nil)
			if err != nil {
				return err
			}
			var info scip.SymbolInformation
			if err := proto.Unmarshal(raw, &info); err != nil {
				return err
			}
			found[symbol] = info.Relationships
			return nil
		},
	})
	require.NoError(t, err)

	require.Len(t, found, len(expected))
	for symbol, want := range expected {
		got, ok := found[symbol]
		require.True(t, ok, "no relationships stored for %s", symbol)
		require.Len(t, got, len(want))
		for i := range want {
			require.Equal(t, want[i].Symbol, got[i].Symbol)
			require.Equal(t, want[i].IsImplementation, got[i].IsImplementation)
		}
	}

	// Symbols carrying no relationships must be NULL, not an empty frame:
	// insertGlobalSymbols is also called with synthetic SymbolInformation
	// values that only have a Symbol.
	var nullCount int
	err = sqlitex.ExecuteTransient(db,
		"SELECT COUNT(*) FROM global_symbols WHERE relationships IS NULL",
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *sqlite.Stmt) error {
				nullCount = stmt.ColumnInt(0)
				return nil
			},
		})
	require.NoError(t, err)
	require.Greater(t, nullCount, 0, "symbols without relationships must store NULL")
}
