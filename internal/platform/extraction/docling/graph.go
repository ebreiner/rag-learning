package docling

import (
	"fmt"
	"rag/internal/extract/step"
)

func wireGraph(flatNodes map[string]*step.Node, rawDoc *rawDoclingDocument) (step.ExtractedDoc, error) {
	extractedDoc := step.ExtractedDoc{}

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

	claimed := map[string]bool{}
	attach := func(host *step.Node, refs []rawRef) error {
		for _, ref := range refs {
			n, ok := flatNodes[ref.Ref]
			if !ok {
				return fmt.Errorf("dangling caption/footnote ref: %s", ref.Ref)
			}
			claimed[ref.Ref] = true
			if n.Parent == host {
				continue // already wired as a child
			}
			n.Parent = host
			host.Children = append(host.Children, n)
		}
		return nil
	}
	for _, table := range rawDoc.Tables {
		node, ok := flatNodes[table.SelfRef]
		if !ok {
			return extractedDoc, fmt.Errorf("no node found in node map with node id: %s", table.SelfRef)
		}
		if err := attach(node, table.Captions); err != nil {
			return extractedDoc, err
		}
		if err := attach(node, table.Footnotes); err != nil {
			return extractedDoc, err
		}
	}

	for _, picture := range rawDoc.Pictures {
		node, ok := flatNodes[picture.SelfRef]
		if !ok {
			return extractedDoc, fmt.Errorf("no node found in node map with node id: %s", picture.SelfRef)
		}
		if err := attach(node, picture.Captions); err != nil {
			return extractedDoc, err
		}
		if err := attach(node, picture.Footnotes); err != nil {
			return extractedDoc, err
		}
	}

	roots := make([]*step.Node, 0)
	rootRefs := make([]rawRef, 0, len(rawDoc.Body.Children)+len(rawDoc.Furniture.Children))
	rootRefs = append(rootRefs, rawDoc.Body.Children...)
	rootRefs = append(rootRefs, rawDoc.Furniture.Children...)
	for _, ref := range rootRefs {
		if _, ok := claimed[ref.Ref]; ok {
			continue // caption / footnote are linked via ref and not children[] in host
		}
		node, ok := flatNodes[ref.Ref]
		if !ok {
			return extractedDoc, fmt.Errorf("dangling root ref: %s", ref.Ref)
		}
		roots = append(roots, node)
	}
	extractedDoc.RootNodes = roots

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
