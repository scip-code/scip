package repro

import (
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"

	reproGrammar "github.com/scip-code/scip/reprolang/grammar"

	"github.com/scip-code/scip/bindings/go/scip"
)

func parseSourceFile(source *scip.SourceFile) (*reproSourceFile, error) {
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(sitter.NewLanguage(reproGrammar.Language())); err != nil {
		return nil, err
	}
	tree := parser.Parse([]byte(source.Text), nil)
	reproSource := newSourceFile(source, tree.RootNode(), tree)
	reproSource.loadStatements()
	return reproSource, nil
}

func (s *reproSourceFile) loadStatements() {
	for i := uint(0); i < s.node.NamedChildCount(); i++ {
		child := s.node.NamedChild(i)
		switch child.Kind() {
		case "definition_statement":
			s.definitions = append(s.definitions, &definitionStatement{
				docstring: s.parseDocstring(child),
				name:      newIdentifier(s, child.ChildByFieldName("name")),
				relations: s.parseRelations(child),
			})
		case "relationships_statement":
			s.relationships = append(s.relationships, &relationshipsStatement{
				name:      newIdentifier(s, child.ChildByFieldName("name")),
				relations: s.parseRelations(child),
			})
		case "reference_statement":
			s.references = append(s.references, &referenceStatement{
				name:         newIdentifier(s, child.ChildByFieldName("name")),
				isForwardDef: child.ChildByFieldName("forward_definition") != nil,
			})
		}
	}
}

func (s *reproSourceFile) parseDocstring(node *sitter.Node) string {
	docstring := node.ChildByFieldName("docstring")
	if docstring == nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(s.nodeText(docstring), "# docstring:"))
}

func (s *reproSourceFile) parseRelations(node *sitter.Node) relationships {
	rels := relationships{}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		switch child.Kind() {
		case "implements":
			rels.implementsRelation = newIdentifier(s, child.ChildByFieldName("name"))
		case "type_defines":
			rels.typeDefinesRelation = newIdentifier(s, child.ChildByFieldName("name"))
		case "references":
			rels.referencesRelation = newIdentifier(s, child.ChildByFieldName("name"))
		case "defined_by":
			rels.definedByRelation = newIdentifier(s, child.ChildByFieldName("name"))
		}
	}
	return rels
}
