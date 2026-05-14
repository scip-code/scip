package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strings"

	"github.com/urfave/cli/v3"
)

func main() {
	app := scipApp()
	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func commands() []*cli.Command {
	lint := lintCommand()
	print := printCommand()
	snapshot := snapshotCommand()
	stats := statsCommand()
	test := testCommand()
	convert := convertCommand()
	return []*cli.Command{&lint, &print, &snapshot, &stats, &test, &convert}
}

//go:embed version.txt
var version string

var Reproducible = "" // set by ldflags in CI

func gitSuffix() string {
	if Reproducible != "" {
		return ""
	}
	var rev, timestamp string
	clean := "true"
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		for _, kv := range buildInfo.Settings {
			switch kv.Key {
			case "vcs.revision":
				rev = kv.Value
			case "vcs.time":
				timestamp = kv.Value
			case "vcs.modified":
				if kv.Value == "true" {
					clean = "false"
				}
			}
		}
	}
	return fmt.Sprintf("-dev\nSHA: %s\ntimestamp: %s\nclean: %s", rev, timestamp, clean)
}

func scipApp() *cli.Command {
	app := &cli.Command{
		Name:        "scip",
		Version:     fmt.Sprintf("v%s%s", strings.TrimSpace(version), gitSuffix()),
		Usage:       "SCIP Code Intelligence Protocol CLI",
		Description: "For more details, see the project README at:\n\n\thttps://github.com/scip-code/scip",
		Commands:    commands(),
	}
	return app
}

func fromFlag(storage *string) *cli.StringFlag {
	return &cli.StringFlag{
		Name:        "from",
		Usage:       "Path to SCIP index file",
		Destination: storage,
		Value:       "index.scip",
	}
}

func commentSyntaxFlag(storage *string) *cli.StringFlag {
	return &cli.StringFlag{
		Name:        "comment-syntax",
		Usage:       "Comment syntax to use for snapshot files",
		Destination: storage,
		Value:       "//",
	}
}

func projectRootFlag(storage *string) *cli.StringFlag {
	return &cli.StringFlag{
		Name: "project-root",
		Usage: "Override project root in the SCIP file. " +
			"This can be helpful when the SCIP index was created on another computer",
		Destination: storage,
	}
}
