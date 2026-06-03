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
	for i := uint(0); i < s.node.ChildCount(); i++ {
		child := s.node.Child(i)
		name := child.ChildByFieldName("name")
		if name == nil {
			continue
		}
		switch child.Kind() {
		case "relationships_statement", "definition_statement":
			docstring := ""
			docstringNode := child.ChildByFieldName("docstring")
			if docstringNode != nil {
				docstring = strings.TrimSpace(strings.TrimPrefix(s.nodeText(docstringNode), "# docstring:"))
			}
			name := newIdentifier(s, child.ChildByFieldName("name"))
			relations := relationships{}
			for i := uint(0); i < child.NamedChildCount(); i++ {
				relation := child.NamedChild(i)
				switch relation.Kind() {
				case "implements":
					relations.implementsRelation = newIdentifier(s, relation.ChildByFieldName("name"))
				case "type_defines":
					relations.typeDefinesRelation = newIdentifier(s, relation.ChildByFieldName("name"))
				case "references":
					relations.referencesRelation = newIdentifier(s, relation.ChildByFieldName("name"))
				case "defined_by":
					relations.definedByRelation = newIdentifier(s, relation.ChildByFieldName("name"))
				}
			}
			if child.Kind() == "definition_statement" {
				s.definitions = append(s.definitions, &definitionStatement{
					docstring: docstring,
					name:      name,
					relations: relations,
				})
			} else {
				s.relationships = append(s.relationships, &relationshipsStatement{
					name:      name,
					relations: relations,
				})
			}
		case "reference_statement":
			isForwardDef := false
			for i := uint(0); i < child.NamedChildCount(); i++ {
				if child.NamedChild(i).Kind() == "forward_definition" {
					isForwardDef = true
					break
				}
			}
			s.references = append(s.references, &referenceStatement{
				name:         newIdentifier(s, child.ChildByFieldName("name")),
				isForwardDef: isForwardDef,
			})
		}
	}
}
