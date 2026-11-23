package retrieval

import (
	"encoding/json"
	"maps"
	"rag/internal/retrieval/step"
	"slices"
	"strings"
)

func filterClause(filter step.Filter) (string, []any) {
	var sb strings.Builder
	args := make([]any, 0)

	if len(filter.Sources) > 0 {
		sb.WriteString(" AND d.source_name IN (?" + strings.Repeat(", ?", len(filter.Sources)-1) + ")")
		for _, s := range filter.Sources {
			args = append(args, s)
		}
	}

	for _, key := range slices.Sorted(maps.Keys(filter.Metadata)) {
		sb.WriteString(" AND json_extract(d.metadata_json, ?) = ?")
		args = append(args, `$."`+key+`"`, filter.Metadata[key])
	}

	if filter.DocIDs != nil {
		ids, _ := json.Marshal(filter.DocIDs) // []int64 cannot fail
		sb.WriteString(" AND d.id IN (SELECT value FROM json_each(?))")
		args = append(args, string(ids))
	}

	return sb.String(), args
}
