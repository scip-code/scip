package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/scip-code/scip/bindings/go/scip/testutil"
)

type testFlags struct {
	from           string // default: 'index.scip'
	commentSyntax  string // default: '//'
	pathFilters    []string
	checkDocuments bool // default: false
}

func testCommand() cli.Command {
	var testFlags testFlags

	test := cli.Command{
		Name:  "test",
		Usage: "Validate a SCIP index against test files",
		Description: fmt.Sprintf(`Validates whether the SCIP data present in an index
matches that specified in human-readable test files, using syntax
similar to the 'snapshot' subcommand. Test file syntax reference:

    https://github.com/scip-code/scip/blob/v%s/docs/test_file_format.md

The test files are located based on the relative_path field
in the SCIP document, interpreted relative to the the directory
the CLI is invoked in.

If you want to instead check all the data in a SCIP index,
use the 'snapshot' subcommand.`, version),
		Flags: []cli.Flag{
			fromFlag(&testFlags.from),
			commentSyntaxFlag(&testFlags.commentSyntax),
			&cli.StringSliceFlag{
				Name:        "filter",
				Aliases:     []string{"f"},
				Usage:       "Explicit list of test files to check. Can be specified multiple times. If not specified, all files are tested.",
				Destination: &testFlags.pathFilters,
			},
			&cli.BoolFlag{
				Name:        "check-documents",
				Usage:       "Whether or not to validate whether every file in the test directory has a correlating document in the SCIP index.",
				Destination: &testFlags.checkDocuments,
				Value:       false,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			dir := cmd.Args().Get(0)

			index, err := readFromOption(testFlags.from)
			if err != nil {
				return err
			}

			if err = testutil.RunTests(
				dir,
				testFlags.pathFilters,
				testFlags.checkDocuments,
				index,
				testFlags.commentSyntax,
				os.Stdout,
			); err != nil {
				return cli.Exit("", 1)
			}
			return nil
		},
	}
	return test
}
