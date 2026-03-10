package mcpbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hedwigai/cli/internal/defs"
)

// contextKey is used for storing auth tokens in context.
type contextKey string

const authTokenKey contextKey = "auth_token"

// ToolInfo holds display info for hai mcp list.
type ToolInfo struct {
	Group       string
	Name        string
	Description string
}

// ListTools returns info about all tools that would be registered.
func ListTools(groups []defs.SpecGroup) []ToolInfo {
	var tools []ToolInfo
	for i := range groups {
		group := &groups[i]
		for j := range group.Operations {
			op := &group.Operations[j]
			tools = append(tools, ToolInfo{
				Group:       group.Name,
				Name:        group.Name + "_" + op.OperationID,
				Description: op.Summary,
			})
		}
	}
	return tools
}

// extractBearerToken is a shared context func that extracts Bearer tokens
// from the Authorization header into context for tool handlers.
func extractBearerToken(ctx context.Context, r *http.Request) context.Context {
	if auth := r.Header.Get("Authorization"); auth != "" {
		token := strings.TrimPrefix(auth, "Bearer ")
		ctx = context.WithValue(ctx, authTokenKey, token)
	}
	return ctx
}

// createMCPServer builds an MCP server with all tools from the given groups.
func createMCPServer(binaryName, version, authEnvVar string, groups []defs.SpecGroup) *server.MCPServer {
	s := server.NewMCPServer(binaryName+"-mcp", version, server.WithToolCapabilities(false))
	for i := range groups {
		group := &groups[i]
		for j := range group.Operations {
			op := &group.Operations[j]
			toolName := group.Name + "_" + op.OperationID
			s.AddTools(server.ServerTool{
				Tool:    buildMCPTool(toolName, op),
				Handler: makeHandler(op, group, authEnvVar),
			})
		}
	}
	return s
}

// ServeHTTP starts an HTTP server serving both transports:
//   - Streamable HTTP at /mcp  (Claude Code, newer clients)
//   - Legacy SSE at /sse + /message  (Claude Desktop, older clients)
//
// Clients authenticate by passing their token in the Authorization header.
func ServeHTTP(binaryName, version string, groups []defs.SpecGroup, authEnvVar string, port int) error {
	// Use PORT env var if set (Railway, Render, etc.), otherwise use flag value.
	if envPort := os.Getenv("PORT"); envPort != "" {
		fmt.Sscanf(envPort, "%d", &port)
	}

	s := createMCPServer(binaryName, version, authEnvVar, groups)

	// Streamable HTTP transport (Claude Code).
	streamableSrv := server.NewStreamableHTTPServer(s,
		server.WithStateLess(true),
		server.WithHTTPContextFunc(extractBearerToken),
	)

	// Legacy SSE transport (Claude Desktop).
	addr := fmt.Sprintf(":%d", port)
	sseSrv := server.NewSSEServer(s,
		server.WithBaseURL(fmt.Sprintf("http://0.0.0.0%s", addr)),
		server.WithSSEContextFunc(extractBearerToken),
	)

	identityBaseURL := getIdentityBaseURL()
	mcpBaseURL := getMCPBaseURL()

	mux := http.NewServeMux()
	// OAuth metadata discovery (unauthenticated — bootstraps the auth flow).
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", oauthMetadataHandler(identityBaseURL))
	// MCP transports wrapped with auth middleware.
	mux.Handle("/mcp", authMiddleware(mcpBaseURL, streamableSrv))
	mux.Handle("/sse", authMiddleware(mcpBaseURL, sseSrv.SSEHandler()))
	mux.Handle("/message", authMiddleware(mcpBaseURL, sseSrv.MessageHandler()))

	fmt.Fprintf(os.Stderr, "MCP server listening on %s\n", addr)
	fmt.Fprintf(os.Stderr, "  Streamable HTTP: POST /mcp\n")
	fmt.Fprintf(os.Stderr, "  Legacy SSE:      GET  /sse + POST /message\n")
	fmt.Fprintf(os.Stderr, "  OAuth metadata:  GET  /.well-known/oauth-authorization-server\n")
	fmt.Fprintf(os.Stderr, "  Identity URL:    %s\n", identityBaseURL)
	return http.ListenAndServe(addr, mux)
}

// ServeMultiHTTP starts an HTTP server with one MCP server per instance, each under its slug prefix.
func ServeMultiHTTP(instances []MCPInstance, version string, port int) error {
	if envPort := os.Getenv("PORT"); envPort != "" {
		fmt.Sscanf(envPort, "%d", &port)
	}

	identityBaseURL := getIdentityBaseURL()
	mcpBaseURL := getMCPBaseURL()
	addr := fmt.Sprintf(":%d", port)

	mux := http.NewServeMux()

	// Shared OAuth metadata (unauthenticated).
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", oauthMetadataHandler(identityBaseURL))

	// Discovery index.
	mux.HandleFunc("GET /", discoveryHandler(instances))

	for _, inst := range instances {
		mcpSrv := createMCPServer(inst.BinaryName, version, inst.AuthEnvVar, inst.Groups)

		toolCount := 0
		for _, g := range inst.Groups {
			toolCount += len(g.Operations)
		}

		// Streamable HTTP transport.
		streamableSrv := server.NewStreamableHTTPServer(mcpSrv,
			server.WithStateLess(true),
			server.WithHTTPContextFunc(extractBearerToken),
		)

		// Legacy SSE transport with slug prefix.
		sseSrv := server.NewSSEServer(mcpSrv,
			server.WithBaseURL(fmt.Sprintf("http://0.0.0.0%s", addr)),
			server.WithStaticBasePath("/"+inst.Slug),
			server.WithSSEContextFunc(extractBearerToken),
		)

		prefix := "/" + inst.Slug
		mux.Handle(prefix+"/mcp", authMiddleware(mcpBaseURL, streamableSrv))
		mux.Handle(prefix+"/sse", authMiddleware(mcpBaseURL, sseSrv.SSEHandler()))
		mux.Handle(prefix+"/message", authMiddleware(mcpBaseURL, sseSrv.MessageHandler()))

		fmt.Fprintf(os.Stderr, "  [%s] %d tools → %s/{mcp,sse,message}\n", inst.Slug, toolCount, prefix)
	}

	fmt.Fprintf(os.Stderr, "Multi-config MCP server listening on %s\n", addr)
	fmt.Fprintf(os.Stderr, "  OAuth metadata: GET /.well-known/oauth-authorization-server\n")
	fmt.Fprintf(os.Stderr, "  Discovery:      GET /\n")
	fmt.Fprintf(os.Stderr, "  Identity URL:   %s\n", identityBaseURL)
	return http.ListenAndServe(addr, mux)
}

// discoveryHandler returns a handler that serves a JSON index of all MCP server instances.
func discoveryHandler(instances []MCPInstance) http.HandlerFunc {
	type endpoint struct {
		MCP     string `json:"mcp"`
		SSE     string `json:"sse"`
		Message string `json:"message"`
	}
	type serverInfo struct {
		Slug       string   `json:"slug"`
		BinaryName string   `json:"binary_name"`
		ToolCount  int      `json:"tool_count"`
		Endpoints  endpoint `json:"endpoints"`
	}
	type discovery struct {
		Servers []serverInfo `json:"servers"`
	}

	var d discovery
	for _, inst := range instances {
		toolCount := 0
		for _, g := range inst.Groups {
			toolCount += len(g.Operations)
		}
		prefix := "/" + inst.Slug
		d.Servers = append(d.Servers, serverInfo{
			Slug:       inst.Slug,
			BinaryName: inst.BinaryName,
			ToolCount:  toolCount,
			Endpoints: endpoint{
				MCP:     prefix + "/mcp",
				SSE:     prefix + "/sse",
				Message: prefix + "/message",
			},
		})
	}

	body, _ := json.Marshal(d)

	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(body)
	}
}

// ServeStdio starts a single stdio MCP server with all tools (prefixed).
func ServeStdio(binaryName, version string, groups []defs.SpecGroup, authEnvVar string) error {
	s := createMCPServer(binaryName, version, authEnvVar, groups)
	return server.ServeStdio(s)
}

// buildMCPTool maps an Operation to an mcp.Tool with input schema.
func buildMCPTool(toolName string, op *defs.Operation) mcp.Tool {
	opts := []mcp.ToolOption{
		mcp.WithDescription(op.Summary),
	}

	for i := range op.Parameters {
		p := &op.Parameters[i]
		var propOpts []mcp.PropertyOption
		if p.Required {
			propOpts = append(propOpts, mcp.Required())
		}
		if p.Description != "" {
			propOpts = append(propOpts, mcp.Description(p.Description))
		}
		if len(p.Enum) > 0 {
			propOpts = append(propOpts, mcp.Enum(p.Enum...))
		}
		switch p.Type {
		case "int", "float":
			opts = append(opts, mcp.WithNumber(p.Name, propOpts...))
		case "bool":
			opts = append(opts, mcp.WithBoolean(p.Name, propOpts...))
		default:
			opts = append(opts, mcp.WithString(p.Name, propOpts...))
		}
	}

	for i := range op.BodyFields {
		f := &op.BodyFields[i]
		var propOpts []mcp.PropertyOption
		if f.Required {
			propOpts = append(propOpts, mcp.Required())
		}
		if f.Description != "" {
			propOpts = append(propOpts, mcp.Description(f.Description))
		}
		switch f.Type {
		case "int", "float":
			opts = append(opts, mcp.WithNumber(f.Name, propOpts...))
		case "bool":
			opts = append(opts, mcp.WithBoolean(f.Name, propOpts...))
		default:
			opts = append(opts, mcp.WithString(f.Name, propOpts...))
		}
	}

	return mcp.NewTool(toolName, opts...)
}

// makeHandler returns a ToolHandlerFunc that executes the HTTP request for an operation.
func makeHandler(op *defs.Operation, group *defs.SpecGroup, authEnvVar string) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()

		// Auth token: prefer context (HTTP Authorization header), fall back to env var (stdio).
		token := ""
		if !op.NoAuth {
			if ctxToken, ok := ctx.Value(authTokenKey).(string); ok && ctxToken != "" {
				token = ctxToken
			} else {
				token = os.Getenv(authEnvVar)
			}
			if token == "" {
				return mcpError("authorization required: pass a Bearer token in the Authorization header"), nil
			}
		}

		// Path param substitution.
		path := op.Path
		for _, p := range op.Parameters {
			if p.In == "path" {
				val := argString(args, p.Name)
				path = strings.ReplaceAll(path, "{"+p.Name+"}", val)
			}
		}

		// Query params.
		var queryParts []string
		for _, p := range op.Parameters {
			if p.In != "query" {
				continue
			}
			val := argString(args, p.Name)
			if val != "" {
				queryParts = append(queryParts, p.Name+"="+val)
			}
		}

		fullURL := strings.TrimRight(op.BaseURL, "/") + path
		if len(queryParts) > 0 {
			fullURL += "?" + strings.Join(queryParts, "&")
		}

		// Auth injection: query mode.
		if !op.NoAuth && group.AuthMode == "query" && group.AuthQueryParam != "" {
			sep := "?"
			if strings.Contains(fullURL, "?") {
				sep = "&"
			}
			fullURL += sep + group.AuthQueryParam + "=" + token
		}

		// Body construction.
		var bodyReader io.Reader
		if op.HasBody {
			bodyMap := make(map[string]any)
			for _, f := range op.BodyFields {
				if v, ok := args[f.Name]; ok {
					switch f.Type {
					case "int":
						bodyMap[f.Name] = toInt(v)
					default:
						bodyMap[f.Name] = v
					}
				}
			}
			if len(bodyMap) > 0 {
				data, err := json.Marshal(bodyMap)
				if err != nil {
					return mcpError(fmt.Sprintf("failed to marshal body: %v", err)), nil
				}
				bodyReader = bytes.NewReader(data)
			}
		}

		// Build HTTP request.
		req, err := http.NewRequestWithContext(ctx, op.Method, fullURL, bodyReader)
		if err != nil {
			return mcpError(fmt.Sprintf("failed to create request: %v", err)), nil
		}

		if bodyReader != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		// Auth injection: header mode.
		if !op.NoAuth && group.AuthMode == "header" {
			req.Header.Set(group.AuthHeader, group.AuthPrefix+token)
		}

		// Header parameters.
		for _, p := range op.Parameters {
			if p.In == "header" {
				if val := argString(args, p.Name); val != "" {
					req.Header.Set(p.Name, val)
				}
			}
		}

		// Execute.
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return mcpError(fmt.Sprintf("request failed: %v", err)), nil
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return mcpError(fmt.Sprintf("failed to read response: %v", err)), nil
		}

		// Return result.
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			text := string(respBody)
			if json.Valid(respBody) {
				var buf bytes.Buffer
				if err := json.Indent(&buf, respBody, "", "  "); err == nil {
					text = buf.String()
				}
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.NewTextContent(text)},
			}, nil
		}

		return mcpError(fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody))), nil
	}
}

func mcpError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.NewTextContent(msg)},
		IsError: true,
	}
}

func argString(args map[string]any, name string) string {
	v, ok := args[name]
	if !ok {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toInt(v any) any {
	if f, ok := v.(float64); ok {
		if f == float64(int64(f)) {
			return int64(f)
		}
	}
	return v
}
