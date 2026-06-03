package grammar

import (
	"testing"

	"github.com/stretchr/testify/assert"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestParse(t *testing.T) {
	assert := assert.New(t)

	parser := sitter.NewParser()
	defer parser.Close()
	err := parser.SetLanguage(sitter.NewLanguage(Language()))
	assert.NoError(err)
	tree := parser.Parse([]byte("definition a implements b\n"), nil)
	defer tree.Close()
	assert.Equal(
		"(source_file (definition_statement name: (identifier (name)) relations: (implements name: (identifier (name)))))",
		tree.RootNode().ToSexp(),
	)
}
