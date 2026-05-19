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
	output              string
	projectRootOverride string
}

func mergeCommand() cli.Command {
	var flags mergeFlags
	return cli.Command{
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
}

func mergeMain(inputs []string, flags mergeFlags) error {
	indexes := make([]*scip.Index, len(inputs))
	for i, p := range inputs {
		idx, err := readFromOption(p)
		if err != nil {
			return fmt.Errorf("reading %s: %w", p, err)
		}
		indexes[i] = idx
	}

	merged, err := mergeIndexes(indexes, flags.projectRootOverride)
	if err != nil {
		return err
	}

	data, err := proto.Marshal(merged)
	if err != nil {
		return fmt.Errorf("marshaling merged index: %w", err)
	}
	if err := os.WriteFile(flags.output, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", flags.output, err)
	}
	return nil
}

// mergeIndexes combines multiple SCIP indexes into one. It determines the
// output project_root as the common URI ancestor of the inputs' project_root
// values (or projectRootOverride if non-empty), rewrites each
// Document.relative_path to be relative to that root, and deduplicates
// documents and external symbols.
func mergeIndexes(indexes []*scip.Index, projectRootOverride string) (*scip.Index, error) {
	if len(indexes) == 0 {
		return nil, errors.New("no indexes to merge")
	}

	// Validate metadata and pick the merged ProtocolVersion / TextEncoding.
	protoVersion := indexes[0].Metadata.GetVersion()
	encoding := scip.TextEncoding_UnspecifiedTextEncoding
	roots := make([]*url.URL, len(indexes))
	for i, idx := range indexes {
		if idx.Metadata == nil {
			return nil, fmt.Errorf("index %d has no metadata", i)
		}
		if idx.Metadata.Version != protoVersion {
			return nil, fmt.Errorf(
				"index %d has incompatible protocol version %v (expected %v)",
				i, idx.Metadata.Version, protoVersion)
		}
		if e := idx.Metadata.TextDocumentEncoding; e != scip.TextEncoding_UnspecifiedTextEncoding {
			if encoding == scip.TextEncoding_UnspecifiedTextEncoding {
				encoding = e
			} else if encoding != e {
				return nil, fmt.Errorf(
					"index %d has incompatible text encoding %v (expected %v)",
					i, e, encoding)
			}
		}
		u, err := parseRootURI(idx.Metadata.ProjectRoot)
		if err != nil {
			return nil, fmt.Errorf("index %d: %w", i, err)
		}
		roots[i] = u
	}

	// Determine the output project_root and the per-input prefix.
	outputURL, prefixes, err := planPaths(roots, projectRootOverride)
	if err != nil {
		return nil, err
	}

	// Aggregate documents (rewriting relative_path) and external symbols.
	var allDocuments []*scip.Document
	var allExternalSymbols []*scip.SymbolInformation
	for i, idx := range indexes {
		for _, doc := range idx.Documents {
			if prefixes[i] != "" {
				doc.RelativePath = path.Join(prefixes[i], doc.RelativePath)
			}
			allDocuments = append(allDocuments, doc)
		}
		allExternalSymbols = append(allExternalSymbols, idx.ExternalSymbols...)
	}

	return &scip.Index{
		Metadata: &scip.Metadata{
			Version: protoVersion,
			ToolInfo: &scip.ToolInfo{
				Name:      "scip",
				Version:   strings.TrimSpace(version),
				Arguments: []string{"merge"},
			},
			ProjectRoot:          outputURL.String(),
			TextDocumentEncoding: encoding,
		},
		Documents:       scip.FlattenDocuments(allDocuments),
		ExternalSymbols: scip.FlattenSymbols(allExternalSymbols),
	}, nil
}

// parseRootURI parses a project_root URI and normalizes its path component
// (trailing slashes trimmed, "." / ".." resolved, "" replaced with "/").
func parseRootURI(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing project_root %q: %w", raw, err)
	}
	if u.Path == "" {
		u.Path = "/"
	} else {
		u.Path = path.Clean(u.Path)
	}
	return u, nil
}

// planPaths returns the output project_root URL and the prefix that must be
// prepended to each input's document relative_paths. If override is non-empty
// it becomes the output root and every input root must be a descendant of it;
// otherwise the output root is the common URI ancestor of all inputs.
func planPaths(inputs []*url.URL, override string) (*url.URL, []string, error) {
	var root *url.URL
	if override != "" {
		u, err := parseRootURI(override)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid --project-root: %w", err)
		}
		root = u
	} else {
		root = inputs[0]
		for _, u := range inputs[1:] {
			if u.Scheme != root.Scheme || u.Host != root.Host {
				return nil, nil, fmt.Errorf(
					"inputs have incompatible URI scheme/host: %q vs %q; "+
						"pass --project-root to override",
					root, u)
			}
			root = &url.URL{Scheme: root.Scheme, Host: root.Host, Path: commonPath(root.Path, u.Path)}
		}
	}

	prefixes := make([]string, len(inputs))
	for i, u := range inputs {
		if u.Scheme != root.Scheme || u.Host != root.Host {
			return nil, nil, fmt.Errorf(
				"index %d project_root %q has different scheme/host than output root %q",
				i, u, root)
		}
		rel, err := relativeTo(root.Path, u.Path)
		if err != nil {
			return nil, nil, fmt.Errorf("index %d project_root %q: %w", i, u, err)
		}
		prefixes[i] = rel
	}
	return root, prefixes, nil
}

// commonPath returns the longest slash-bounded ancestor of both a and b.
// Inputs must be absolute (start with "/") and cleaned.
func commonPath(a, b string) string {
	for !isAncestor(a, b) {
		a = path.Dir(a)
	}
	return a
}

// isAncestor reports whether parent is an ancestor of (or equal to) child.
// Inputs must be cleaned.
func isAncestor(parent, child string) bool {
	if parent == "/" {
		return strings.HasPrefix(child, "/")
	}
	return child == parent || strings.HasPrefix(child, parent+"/")
}

// relativeTo returns child's path relative to parent (no leading slash).
// Returns an error if parent is not an ancestor of child.
func relativeTo(parent, child string) (string, error) {
	if child == parent {
		return "", nil
	}
	if parent == "/" {
		return strings.TrimPrefix(child, "/"), nil
	}
	if rel, ok := strings.CutPrefix(child, parent+"/"); ok {
		return rel, nil
	}
	return "", fmt.Errorf("%q is not under %q", child, parent)
}
