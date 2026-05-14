package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/scip-code/scip/bindings/go/scip"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestJSONPrinting(t *testing.T) {
	app := scipApp()
	// Redirect CLI writer to a buffer
	var buff bytes.Buffer
	app.Writer = io.Writer(&buff)
	idx := makeIndex([]string{"a"}, stringMap{"f": {"b"}}, stringMap{"f": {"a", "b"}})

	idx.Metadata = &scip.Metadata{ProjectRoot: "howdy"}

	// Serialise SCIP index
	indexBytes, err := proto.Marshal(idx)

	// Write SCIP index to a temp file
	dir := os.TempDir()
	file, err := ioutil.TempFile(dir, "scip-cli-json-test*.scip")
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(file.Name())

	_, fErr := file.Write(indexBytes)
	if fErr != nil {
		log.Fatal(fErr)
	}

	// Run the JSON command with the temporary file
	runErr := app.Run(
		context.Background(), []string{"scip", "print", "--json", file.Name()})
	if runErr != nil {
		log.Fatal(runErr)
	}

	type JsonIndex struct {
		Metadata struct {
			ProjectRoot string `json:"project_root"`
		}
		Documents []struct {
			RelativePath string `json:"relative_path"`
		} `json:"documents"`
	}

	var roundtripResult JsonIndex

	bytes := buff.Bytes()

	jsonErr := json.Unmarshal(bytes, &roundtripResult)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	require.Equal(t, "howdy", roundtripResult.Metadata.ProjectRoot)
	require.Equal(t, 1, len(roundtripResult.Documents))
	require.Equal(t, "f", roundtripResult.Documents[0].RelativePath)
}
