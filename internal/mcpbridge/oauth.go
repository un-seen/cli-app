package mcpbridge

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// oauthMetadataHandler returns a handler for GET /.well-known/oauth-authorization-server.
// It serves the OAuth 2.1 Authorization Server Metadata document pointing to identity-rs.
func oauthMetadataHandler(identityBaseURL string) http.HandlerFunc {
	metadata := map[string]any{
		"issuer":                                identityBaseURL,
		"authorization_endpoint":                identityBaseURL + "/authorize",
		"token_endpoint":                        identityBaseURL + "/oauth/token",
		"registration_endpoint":                 identityBaseURL + "/oauth/register",
		"revocation_endpoint":                   identityBaseURL + "/oauth/revoke",
		"introspection_endpoint":                identityBaseURL + "/oauth/introspect",
		"jwks_uri":                              identityBaseURL + "/.well-known/jwks.json",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"code_challenge_methods_supported":      []string{"S256"},
		"scopes_supported":                      []string{"read", "write", "admin"},
	}

	body, _ := json.Marshal(metadata)

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(body)
	}
}

// authMiddleware wraps an HTTP handler to require a Bearer token.
// If no Authorization header is present, it responds with HTTP 401 and a
// WWW-Authenticate header pointing to the metadata endpoint, triggering the
// MCP OAuth discovery flow.
func authMiddleware(mcpBaseURL string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow CORS preflight through.
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		auth := r.Header.Get("Authorization")
		if auth == "" || len(auth) < 8 {
			w.Header().Set("WWW-Authenticate",
				fmt.Sprintf(`Bearer resource_metadata="%s/.well-known/oauth-authorization-server"`, mcpBaseURL))
			w.Header().Set("Access-Control-Allow-Origin", "*")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Token present — pass through to the MCP handler.
		// Token validity is checked downstream by tool handlers.
		next.ServeHTTP(w, r)
	})
}

// getIdentityBaseURL returns the identity service base URL from env or default.
func getIdentityBaseURL() string {
	if u := os.Getenv("IDENTITY_BASE_URL"); u != "" {
		return u
	}
	return "https://identity.hedwigai.com"
}

// getMCPBaseURL returns the external base URL of this MCP server from env or default.
func getMCPBaseURL() string {
	if u := os.Getenv("MCP_BASE_URL"); u != "" {
		return u
	}
	return "https://mcp.hedwigai.com"
}
