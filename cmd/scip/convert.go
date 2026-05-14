package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/urfave/cli/v3"
	"google.golang.org/protobuf/proto"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"

	"github.com/scip-code/scip/bindings/go/scip"
)

const (
	chunkSizeHint = 200 // Number of occurrences per chunk
)

func convertCommand() cli.Command {
	var indexPath, outputPath, cpuProfilePath string

	command := cli.Command{
		Name:  "expt-convert",
		Usage: "[EXPERIMENTAL] Convert a SCIP index to a SQLite database",
		Description: `Converts a SCIP index to a SQLite database.

For inspecting the data, use the SQLite CLI.
For inspecting the schema, use .schema.

Occurrences are stored opaquely as a blob to prevent the DB size from growing very quickly.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "output",
				Usage:       "Path to output SQLite database file",
				Destination: &outputPath,
				Value:       "index.db",
			},
			&cli.StringFlag{
				Name:        "cpu-profile",
				Usage:       "Path to output prof file",
				Destination: &cpuProfilePath,
				Value:       "",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			indexPath = cmd.Args().Get(0)
			if indexPath == "" {
				return errors.New("missing argument for path to SCIP index")
			}

			err := convertMain(indexPath, outputPath, cpuProfilePath, chunkSizeHint)
			if err == nil {
				fmt.Fprintf(
					cmd.Root().Writer,
					"Successfully converted SCIP index to SQLite database at %s\n",
					outputPath)
			}
			return err
		},
	}
	return command
}

func convertMain(indexPath, sqliteDBPath, cpuProfilePath string, chunkSize int) (err error) {
	index, err := readFromOption(indexPath)
	if err != nil {
		return err
	}

	// Create the output directory if it doesn't exist
	outputDir := filepath.Dir(sqliteDBPath)
	if outputDir != "." {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
		}
	}
	file, err := os.Create(sqliteDBPath) // truncates file if present
	if err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		panic(err)
	}

	if cpuProfilePath != "" {
		f, err := os.Create(cpuProfilePath)
		if err != nil {
			log.Fatal(err)
		}
		if err = pprof.StartCPUProfile(f); err != nil {
			log.Fatal(err)
		}
		defer pprof.StopCPUProfile()
	}

	// Create and set up the SQLite database
	db, err := createSQLiteDatabase(sqliteDBPath)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, db.Close())
	}()

	writer, err := zstd.NewWriter(bytes.NewBuffer(nil))
	if err != nil {
		return fmt.Errorf("zstd writer creation: %w", err)
	}
	// Convert the SCIP index to the SQLite database
	converter := NewConverter(db, chunkSize, writer)
	if err := converter.Convert(index); err != nil {
		return err
	}
	if err := prepareIndexes(db); err != nil {
		return err
	}

	return nil
}

func prepareIndexes(conn *sqlite.Conn) error {
	endFn, err := sqlitex.ImmediateTransaction(conn)
	defer endFn(&err)

	indexCreationStatements := []string{
		`CREATE INDEX idx_chunks_line_range ON chunks(document_id, start_line, end_line);`,
		`CREATE INDEX idx_mentions_symbol_id_role ON mentions(symbol_id, role);`,
		`CREATE INDEX idx_defn_enclosing_ranges_symbol_id ON defn_enclosing_ranges(symbol_id);`,
		`CREATE INDEX idx_defn_enclosing_ranges_document ON defn_enclosing_ranges(document_id, start_line, end_line);`,
		`CREATE INDEX idx_chunks_doc_id ON chunks(document_id)`,
		`CREATE INDEX idx_global_symbols_symbol ON global_symbols(symbol);`,
	}
	return executeAll(conn, indexCreationStatements)
}

func createSQLiteDatabase(path string) (conn *sqlite.Conn, err error) {
	// Open a new SQLite database connection
	conn, err = sqlite.OpenConn(path, sqlite.OpenCreate|sqlite.OpenReadWrite|sqlite.OpenWAL)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database at %s: %w", path, err)
	}

	defer func() {
		if err != nil {
			err = errors.Join(err, conn.Close())
		}
	}()

	statements := []string{
		`PRAGMA synchronous = normal;`,
		`PRAGMA temp_store = memory;`,
		`PRAGMA strict = ON;`,
		`PRAGMA foreign_keys = ON;`,
		`PRAGMA journal_mode = WAL;`,
	}

	if err = executeAll(conn, statements); err != nil {
		return conn, err
	}

	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}

	txStatements := []string{
		`CREATE TABLE documents (
			id INTEGER PRIMARY KEY,
			language TEXT,
			relative_path TEXT NOT NULL UNIQUE,
			position_encoding TEXT,
			text TEXT
		);`,
		`CREATE TABLE chunks (
			id INTEGER PRIMARY KEY,
			document_id INTEGER NOT NULL,
			chunk_index INTEGER NOT NULL,
			start_line INTEGER NOT NULL,
			end_line INTEGER NOT NULL,
			occurrences BLOB NOT NULL,
			FOREIGN KEY (document_id) REFERENCES documents(id)
		);`,
		`CREATE TABLE global_symbols (
			id INTEGER PRIMARY KEY,
			symbol TEXT NOT NULL UNIQUE,
			display_name TEXT,
			kind INTEGER,
			documentation TEXT,
			signature BLOB,
			enclosing_symbol TEXT,
			relationships BLOB
		);`,
		`CREATE TABLE mentions (
			chunk_id INTEGER NOT NULL,
			symbol_id INTEGER NOT NULL,
			role INTEGER NOT NULL,
			PRIMARY KEY (chunk_id, symbol_id, role),
			FOREIGN KEY (chunk_id) REFERENCES chunks(id),
			FOREIGN KEY (symbol_id) REFERENCES global_symbols(id)
		);`,
		`CREATE TABLE defn_enclosing_ranges (
			id INTEGER PRIMARY KEY,
			document_id INTEGER NOT NULL,
			symbol_id INTEGER NOT NULL,
			start_line INTEGER NOT NULL,
			start_char INTEGER NOT NULL,
			end_line INTEGER NOT NULL,
			end_char INTEGER NOT NULL,
			FOREIGN KEY (document_id) REFERENCES documents(id),
			FOREIGN KEY (symbol_id) REFERENCES global_symbols(id)
		);`,
	}

	err = executeAll(conn, txStatements)
	return conn, nil
}

func executeAll(conn *sqlite.Conn, statements []string) error {
	for _, stmt := range statements {
		if err := sqlitex.ExecuteTransient(conn, stmt, nil); err != nil {
			return fmt.Errorf("failed to execute statement: %s: %w", stmt, err)
		}
	}
	return nil
}

// Converter handles the conversion from SCIP to SQLite
type Converter struct {
	conn       *sqlite.Conn
	chunkSize  int
	zstdWriter *zstd.Encoder
}

// NewConverter creates a new converter instance
func NewConverter(conn *sqlite.Conn, chunkSize int, writer *zstd.Encoder) *Converter {
	return &Converter{
		conn,
		chunkSize,
		writer,
	}
}

type DocPosition struct {
	Index int
	ID    int64
}

// Convert processes the SCIP index and writes it to the SQLite database
func (c *Converter) Convert(index *scip.Index) (err error) {
	endFn, err := sqlitex.ImmediateTransaction(c.conn)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer endFn(&err)

	docPositions := map[string]DocPosition{}
	symbolToID := map[string]int64{}

	for i, doc := range index.Documents {
		if pos, ok := docPositions[doc.RelativePath]; ok {
			slog.Warn("found multiple documents with identical relative path; ignoring duplicates",
				slog.String("path", doc.RelativePath),
				slog.Int("firstIndex", pos.Index),
				slog.Int("duplicateIndex", i))
			continue
		}
		doc = scip.CanonicalizeDocument(doc)
		index.Documents[i] = doc

		docID, err := c.insertDocument(doc)
		if err != nil {
			return fmt.Errorf("document at index %d: %w", i, err)
		}
		docPositions[doc.RelativePath] = DocPosition{i, docID}

		for _, symbol := range doc.Symbols {
			if symbol.Symbol == "" {
				return fmt.Errorf("empty symbol in SymbolInformation for document %q", doc.RelativePath)
			}
			if scip.IsLocalSymbol(symbol.Symbol) {
				continue
			}
			if _, ok := symbolToID[symbol.Symbol]; ok {
				continue
			}
			symbolID, err := c.insertGlobalSymbols(symbol)
			if err != nil {
				return fmt.Errorf("in document %q: %w", doc.RelativePath, err)
			}
			symbolToID[symbol.Symbol] = symbolID
		}
	}

	for _, docPosition := range docPositions {
		doc := index.Documents[docPosition.Index]
		docID := docPosition.ID
		if err = c.insertEnclosingRangeData(symbolToID, doc.Occurrences, docID); err != nil {
			return fmt.Errorf("in document %q: %w", doc.RelativePath, err)
		}

		if err = c.insertOccurrenceData(doc, docID, symbolToID); err != nil {
			return err
		}
	}

	// We intentionally don't process external symbols

	return nil
}

func (c *Converter) insertOccurrenceData(doc *scip.Document, docID int64, symbolToID map[string]int64) error {
	chunkedOccurrences := chunkOccurrences(doc.Occurrences, c.chunkSize)

	mentionStmt, err := c.conn.Prepare("INSERT INTO mentions (chunk_id, symbol_id, role) VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("failed to prepare mention statement: %w", err)
	}

	for chunkIndex, chunk := range chunkedOccurrences {
		chunkID, err := c.insertChunk(chunk, docID, chunkIndex)
		if err != nil {
			return fmt.Errorf("failed to insert chunk %d: %w", chunkIndex, err)
		}

		// Prepare data for entry into mentions table
		symbolRoles := make(map[string]map[int32]struct{})
		for _, occ := range chunk.Occurrences {
			if scip.IsLocalSymbol(occ.Symbol) {
				continue
			}
			if symbolRoles[occ.Symbol] == nil {
				symbolRoles[occ.Symbol] = make(map[int32]struct{})
			}
			symbolRoles[occ.Symbol][occ.SymbolRoles] = struct{}{}
			if _, ok := symbolToID[occ.Symbol]; !ok {
				symbolToID[occ.Symbol], err = c.insertGlobalSymbols(&scip.SymbolInformation{Symbol: occ.Symbol})
				if err != nil {
					return fmt.Errorf("inserting symbol %q for occurrence: %w", occ.Symbol, err)
				}
			}
		}

		// Add mentions for each symbol in this chunk
		for symbol, roleMap := range symbolRoles {
			symbolID, ok := symbolToID[symbol]
			if !ok {
				panic(fmt.Sprintf("invariant violation: symbol %q not found in symbolToID, "+
					"but expected it to have been inserted when inserting rows for doc.symbols "+
					"or doc.occurrences", symbol))
			}

			for role := range roleMap {
				mentionStmt.BindInt64(1, chunkID)
				mentionStmt.BindInt64(2, symbolID)
				mentionStmt.BindInt64(3, int64(role))

				if _, err = mentionStmt.Step(); err != nil {
					return fmt.Errorf("failed to insert mention for symbol %q with role %d: %w", symbol, role, err)
				}
				if err = mentionStmt.Reset(); err != nil {
					return fmt.Errorf("resetting insert into mentions statement: %w", err)
				}
			}
		}
	}
	return nil
}

func (c *Converter) insertEnclosingRangeData(symbolToID map[string]int64, occs []*scip.Occurrence, docID int64) error {
	defnEnclRangesStmt, err := c.conn.Prepare("INSERT INTO defn_enclosing_ranges (document_id, symbol_id, start_line, start_char, end_line, end_char) VALUES (?, ?, ?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("failed to prepare defn_enclosing_ranges insert statement: %w", err)
	}

	// Look for definition occurrences with enclosing ranges
	for _, occ := range occs {
		// Technically, local symbols could be part of the hierarchy, but
		// ignore them for now
		if !scip.SymbolRole_Definition.Matches(occ) ||
			scip.IsLocalSymbol(occ.Symbol) ||
			len(occ.EnclosingRange) < 3 {
			continue
		}
		symbolID, ok := symbolToID[occ.Symbol]
		if !ok {
			return fmt.Errorf("symbol %q has definition occurrence, but no SymbolInformation", occ.Symbol)
		}
		enclRange, err := scip.NewRange(occ.EnclosingRange)
		if err != nil {
			return fmt.Errorf("bad enclosing range %v for symbol %q: %w", occ.EnclosingRange, occ.Symbol, err)
		}

		defnEnclRangesStmt.BindInt64(1, docID)
		defnEnclRangesStmt.BindInt64(2, symbolID)
		defnEnclRangesStmt.BindInt64(3, int64(enclRange.Start.Line))
		defnEnclRangesStmt.BindInt64(4, int64(enclRange.Start.Character))
		defnEnclRangesStmt.BindInt64(5, int64(enclRange.End.Line))
		defnEnclRangesStmt.BindInt64(6, int64(enclRange.End.Character))

		_, err = defnEnclRangesStmt.Step()
		if err != nil {
			return fmt.Errorf("insert into defn_enclosing_ranges for symbol %q: %w", occ.Symbol, err)
		}

		if err = defnEnclRangesStmt.Reset(); err != nil {
			return fmt.Errorf("resetting prepared statement for defn_enclosing_ranges: %w", err)
		}
	}
	return nil
}

func (c *Converter) insertGlobalSymbols(symbol *scip.SymbolInformation) (symbolID int64, err error) {
	documentation := strings.Join(symbol.Documentation, "\n")

	insertStmt, err := c.conn.Prepare(
		`INSERT INTO global_symbols (symbol, display_name, kind, documentation, enclosing_symbol)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(symbol) DO NOTHING
		RETURNING id`)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare symbol insert statement: %w", err)
	}

	insertStmt.BindText(1, symbol.Symbol)
	if symbol.DisplayName == "" {
		insertStmt.BindNull(2)
	} else {
		insertStmt.BindText(2, symbol.DisplayName)
	}
	if symbol.Kind == scip.SymbolInformation_UnspecifiedKind {
		insertStmt.BindNull(3)
	} else {
		insertStmt.BindInt64(3, int64(symbol.Kind))
	}
	if documentation == "" {
		insertStmt.BindNull(4)
	} else {
		insertStmt.BindText(4, documentation)
	}
	if symbol.EnclosingSymbol == "" {
		insertStmt.BindNull(5)
	} else {
		insertStmt.BindText(5, symbol.EnclosingSymbol)
	}

	if _, err = insertStmt.Step(); err != nil {
		return 0, fmt.Errorf("failed to insert symbol %s: %w", symbol.Symbol, err)
	}
	if c.conn.Changes() == 0 {
		log.Fatal(fmt.Errorf("expected map to de-duplicate inserts eagerly, but attempted to insert symbol %q"+
			"despite it existing for ID: %d", symbol.Symbol, symbolID))
	}
	symbolID = insertStmt.ColumnInt64(0)
	err = insertStmt.Reset()
	return symbolID, err
}

func (c *Chunk) toDBFormat(encoder *zstd.Encoder) ([]byte, error) {
	occurrencesBlob, err := proto.Marshal(&scip.Document{
		Occurrences: c.Occurrences,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to serialize occurrences: %w", err)
	}

	var buf bytes.Buffer
	encoder.Reset(&buf)
	if _, err = encoder.Write(occurrencesBlob); err != nil {
		return nil, fmt.Errorf("compression error: %w", err)
	}
	if err = encoder.Close(); err != nil {
		return nil, fmt.Errorf("flushing encoder: %w", err)
	}
	return buf.Bytes(), nil
}

func (c *Chunk) fromDBFormat(reader *bytes.Reader, decoder *zstd.Decoder) error {
	if err := decoder.Reset(reader); err != nil {
		return fmt.Errorf("resetting zstd Decoder: %w", err)
	}
	protoBytes, err := io.ReadAll(decoder)
	if err != nil {
		return fmt.Errorf("reading compressed data: %w", err)
	}

	var tmpDoc scip.Document
	if err = proto.Unmarshal(protoBytes, &tmpDoc); err != nil {
		return fmt.Errorf("failed to unmarshal occurrences: %w", err)
	}
	c.Occurrences = tmpDoc.Occurrences
	return nil
}

func (c *Converter) insertChunk(chunk Chunk, docID int64, chunkIndex int) (chunkID int64, err error) {
	compressedOccurrences, err := chunk.toDBFormat(c.zstdWriter)
	if err != nil {
		return 0, fmt.Errorf("failed to serialize chunk: %w", err)
	}

	chunkStmt, err := c.conn.Prepare(
		`INSERT INTO chunks (document_id, chunk_index, start_line, end_line, occurrences)
		VALUES (?, ?, ?, ?, ?) RETURNING id`)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare chunk statement: %w", err)
	}

	chunkStmt.BindInt64(1, docID)
	chunkStmt.BindInt64(2, int64(chunkIndex))
	chunkStmt.BindInt64(3, int64(chunk.StartLine))
	chunkStmt.BindInt64(4, int64(chunk.EndLine))
	chunkStmt.BindBytes(5, compressedOccurrences)

	_, err = chunkStmt.Step()
	if err != nil {
		return 0, fmt.Errorf("failed to insert chunk: %w", err)
	}
	chunkID = chunkStmt.ColumnInt64(0)
	err = chunkStmt.Reset()
	return chunkID, err
}

func (c *Converter) insertDocument(doc *scip.Document) (docID int64, err error) {
	docStmt, err := c.conn.Prepare(
		`INSERT INTO documents (language, relative_path, position_encoding, text)
		VALUES (?, ?, ?, ?)
		RETURNING id`)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare document statement: %w", err)
	}

	if doc.Language == "" {
		docStmt.BindNull(1)
	} else {
		docStmt.BindText(1, doc.Language)
	}
	if doc.RelativePath == "" {
		return 0, errors.New("relative path must not be empty")
	}
	docStmt.BindText(2, doc.RelativePath)
	switch doc.PositionEncoding {
	case scip.PositionEncoding_UnspecifiedPositionEncoding:
		docStmt.BindNull(3)
	case scip.PositionEncoding_UTF8CodeUnitOffsetFromLineStart:
		docStmt.BindText(3, "UTF-8")
	case scip.PositionEncoding_UTF16CodeUnitOffsetFromLineStart:
		docStmt.BindText(3, "UTF-16")
	case scip.PositionEncoding_UTF32CodeUnitOffsetFromLineStart:
		docStmt.BindText(3, "UTF-32")
	default:
		return 0, fmt.Errorf("unknown position encoding %d", doc.PositionEncoding)
	}
	if doc.Text == "" {
		docStmt.BindNull(4)
	} else {
		docStmt.BindText(4, doc.Text)
	}

	if _, err = docStmt.Step(); err != nil {
		return 0, fmt.Errorf("failed to insert document %s: %w", doc.RelativePath, err)
	}
	docID = docStmt.ColumnInt64(0)
	err = docStmt.Reset()
	return docID, err
}

type Chunk struct {
	StartLine   int32
	EndLine     int32
	Occurrences []*scip.Occurrence
}

// chunkOccurrences splits occurrences into chunks of the specified size
func chunkOccurrences(occurrences []*scip.Occurrence, chunkSize int) []Chunk {
	if len(occurrences) == 0 {
		return nil
	}

	chunks := make([]Chunk, 0, len(occurrences)/chunkSize)
	hi := min(len(occurrences), chunkSize)
	for lo := 0; lo < hi && hi <= len(occurrences); {
		for ; hi <= len(occurrences)-1 &&
			scip.NewRangeUnchecked(occurrences[hi-1].Range).Start.Line ==
				scip.NewRangeUnchecked(occurrences[hi].Range).Start.Line; hi++ {
		}
		chunks = append(chunks, Chunk{
			StartLine:   scip.NewRangeUnchecked(occurrences[lo].Range).Start.Line,
			EndLine:     scip.NewRangeUnchecked(occurrences[hi-1].Range).Start.Line,
			Occurrences: occurrences[lo:hi],
		})

		lo = hi
		hi = min(len(occurrences), hi+chunkSize)
	}

	return chunks
}
