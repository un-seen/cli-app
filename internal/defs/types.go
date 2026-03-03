package defs

// SpecGroup represents all operations derived from a single OpenAPI spec.
type SpecGroup struct {
	Name           string
	AuthMode       string // "header" or "query"
	AuthHeader     string
	AuthPrefix     string
	AuthQueryParam string
	Operations     []Operation
}

// Operation represents a single API endpoint mapped to a CLI command.
type Operation struct {
	Tag         string
	OperationID string // kebab-cased
	Summary     string
	Description string
	Method      string // GET, POST, PUT, DELETE, PATCH
	Path        string // URL path template, e.g. /users/{id}
	BaseURL     string
	NoAuth      bool // true if operation has security: []
	HasBody     bool
	Parameters  []Parameter
	BodyFields  []BodyField
}

// Parameter represents a CLI flag derived from an OpenAPI parameter.
type Parameter struct {
	Name        string
	In          string // "path", "query", "header"
	Type        string // "string", "int", "bool", "float"
	Required    bool
	Default     string // string representation of default value
	Description string
	Enum        []string
}

// BodyField represents a flag derived from a requestBody object property.
type BodyField struct {
	Name        string
	Type        string // "string", "int", "bool", "float"
	Required    bool
	Description string
}
