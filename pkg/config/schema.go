package config

import _ "embed"

//go:embed schema/route.v1.schema.json
var routeSchemaV1 []byte

// SchemaID is the canonical $id of an embedded JSON Schema.
type SchemaID string

const SchemaIDRouteV1 SchemaID = "https://eventrouter.dev/schemas/route.v1.schema.json"

// SchemaCatalog serves the embedded route document schemas by API version.
type SchemaCatalog struct{}

// RouteV1 returns a copy of the router/v1 JSON Schema bytes.
func (SchemaCatalog) RouteV1() []byte {
	out := make([]byte, len(routeSchemaV1))
	copy(out, routeSchemaV1)
	return out
}
