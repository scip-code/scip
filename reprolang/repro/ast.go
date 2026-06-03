package repro

import (
	"fmt"

	sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/scip-code/scip/bindings/go/scip"
)

type relationships struct {
	implementsRelation  *identifier
	referencesRelation  *identifier
	typeDefinesRelation *identifier
	definedByRelation   *identifier
}

type definitionStatement struct {
	docstring string
	name      *identifier
	relations relationships
}

func (r *relationships) identifiers() []*identifier {
	return []*identifier{r.implementsRelation, r.referencesRelation, r.typeDefinesRelation, r.definedByRelation}
}

type referenceStatement struct {
	name         *identifier
	isForwardDef bool
}

type identifier struct {
	value    string
	symbol   string
	isLocal  bool
	position scip.Range
}

func newIdentifier(s *reproSourceFile, n *sitter.Node) *identifier {
	if n == nil {
		return nil
	}
	if n.Kind() != "identifier" {
		panic("expected identifier, obtained " + n.Kind())
	}
	value := s.nodeText(n)
	isLocal := false
	localIdentifier := n.ChildByFieldName("local")
	if localIdentifier != nil {
		isLocal = true
		nameNode := localIdentifier.ChildByFieldName("name")
		if nameNode != nil {
			value = s.nodeText(nameNode)
		}
	}
	globalIdentifier := n.ChildByFieldName("global")
	if globalIdentifier != nil {
		projectName := globalIdentifier.ChildByFieldName("project_name")
		descriptors := globalIdentifier.ChildByFieldName("descriptors")
		if projectName != nil && descriptors != nil {
			value = fmt.Sprintf("global %v %v", s.nodeText(projectName), s.nodeText(descriptors))
		}
	}
	return &identifier{
		value:    value,
		isLocal:  isLocal,
		position: NewRangePositionFromNode(n),
	}
}

type relationshipsStatement struct {
	name      *identifier
	relations relationships
}

func NewRangePositionFromNode(node *sitter.Node) scip.Range {
	return scip.Range{
		Start: scip.Position{
			Line:      int32(node.StartPosition().Row),
			Character: int32(node.StartPosition().Column),
		},
		End: scip.Position{
			Line:      int32(node.EndPosition().Row),
			Character: int32(node.EndPosition().Column),
		},
	}
}

func (i *identifier) resolveSymbol(localScope *reproScope, context *reproContext) error {
	scope := context.globalScope
	if i.isLocal {
		scope = localScope
	}
	symbol, ok := scope.names[i.value]
	if !ok {
		return fmt.Errorf("could not resolve local symbol %q", i.value)
	}
	i.symbol = symbol
	return nil
}
