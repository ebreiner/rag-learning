package schema

import _ "embed"

//go:embed sqlc_schema.sql
var SQLC string

//go:embed manual_schema.sql
var Manual string

// go: embed fts_trigger.sql
var FTSTrigger string
