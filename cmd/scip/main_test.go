package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	Reproducible = "true"
	os.Exit(m.Run())
}

func TestCLIReferenceInSync(t *testing.T) {
	app := scipApp()
	readmeBytes, err := os.ReadFile(filepath.Join("..", "..", "docs", "CLI.md"))
	require.Nil(t, err)
	readme := string(readmeBytes)

	check := func(args []string, commandName string) {
		r, w, err := os.Pipe()
		require.Nil(t, err)
		origStdout := os.Stdout
		os.Stdout = w
		defer func() { os.Stdout = origStdout }()

		require.Nil(t, app.Run(context.Background(), args))
		_, err = w.Write([]byte{0}) // needed for Read below to terminate
		require.Nil(t, err)
		helpBytes := make([]byte, 1024*1024)
		n, err := r.Read(helpBytes)
		helpBytes = helpBytes[:n-1] // ignore NULL at end
		require.Nil(t, err)
		help := strings.TrimSpace(string(helpBytes))
		require.Truef(t, strings.Contains(readme, help),
			"docs/CLI.md missing help text %s for %s.\nRun `%s` and paste the output in the Readme.",
			help, commandName, strings.Join(append([]string{"scip"}, args...), " "))
	}

	check([]string{"--help"}, "scip")

	for _, command := range commands() {
		check([]string{command.Name, "--help"}, fmt.Sprintf("subcommand %s", command.Name))
	}
}
