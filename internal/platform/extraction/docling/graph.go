package docling

import (
	"fmt"
	"rag/internal/extract/step"
)

func wireGraph(flatNodes map[string]*step.Node, rawDoc *rawDoclingDocument) (step.ExtractedDoc, error) {
	extractedDoc := step.ExtractedDoc{}
	roots := make([]*step.Node, 0, 0)
	rootRefs := make([]rawRef, 0, len(rawDoc.Body.Children)+len(rawDoc.Furniture.Children))
	rootRefs = append(rootRefs, rawDoc.Body.Children...)
	rootRefs = append(rootRefs, rawDoc.Furniture.Children...)
	for _, ref := range rootRefs {
		node, ok := flatNodes[ref.Ref]
		if !ok {
			return extractedDoc, fmt.Errorf("dangling root ref: %s", ref.Ref)
		}
		roots = append(roots, node)
	}
	extractedDoc.RootNodes = roots

	for _, group := range rawDoc.Groups {
		node, ok := flatNodes[group.SelfRef]
		if !ok {
			return extractedDoc, fmt.Errorf("no node found for id %s", group.SelfRef)
		}
		err := wireNode(node, group.Parent, group.Children, flatNodes)
		if err != nil {
			return extractedDoc, err
		}
	}

	for _, table := range rawDoc.Tables {
		node, ok := flatNodes[table.SelfRef]
		if !ok {
			return extractedDoc, fmt.Errorf("no node found for id %s", table.SelfRef)
		}
		err := wireNode(node, table.Parent, table.Children, flatNodes)
		if err != nil {
			return extractedDoc, err
		}
	}

	for _, pic := range rawDoc.Pictures {
		node, ok := flatNodes[pic.SelfRef]
		if !ok {
			return extractedDoc, fmt.Errorf("no node found for id %s", pic.SelfRef)
		}
		err := wireNode(node, pic.Parent, pic.Children, flatNodes)
		if err != nil {
			return extractedDoc, err
		}
	}

	for _, text := range rawDoc.Texts {
		node, ok := flatNodes[text.SelfRef]
		if !ok {
			return extractedDoc, fmt.Errorf("no node found for id %s", text.SelfRef)
		}
		err := wireNode(node, text.Parent, text.Children, flatNodes)
		if err != nil {
			return extractedDoc, err
		}
	}

	return extractedDoc, nil
}

func wireNode(node *step.Node, parent *rawRef, children []rawRef, flatNodes map[string]*step.Node) error {
	if parent != nil {
		switch parent.Ref {
		case "":
			return fmt.Errorf("empty value for parent ref, malformed input")
		case "#/body", "#/furniture":
			node.Parent = nil
		default:
			parentNode, ok := flatNodes[parent.Ref]
			if !ok {
				return fmt.Errorf("cannot resolve ref to parent, ref: '%s'", parent.Ref)
			}
			node.Parent = parentNode
		}
	}

	nodeChildren := make([]*step.Node, 0, len(children))
	for _, child := range children {
		childNode, ok := flatNodes[child.Ref]
		if !ok {
			return fmt.Errorf("dangling child ref: %s", child.Ref)
		}
		nodeChildren = append(nodeChildren, childNode)
	}
	node.Children = nodeChildren

	return nil
}
