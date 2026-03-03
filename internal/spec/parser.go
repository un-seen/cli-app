package spec

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hedwigai/cli/internal/defs"
)

// ParseSpec parses raw OpenAPI spec bytes into a SpecGroup.
// Auth mode, headers, and base URL are all inferred from the spec itself.
func ParseSpec(data []byte, groupName string) (*defs.SpecGroup, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse spec %q: %w", groupName, err)
	}

	// Validate but treat as warning — real-world specs often have minor issues.
	if err := doc.Validate(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: spec %q has validation issues: %v\n", groupName, err)
	}

	authMode, authHeader, authPrefix, authQueryParam := resolveAuth(doc)

	baseURL := ""
	if len(doc.Servers) > 0 {
		baseURL = doc.Servers[0].URL
	}

	operations, err := parseOperations(doc, baseURL)
	if err != nil {
		return nil, err
	}

	return &defs.SpecGroup{
		Name:           groupName,
		AuthMode:       authMode,
		AuthHeader:     authHeader,
		AuthPrefix:     authPrefix,
		AuthQueryParam: authQueryParam,
		Operations:     operations,
	}, nil
}

// resolveAuth infers authentication details from the OpenAPI securitySchemes.
func resolveAuth(doc *openapi3.T) (mode, header, prefix, queryParam string) {
	if doc.Components != nil && doc.Components.SecuritySchemes != nil {
		for _, ref := range doc.Components.SecuritySchemes {
			if ref.Value == nil {
				continue
			}
			if ref.Value.Type == "http" && ref.Value.Scheme == "bearer" {
				return "header", "Authorization", "Bearer ", ""
			}
			if ref.Value.Type == "apiKey" && ref.Value.In == "query" {
				return "query", "", "", ref.Value.Name
			}
			if ref.Value.Type == "apiKey" && ref.Value.In == "header" {
				return "header", ref.Value.Name, "", ""
			}
		}
	}
	// Default: bearer token in Authorization header.
	return "header", "Authorization", "Bearer ", ""
}

func parseOperations(doc *openapi3.T, baseURL string) ([]defs.Operation, error) {
	var operations []defs.Operation

	if doc.Paths == nil {
		return operations, nil
	}

	paths := make([]string, 0, len(doc.Paths.Map()))
	for path := range doc.Paths.Map() {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		pathItem := doc.Paths.Map()[path]

		methods := []struct {
			name string
			op   *openapi3.Operation
		}{
			{"DELETE", pathItem.Delete},
			{"GET", pathItem.Get},
			{"PATCH", pathItem.Patch},
			{"POST", pathItem.Post},
			{"PUT", pathItem.Put},
		}

		for _, m := range methods {
			if m.op == nil {
				continue
			}
			op, err := parseOperation(m.op, m.name, path, baseURL, pathItem.Parameters)
			if err != nil {
				return nil, err
			}
			operations = append(operations, *op)
		}
	}

	return operations, nil
}

func parseOperation(op *openapi3.Operation, method, path, baseURL string, pathParams openapi3.Parameters) (*defs.Operation, error) {
	tag := "default"
	if len(op.Tags) > 0 {
		tag = op.Tags[0]
	}

	operationID := op.OperationID
	if operationID == "" {
		operationID = strings.ToLower(method) + strings.ReplaceAll(strings.ReplaceAll(path, "/", "-"), "{", "")
		operationID = strings.ReplaceAll(operationID, "}", "")
	}
	operationID = toKebabCase(operationID)

	noAuth := false
	if op.Security != nil && len(*op.Security) == 0 {
		noAuth = true
	}

	var params []defs.Parameter
	allParams := make(openapi3.Parameters, 0)
	allParams = append(allParams, pathParams...)
	allParams = append(allParams, op.Parameters...)

	for _, paramRef := range allParams {
		if paramRef.Value == nil {
			continue
		}
		param := paramRef.Value
		p := defs.Parameter{
			Name:        param.Name,
			In:          param.In,
			Required:    param.Required,
			Description: param.Description,
			Type:        "string",
		}

		if param.Schema != nil && param.Schema.Value != nil {
			schema := param.Schema.Value
			p.Type = mapSchemaType(schema.Type)
			if schema.Default != nil {
				p.Default = fmt.Sprintf("%v", schema.Default)
			}
			p.Enum = enumToStrings(schema.Enum)
		}

		params = append(params, p)
	}

	hasBody := false
	var bodyFields []defs.BodyField

	if op.RequestBody != nil && op.RequestBody.Value != nil {
		hasBody = true
		body := op.RequestBody.Value

		if content, ok := body.Content["application/json"]; ok {
			if content.Schema != nil && content.Schema.Value != nil {
				schema := content.Schema.Value
				if schema.Type != nil && schema.Type.Is("object") && len(schema.Properties) <= 8 {
					for name, propRef := range schema.Properties {
						if propRef.Value == nil {
							continue
						}
						required := false
						for _, r := range schema.Required {
							if r == name {
								required = true
								break
							}
						}
						bodyFields = append(bodyFields, defs.BodyField{
							Name:        name,
							Type:        mapSchemaType(propRef.Value.Type),
							Required:    required,
							Description: propRef.Value.Description,
						})
					}
					sort.Slice(bodyFields, func(i, j int) bool {
						return bodyFields[i].Name < bodyFields[j].Name
					})
				}
			}
		}
	}

	return &defs.Operation{
		Tag:         tag,
		OperationID: operationID,
		Summary:     op.Summary,
		Description: op.Description,
		Method:      method,
		Path:        path,
		BaseURL:     baseURL,
		NoAuth:      noAuth,
		HasBody:     hasBody,
		Parameters:  params,
		BodyFields:  bodyFields,
	}, nil
}

func toKebabCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				prev := rune(s[i-1])
				if prev != '-' && prev != '_' && prev != ' ' {
					result.WriteByte('-')
				}
			}
			result.WriteRune(r + 32)
		} else if r == '_' || r == ' ' {
			result.WriteByte('-')
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func mapSchemaType(t *openapi3.Types) string {
	if t == nil {
		return "string"
	}
	if t.Is("integer") {
		return "int"
	}
	if t.Is("number") {
		return "float"
	}
	if t.Is("boolean") {
		return "bool"
	}
	return "string"
}

func enumToStrings(enums []interface{}) []string {
	if len(enums) == 0 {
		return nil
	}
	result := make([]string, len(enums))
	for i, e := range enums {
		result[i] = fmt.Sprintf("%v", e)
	}
	return result
}
