package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/urfave/cli/v3"
	"google.golang.org/protobuf/proto"

	"github.com/scip-code/scip/bindings/go/scip"
)

type mergeFlags struct {
	output            string
	projectRootOverride string
}

func mergeCommand() cli.Command {
	var flags mergeFlags
	command := cli.Command{
		Name:  "merge",
		Usage: "Merge multiple SCIP indexes into a single SCIP index",
		Description: `Merges two or more SCIP indexes into one.

The output project_root is inferred as the common URI ancestor of the input
indexes' project_root values. Each input document's relative_path is rewritten
to be relative to that common root.

For example, given:

  a.scip with project_root file:///repo/frontend, document src/a.ts
  b.scip with project_root file:///repo/backend,  document src/b.go

The merged index will have:

  project_root file:///repo
  documents frontend/src/a.ts and backend/src/b.go

Documents and symbols are deduplicated after rewriting.

Use --project-root to override the inferred root (each input's root must then
be a descendant of the override).

Example usage:

  scip merge --output merged.scip a.scip b.scip c.scip`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "output",
				Usage:       "Path to write the merged SCIP index",
				Destination: &flags.output,
				Value:       "index.scip",
			},
			&cli.StringFlag{
				Name:        "project-root",
				Usage:       "Override the inferred output project_root URI. Each input project_root must be a descendant of this URI.",
				Destination: &flags.projectRootOverride,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			inputs := cmd.Args().Slice()
			if len(inputs) == 0 {
				return errors.New("at least one input SCIP index path is required")
			}
			return mergeMain(inputs, flags)
		},
	}
	return command
}

func mergeMain(inputs []string, flags mergeFlags) error {
	indexes := make([]*scip.Index, 0, len(inputs))
	for _, p := range inputs {
		idx, err := readFromOption(p)
		if err != nil {
			return fmt.Errorf("reading %s: %w", p, err)
		}
		indexes = append(indexes, idx)
	}

	merged, err := mergeIndexes(indexes, flags.projectRootOverride)
	if err != nil {
		return err
	}

	bytes, err := proto.Marshal(merged)
	if err != nil {
		return fmt.Errorf("marshaling merged index: %w", err)
	}
	if err := os.WriteFile(flags.output, bytes, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", flags.output, err)
	}
	return nil
}

// mergeIndexes combines multiple SCIP indexes into one. It determines the
// output project_root by computing the common URI ancestor of the inputs'
// project_root values (unless projectRootOverride is non-empty), rewrites
// each Document.relative_path to be relative to that root, and deduplicates
// documents and external symbols.
func mergeIndexes(indexes []*scip.Index, projectRootOverride string) (*scip.Index, error) {
	if len(indexes) == 0 {
		return nil, errors.New("no indexes to merge")
	}

	// Validate metadata is present everywhere.
	for i, idx := range indexes {
		if idx.Metadata == nil {
			return nil, fmt.Errorf("index %d has no metadata", i)
		}
	}

	// Validate compatible ProtocolVersion and TextDocumentEncoding.
	protoVersion := indexes[0].Metadata.Version
	encoding := indexes[0].Metadata.TextDocumentEncoding
	for i, idx := range indexes[1:] {
		if idx.Metadata.Version != protoVersion {
			return nil, fmt.Errorf(
				"index %d has incompatible protocol version %v (expected %v)",
				i+1, idx.Metadata.Version, protoVersion)
		}
		if !encodingsCompatible(encoding, idx.Metadata.TextDocumentEncoding) {
			return nil, fmt.Errorf(
				"index %d has incompatible text encoding %v (expected %v)",
				i+1, idx.Metadata.TextDocumentEncoding, encoding)
		}
		// Promote from Unspecified to a concrete encoding if one input has it.
		if encoding == scip.TextEncoding_UnspecifiedTextEncoding {
			encoding = idx.Metadata.TextDocumentEncoding
		}
	}

	// Determine the output project_root and per-input prefix.
	outputRoot, prefixes, err := computeOutputRootAndPrefixes(indexes, projectRootOverride)
	if err != nil {
		return nil, err
	}

	// Aggregate documents (rewriting paths) and external symbols.
	var allDocuments []*scip.Document
	var allExternalSymbols []*scip.SymbolInformation
	for i, idx := range indexes {
		for _, doc := range idx.Documents {
			if prefixes[i] != "" {
				if doc.RelativePath == "" {
					doc.RelativePath = prefixes[i]
				} else {
					doc.RelativePath = path.Join(prefixes[i], doc.RelativePath)
				}
			}
			allDocuments = append(allDocuments, doc)
		}
		allExternalSymbols = append(allExternalSymbols, idx.ExternalSymbols...)
	}

	mergedDocuments := scip.FlattenDocuments(allDocuments)
	mergedExternalSymbols := scip.FlattenSymbols(allExternalSymbols)

	return &scip.Index{
		Metadata: &scip.Metadata{
			Version: protoVersion,
			ToolInfo: &scip.ToolInfo{
				Name:      "scip",
				Version:   strings.TrimSpace(version),
				Arguments: []string{"merge"},
			},
			ProjectRoot:          outputRoot,
			TextDocumentEncoding: encoding,
		},
		Documents:       mergedDocuments,
		ExternalSymbols: mergedExternalSymbols,
	}, nil
}

// encodingsCompatible reports whether two TextEncoding values can be merged.
// Unspecified is treated as compatible with any value.
func encodingsCompatible(a, b scip.TextEncoding) bool {
	if a == scip.TextEncoding_UnspecifiedTextEncoding ||
		b == scip.TextEncoding_UnspecifiedTextEncoding {
		return true
	}
	return a == b
}

// computeOutputRootAndPrefixes returns the URI to use as the merged index's
// project_root, together with one path-prefix per input index. The prefix for
// input i is what should be prepended to each document's relative_path so that
// it remains relative to the output root.
//
// If projectRootOverride is non-empty, it is used directly and each input root
// must be a descendant of it. Otherwise, the output root is the common URI
// ancestor of all inputs.
func computeOutputRootAndPrefixes(indexes []*scip.Index, projectRootOverride string) (string, []string, error) {
	roots := make([]string, len(indexes))
	for i, idx := range indexes {
		roots[i] = idx.Metadata.ProjectRoot
	}

	var outputRoot string
	if projectRootOverride != "" {
		outputRoot = projectRootOverride
	} else {
		var err error
		outputRoot, err = commonAncestorURI(roots)
		if err != nil {
			return "", nil, err
		}
	}

	prefixes := make([]string, len(indexes))
	for i, r := range roots {
		prefix, err := relativePrefix(outputRoot, r)
		if err != nil {
			return "", nil, fmt.Errorf(
				"index %d project_root %q is not under output root %q: %w",
				i, r, outputRoot, err)
		}
		prefixes[i] = prefix
	}
	return outputRoot, prefixes, nil
}

// commonAncestorURI returns the longest URI that is an ancestor of every input
// URI. All inputs must share the same scheme and host.
func commonAncestorURI(roots []string) (string, error) {
	if len(roots) == 0 {
		return "", errors.New("no project roots provided")
	}

	first, err := url.Parse(roots[0])
	if err != nil {
		return "", fmt.Errorf("parsing project_root %q: %w", roots[0], err)
	}

	commonScheme := first.Scheme
	commonHost := first.Host
	commonPath := normalizeURIPath(first.Path)

	for _, r := range roots[1:] {
		u, err := url.Parse(r)
		if err != nil {
			return "", fmt.Errorf("parsing project_root %q: %w", r, err)
		}
		if u.Scheme != commonScheme {
			return "", fmt.Errorf(
				"incompatible URI schemes %q and %q across inputs; "+
					"pass --project-root to override",
				commonScheme, u.Scheme)
		}
		if u.Host != commonHost {
			return "", fmt.Errorf(
				"incompatible URI hosts %q and %q across inputs; "+
					"pass --project-root to override",
				commonHost, u.Host)
		}
		commonPath = commonPathPrefix(commonPath, normalizeURIPath(u.Path))
	}

	out := url.URL{
		Scheme: commonScheme,
		Host:   commonHost,
		Path:   commonPath,
	}
	return out.String(), nil
}

// normalizeURIPath trims trailing slashes while preserving the root "/".
func normalizeURIPath(p string) string {
	if p == "" || p == "/" {
		return p
	}
	return strings.TrimRight(p, "/")
}

// commonPathPrefix returns the longest path prefix of a and b along "/"
// component boundaries.
func commonPathPrefix(a, b string) string {
	aParts := strings.Split(a, "/")
	bParts := strings.Split(b, "/")
	n := len(aParts)
	if len(bParts) < n {
		n = len(bParts)
	}
	i := 0
	for ; i < n; i++ {
		if aParts[i] != bParts[i] {
			break
		}
	}
	common := strings.Join(aParts[:i], "/")
	// If only the leading empty element matched, both paths were absolute and
	// the only shared ancestor is the root.
	if common == "" && i > 0 && strings.HasPrefix(a, "/") && strings.HasPrefix(b, "/") {
		return "/"
	}
	return common
}

// relativePrefix returns the path that, when prepended to a relative_path under
// the input root, makes it relative to the output root.
func relativePrefix(outputRoot, inputRoot string) (string, error) {
	outURL, err := url.Parse(outputRoot)
	if err != nil {
		return "", fmt.Errorf("parsing output project_root %q: %w", outputRoot, err)
	}
	inURL, err := url.Parse(inputRoot)
	if err != nil {
		return "", fmt.Errorf("parsing input project_root %q: %w", inputRoot, err)
	}
	if outURL.Scheme != inURL.Scheme {
		return "", fmt.Errorf("scheme mismatch: %q vs %q", outURL.Scheme, inURL.Scheme)
	}
	if outURL.Host != inURL.Host {
		return "", fmt.Errorf("host mismatch: %q vs %q", outURL.Host, inURL.Host)
	}

	outPath := normalizeURIPath(outURL.Path)
	inPath := normalizeURIPath(inURL.Path)

	if outPath == inPath {
		return "", nil
	}
	// outPath must be a directory prefix of inPath.
	if outPath == "/" {
		return strings.TrimLeft(inPath, "/"), nil
	}
	if !strings.HasPrefix(inPath, outPath+"/") {
		return "", fmt.Errorf("%q is not under %q", inPath, outPath)
	}
	return strings.TrimPrefix(inPath, outPath+"/"), nil
}
