package docling

import (
	"encoding/base64"
	"fmt"
	"log"
	"rag/internal/extract/step"
	"strconv"
	"strings"
)

type pageHeightLookup map[int64]float64

func buildNodes(rawDoc *rawDoclingDocument) (map[string]*step.Node, error) {
	nodes := make(map[string]*step.Node)
	heightLookup, err := pageHeights(rawDoc.Pages)
	if err != nil {
		return nodes, err
	}

	for _, text := range rawDoc.Texts {
		node := &step.Node{}
		if err := textNode(node, text); err != nil {
			return nodes, err
		} else {
			nodes[node.ID] = node
		}
	}

	for _, table := range rawDoc.Tables {
		node := &step.Node{}
		err := tableNode(node, table, heightLookup)
		if err != nil {
			return nodes, err
		}
		nodes[node.ID] = node

	}

	for _, picture := range rawDoc.Pictures {
		node := &step.Node{}
		if err := pictureNode(node, picture); err != nil {
			return nodes, err
		}
		nodes[node.ID] = node
	}

	for _, group := range rawDoc.Groups {
		node := &step.Node{}
		if err := groupNode(node, group); err != nil {
			return nodes, err
		}
		nodes[node.ID] = node
	}

	return nodes, nil
}

func textNode(node *step.Node, text rawTextItem) error {
	if text.SelfRef == "" {
		return fmt.Errorf("empty self reference in text node construction")
	}
	if err := setContentLayer(node, text.ContentLayer); err != nil {
		return err
	}

	rawProvs := text.Prov
	provs := make([]step.Provenance, 0)
	// TODO: test for nodes that span multiple pages / Multi prov test
	//TODO: can prov enrichment in nodes be generalized nicely? -> see later
	for _, rawProv := range rawProvs {
		prov := step.Provenance{Page: rawProv.PageNo}
		provs = append(provs, prov)
	}

	switch text.Label {
	case "text":
		node.Paragraph = &step.ParagraphContent{Text: text.Text}
		node.Kind = step.KindParagraph
	case "section_header", "title":
		if text.Label == "title" {
			level := int64(1)
			text.Level = &level
		}
		if text.Level == nil {
			return fmt.Errorf("value for level is nil")
		}
		if *text.Level < 1 || *text.Level > 5 {
			return fmt.Errorf("level %d out of expected range [1,5]", *text.Level)
		}
		if text.Text == "" {
			// TODO: how should this be handled? whats the required stuff for the domain model, whats optional?
			log.Print("warning: empty string as paragraph value found")
		}
		headingContent := &step.HeadingContent{Level: *text.Level, Text: text.Text}
		node.Heading = headingContent
		node.Kind = step.KindHeading
	case "list_item":
		if text.Text == "" {
			log.Print("warning: empty list_item text")
		}
		item := &step.ListItemContent{Text: text.Text}
		if text.Marker == nil || *text.Marker == "" {
			log.Print("warning: nil or empty list_item marker")
			item.Marker = "-"
		} else {
			item.Marker = *text.Marker
		}
		if text.Enumerated == nil {
			log.Print("warning: nil list_item enumeration")
			item.Enumerated = false
		} else {
			item.Enumerated = *text.Enumerated
		}
		node.ListItem = item
		node.Kind = step.KindListItem
	case "caption", "footnote", "page_header", "page_footer", "code":
		log.Printf("warning: no support for node of label '%s'", text.Label)
	default:
		log.Printf("debug: error case, full value dump: \n\n%+v\n\n", text)
		return fmt.Errorf("unknown label type '%s'", text.Label)
	}

	node.ID = text.SelfRef
	node.Provenance = provs
	return nil
}

func groupNode(node *step.Node, group rawGroupItem) error {
	if group.SelfRef == "" {
		return fmt.Errorf("missing self ref field as node id")
	}

	if err := setContentLayer(node, group.ContentLayer); err != nil {
		return err
	}

	switch group.Label {
	case "list":
		node.Kind = step.KindList
	//	case "section":
	//		node.Kind = step.KindHeading
	//		node.Heading.Level = sec
	case "section":
		log.Printf("warning: no support for node of label '%s'", group.Label)
	case "inline":
		node.Kind = step.KindGroup
	default:
		log.Printf("debug: error case, full value dump: \n\n%+v\n\n", group)
		return fmt.Errorf("unknown label type '%s'", group.Label)
	}
	node.ID = group.SelfRef
	return nil
}

func pictureNode(node *step.Node, rawPic rawPictureItem) error {
	if rawPic.SelfRef == "" {
		return fmt.Errorf("missing self ref field as node id")
	}
	if rawPic.Label != "picture" {
		return fmt.Errorf("unknown label type '%s'", rawPic.Label)
	}

	if err := setContentLayer(node, rawPic.ContentLayer); err != nil {
		return err
	}

	node.ID = rawPic.SelfRef
	node.Kind = step.KindPicture
	node.Picture = &step.PictureContent{}

	provs := make([]step.Provenance, 0)
	for _, p := range rawPic.Prov {
		provs = append(provs, step.Provenance{Page: p.PageNo})
	}
	node.Provenance = provs

	for _, raw := range rawPic.Annotations {
		a, ok := raw.(string)
		if !ok {
			return fmt.Errorf("unexpected annotation shape %T", raw)
		}
		node.Picture.Annotations = append(node.Picture.Annotations, a)
	}

	if rawPic.Image == nil {
		log.Print("warning: missing picture data")
		return nil
	}

	// rawPic.Image.URI may be a bare base64 payload or prefixed with "data:...;base64,"
	payload := rawPic.Image.URI
	if idx := strings.LastIndex(payload, ","); idx != -1 {
		payload = payload[idx+1:]
	}

	imageData := make([]byte, base64.StdEncoding.DecodedLen(len(payload)))
	n, err := base64.StdEncoding.Decode(imageData, []byte(payload))
	if err != nil {
		return fmt.Errorf("cannot base64 decode image data: %w", err)
	}

	node.Picture.Image = &step.PictureImage{
		MimeType: rawPic.Image.MimeType,
		Data:     imageData[:n],
		Width:    rawPic.Image.Size.Width,
		Height:   rawPic.Image.Size.Height,
	}

	return nil
}

func tableNode(node *step.Node, table rawTableItem, heightLookup pageHeightLookup) error {
	if table.SelfRef == "" {
		return fmt.Errorf("missing self ref for table node")
	}

	if table.Label != "table" { // TODO: thesis, proof is still missing
		return fmt.Errorf("wrong label '%s' for table node", table.Label)
	}

	if err := setContentLayer(node, table.ContentLayer); err != nil {
		return err
	}

	tableData := &step.TableContent{}
	cells := make([]step.TableCell, 0, len(table.Data.Cells))
	for _, rawCell := range table.Data.Cells {
		cell, err := tableCell(rawCell)
		if err != nil {
			return err
		}
		cells = append(cells, cell)
	}
	tableData.Cells = cells

	provs := make([]step.Provenance, 0, len(table.Prov))
	if len(table.Prov) == 0 {
		log.Print("warning: table node has no provenerance data")
	}
	for _, rawProv := range table.Prov {
		prov := step.Provenance{Page: rawProv.PageNo}
		if len(rawProv.CharSpan) == 2 {
			if rawProv.CharSpan[0] == 0 && rawProv.CharSpan[1] == 0 {
				prov.CharSpan = nil
			} else {
				charSpan := &step.CharSpan{
					Start: rawProv.CharSpan[0],
					End:   rawProv.CharSpan[1],
				}
				prov.CharSpan = charSpan
			}
		}

		if rawProv.BBox.CoordOrigin == "" {
			return fmt.Errorf("bbox missing required orientation info for building table node")
		} else {
			height, ok := heightLookup[rawProv.PageNo]
			if !ok {
				return fmt.Errorf("no height found for page '%d' at table node construction", rawProv.PageNo)
			}
			normalizedBbox, err := bboxNormalize(rawProv.BBox, height)
			if err != nil {
				return err
			}
			prov.BBox = &normalizedBbox
		}
		provs = append(provs, prov)
	}
	node.Provenance = provs
	node.ID = table.SelfRef
	node.Kind = step.KindTable
	tableData.Rows = table.Data.NumRows
	tableData.Cols = table.Data.NumCols
	node.Table = tableData

	return nil
}

func tableCell(rawCell rawTableCell) (step.TableCell, error) {
	cell := step.TableCell{}
	if rawCell.Text == "" {
		log.Print("warning: empty text on table cell")
	}
	if rawCell.RowSpan != rawCell.EndRowOffsetIdx-rawCell.StartRowOffsetIdx {
		return cell, fmt.Errorf("row span and offsets disagree, probably something weird with the source data")
	}
	if rawCell.ColSpan != rawCell.EndColOffsetIdx-rawCell.StartColOffsetIdx {
		return cell, fmt.Errorf("col span and offsets disagree, probably something weird with the source data")
	}
	switch rawCell.BBox.CoordOrigin {
	case "":
		log.Print("warning: missing cell bbox")
	case "TOPLEFT":
		cell.BBox = &step.BBox{Left: rawCell.BBox.Left, Top: rawCell.BBox.Top, Right: rawCell.BBox.Right, Bottom: rawCell.BBox.Bottom}
	default:
		return step.TableCell{}, fmt.Errorf("unexpected cell bbox origin %q, want TOPLEFT", rawCell.BBox.CoordOrigin)
	}

	cell.ColStart = rawCell.StartColOffsetIdx
	cell.ColEnd = rawCell.EndColOffsetIdx
	cell.RowStart = rawCell.StartRowOffsetIdx
	cell.RowEnd = rawCell.EndRowOffsetIdx
	cell.IsColumnHeader = rawCell.ColumnHeader
	cell.IsRowHeader = rawCell.RowHeader
	cell.Fillable = rawCell.Fillable
	cell.RowSection = rawCell.RowSection
	cell.Text = rawCell.Text

	return cell, nil
}

func bboxNormalize(rawB rawBBox, height float64) (step.BBox, error) {
	normalizedB := step.BBox{}
	switch rawB.CoordOrigin {
	case "BOTTOMLEFT": // flip to top left orientation
		normalizedB.Left = rawB.Left
		normalizedB.Right = rawB.Right
		normalizedB.Top = height - rawB.Top
		normalizedB.Bottom = height - rawB.Bottom
	case "TOPLEFT":
		normalizedB.Left = rawB.Left
		normalizedB.Right = rawB.Right
		normalizedB.Top = rawB.Top
		normalizedB.Bottom = rawB.Bottom
	default:
		return normalizedB, fmt.Errorf("unknown bbox orientation for construction table node")

	}

	return normalizedB, nil
}

func pageHeights(pages map[string]rawPage) (pageHeightLookup, error) {
	heights := make(pageHeightLookup, len(pages))
	for key, page := range pages {
		pageNo, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid page key %q: %w", key, err)
		}
		heights[pageNo] = page.Size.Height
	}
	return heights, nil
}

func setContentLayer(node *step.Node, contentLayer string) error {
	switch contentLayer {
	case "body":
		node.Layer = step.LayerBody
	case "furniture":
		node.Layer = step.LayerFurniture
	default:
		return fmt.Errorf("unknown content layer '%s'", contentLayer)
	}

	return nil
}
