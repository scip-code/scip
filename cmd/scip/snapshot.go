package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/scip-code/scip/bindings/go/scip"
	"github.com/scip-code/scip/bindings/go/scip/testutil"
)

type snapshotFlags struct {
	from              string
	output            string
	customProjectRoot string
	strict            bool
	commentSyntax     string
}

func snapshotCommand() cli.Command {
	var snapshotFlags snapshotFlags
	snapshot := cli.Command{
		Name:  "snapshot",
		Usage: "Generate snapshot files for golden testing",
		Description: `The snapshot subcommand generates snapshot files which
can be use for inspecting the output of an index in a
visual way. Occurrences are marked with caret signs (^)
and symbol information.

For testing a SCIP indexer, you can either use this subcommand
along with 'git diff' or equivalent, or you can use the dedicated
'test' subcommand for more targeted checks.
`,
		Flags: []cli.Flag{
			fromFlag(&snapshotFlags.from),
			&cli.StringFlag{
				Name:        "to",
				Usage:       "Path to output directory for snapshot files",
				Destination: &snapshotFlags.output,
				Value:       "scip-snapshot",
			},
			projectRootFlag(&snapshotFlags.customProjectRoot),
			&cli.BoolFlag{
				Name:        "strict",
				Usage:       "If true, fail fast on errors",
				Destination: &snapshotFlags.strict,
				Value:       true,
			},
			commentSyntaxFlag(&snapshotFlags.commentSyntax),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return snapshotMain(snapshotFlags)
		},
	}
	return snapshot
}

func snapshotMain(flags snapshotFlags) error {
	from := flags.from
	index, err := readFromOption(from)
	if err != nil {
		return err
	}
	output := flags.output
	err = os.RemoveAll(output)
	if err != nil {
		return err
	}
	symbolFormatter := scip.LenientVerboseSymbolFormatter
	if flags.strict {
		symbolFormatter = scip.VerboseSymbolFormatter
		symbolFormatter.OnError = func(err error) error {
			return fmt.Errorf("use --strict=false to ignore this error: %w", err)
		}
	}
	snapshots, err := testutil.FormatSnapshots(index, flags.commentSyntax, symbolFormatter, flags.customProjectRoot)
	if err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		outputPath := filepath.Join(output, snapshot.RelativePath)
		err = os.MkdirAll(filepath.Dir(outputPath), 0755)
		if err != nil {
			return err
		}
		err = os.WriteFile(outputPath, []byte(snapshot.Text), 0755)
		if err != nil {
			return err
		}
	}
	fmt.Println("done: " + output)
	return nil
}
