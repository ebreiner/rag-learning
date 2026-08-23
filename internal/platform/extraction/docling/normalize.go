package docling

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"rag/internal/extract/step"
	"rag/internal/platform/telemetry/logging"
	"strconv"
	"strings"
)

type pageHeightLookup map[int64]float64

func buildNodes(ctx context.Context, rawDoc *rawDoclingDocument, logger *slog.Logger) (map[string]*step.Node, error) {
	nodes := make(map[string]*step.Node)
	heightLookup, err := pageHeights(rawDoc.Pages)
	if err != nil {
		return nodes, err
	}

	for _, text := range rawDoc.Texts {
		node := &step.Node{}
		if err := textNode(ctx, node, text, logger); err != nil {
			return nodes, err
		} else {
			nodes[node.ID] = node
		}
	}

	for _, table := range rawDoc.Tables {
		node := &step.Node{}
		err := tableNode(ctx, node, table, heightLookup, logger)
		if err != nil {
			return nodes, err
		}
		nodes[node.ID] = node

	}

	for _, picture := range rawDoc.Pictures {
		node := &step.Node{}
		if err := pictureNode(ctx, node, picture, logger); err != nil {
			return nodes, err
		}
		nodes[node.ID] = node
	}

	for _, group := range rawDoc.Groups {
		node := &step.Node{}
		if err := groupNode(ctx, node, group, logger); err != nil {
			return nodes, err
		}
		nodes[node.ID] = node
	}

	return nodes, nil
}

func textNode(ctx context.Context, node *step.Node, text rawTextItem, logger *slog.Logger) error {
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
			logger.WarnContext(ctx, "build-nodes", logging.KeyNodeType, text.Label, "warn", "empty text")
		}
		headingContent := &step.HeadingContent{Level: *text.Level, Text: text.Text}
		node.Heading = headingContent
		node.Kind = step.KindHeading
	case "list_item":
		if text.Text == "" {
			logger.WarnContext(ctx, "build-nodes", logging.KeyNodeType, text.Label, "warn", "empty text")
		}
		item := &step.ListItemContent{Text: text.Text}
		if text.Marker == nil || *text.Marker == "" {
			logger.WarnContext(ctx, "build-nodes", logging.KeyNodeType, text.Label, "warn", "nil or empty list marker, defaulting to '-'")
			item.Marker = "-"
		} else {
			item.Marker = *text.Marker
		}
		if text.Enumerated == nil {
			logger.WarnContext(ctx, "build-nodes", logging.KeyNodeType, text.Label, "warn", "empty enumerated text")
			item.Enumerated = false
		} else {
			item.Enumerated = *text.Enumerated
		}
		node.ListItem = item
		node.Kind = step.KindListItem
	case "caption", "footnote", "form", "key_value_region", "page_header", "page_footer", "code", "formula", "checkbox_selected", "checkbox_unselected", "chart", "document_index", "grading_scale", "handwritten_text", "empty_value", "reference", "field_region", "field_heading", "field_item", "field_key", "field_value", "field_hint", "marker", "paragraph":
		logger.WarnContext(ctx, "build-nodes", logging.KeyNodeType, text.Label, "warn", fmt.Sprintf("unknown content type label '%s'", text.Label))
		node.Kind = step.KindUnsupported
	default:
		logger.WarnContext(ctx, "build-nodes", logging.KeyNodeType, text.Label, "warn", fmt.Sprintf("unknown content type label '%s'", text.Label))
		node.Kind = step.KindUnsupported
	}

	node.ID = text.SelfRef
	node.Provenance = provs
	return nil
}

func groupNode(ctx context.Context, node *step.Node, group rawGroupItem, logger *slog.Logger) error {
	if group.SelfRef == "" {
		return fmt.Errorf("missing self ref field as node id")
	}

	if err := setContentLayer(node, group.ContentLayer); err != nil {
		return err
	}

	switch group.Label {
	case "list":
		node.Kind = step.KindList
	case "inline":
		node.Kind = step.KindGroup
	case "section", "key_value_area", "form_area", "unspecified", "ordered_list", "chapter", "sheet", "slide", "comment_section", "picture_area":
		logger.WarnContext(ctx, "build-nodes", logging.KeyNodeType, step.KindGroup, "warn", fmt.Sprintf("unsupported group type label '%s'", group.Label))
		node.Kind = step.KindUnsupported
	default:
		logger.WarnContext(ctx, "build-nodes", logging.KeyNodeType, step.KindUnsupported, "warn", fmt.Sprintf("unsupported group type label '%s'", group.Label))
		node.Kind = step.KindUnsupported
	}
	node.ID = group.SelfRef
	return nil
}

func pictureNode(ctx context.Context, node *step.Node, rawPic rawPictureItem, logger *slog.Logger) error {
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
		logger.WarnContext(ctx, "build-nodes", logging.KeyNodeType, step.KindPicture, "warn", "empty picture data")
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

func tableNode(ctx context.Context, node *step.Node, table rawTableItem, heightLookup pageHeightLookup, logger *slog.Logger) error {
	if table.SelfRef == "" {
		return fmt.Errorf("missing self ref for table node")
	}

	// i'm only here to catch, log and filter unknown table node labels
	switch table.Label {
	case "table":
	default:
		logger.WarnContext(ctx, "build-nodes", logging.KeyNodeType, table.Label, "warn", fmt.Sprintf("unsupported table node type '%s' skipping", table.Label))
		node.ID = table.SelfRef
		node.Kind = step.KindUnsupported

		return nil
	}

	// switch should capture all unknown or unsupported labels, if this errors something broke probably
	if table.Label != "table" {
		return fmt.Errorf("wrong label '%s' for table node", table.Label)
	}

	if err := setContentLayer(node, table.ContentLayer); err != nil {
		return err
	}

	tableData := &step.TableContent{}
	cells := make([]step.TableCell, 0, len(table.Data.Cells))
	for _, rawCell := range table.Data.Cells {
		cell, err := tableCell(ctx, rawCell, logger)
		if err != nil {
			return err
		}
		cells = append(cells, cell)
	}
	tableData.Cells = cells

	provs := make([]step.Provenance, 0, len(table.Prov))
	if len(table.Prov) == 0 {
		logger.WarnContext(ctx, "build-nodes", logging.KeyNodeType, node.Kind, "warn", "empty provenance data in table")
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

func tableCell(ctx context.Context, rawCell rawTableCell, logger *slog.Logger) (step.TableCell, error) {
	cell := step.TableCell{}
	if rawCell.Text == "" {
		logger.WarnContext(ctx, "build-nodes", logging.KeyNodeType, step.KindTable, "warn", "table cell has empty text")
	}
	if rawCell.RowSpan != rawCell.EndRowOffsetIdx-rawCell.StartRowOffsetIdx {
		return cell, fmt.Errorf("row span and offsets disagree, probably something weird with the source data")
	}
	if rawCell.ColSpan != rawCell.EndColOffsetIdx-rawCell.StartColOffsetIdx {
		return cell, fmt.Errorf("col span and offsets disagree, probably something weird with the source data")
	}
	switch rawCell.BBox.CoordOrigin {
	case "":
		logger.WarnContext(ctx, "build-nodes", logging.KeyNodeType, step.KindTable, "warn", "table cell misses bbox data")
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
	case "invisible":
		node.Layer = step.LayerInvisible
	case "notes":
		node.Layer = step.LayerNotes
	case "background":
		node.Layer = step.LayerBackground
	default:
		return fmt.Errorf("unknown content layer '%s'", contentLayer)
	}

	return nil
}
