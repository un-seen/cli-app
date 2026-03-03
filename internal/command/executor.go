package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hedwigai/cli/generated"
	"github.com/hedwigai/cli/internal/defs"
)

func executeOperation(cmd *cobra.Command, op *defs.Operation, group *defs.SpecGroup) error {
	// Resolve auth token.
	token := ""
	if !op.NoAuth {
		token = os.Getenv(generated.AuthEnvVar)
		if token == "" {
			fmt.Fprintf(os.Stderr, "Error: %s environment variable is not set.\nExport it with: export %s=\"your-token-here\"\n",
				generated.AuthEnvVar, generated.AuthEnvVar)
			os.Exit(1)
		}
	}

	// Validate enum values before making the request.
	for _, param := range op.Parameters {
		if len(param.Enum) == 0 {
			continue
		}
		val := getFlagValue(cmd, &param)
		if val == "" {
			continue
		}
		valid := false
		for _, e := range param.Enum {
			if val == e {
				valid = true
				break
			}
		}
		if !valid {
			fmt.Fprintf(os.Stderr, "Error: --%s must be one of: %s\n", param.Name, strings.Join(param.Enum, ", "))
			os.Exit(1)
		}
	}

	// Build URL.
	reqBaseURL := op.BaseURL
	if baseURL != "" {
		reqBaseURL = baseURL
	}

	path := op.Path
	for _, param := range op.Parameters {
		if param.In == "path" {
			val := getFlagValue(cmd, &param)
			path = strings.ReplaceAll(path, "{"+param.Name+"}", val)
		}
	}

	// Query parameters: include only explicitly-set flags.
	var queryParts []string
	for _, param := range op.Parameters {
		if param.In != "query" {
			continue
		}
		flagName := param.Name
		if !cmd.Flags().Changed(flagName) {
			continue
		}
		val := getFlagValue(cmd, &param)
		if val != "" {
			queryParts = append(queryParts, param.Name+"="+val)
		}
	}

	fullURL := strings.TrimRight(reqBaseURL, "/") + path
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

	// Build request body.
	var bodyReader io.Reader
	contentType := ""

	if op.HasBody {
		bodyStr, _ := cmd.Flags().GetString("body")
		bodyFile, _ := cmd.Flags().GetString("body-file")

		switch {
		case bodyStr != "":
			if !json.Valid([]byte(bodyStr)) {
				fmt.Fprintln(os.Stderr, "Error: --body is not valid JSON")
				os.Exit(1)
			}
			bodyReader = strings.NewReader(bodyStr)
			contentType = "application/json"

		case bodyFile != "":
			data, err := os.ReadFile(bodyFile)
			if err != nil {
				return fmt.Errorf("failed to read body file: %w", err)
			}
			bodyReader = bytes.NewReader(data)
			contentType = "application/json"

		default:
			if body := buildBodyFromFields(cmd, op.BodyFields); body != nil {
				bodyReader = bytes.NewReader(body)
				contentType = "application/json"
			}
		}
	}

	// Create HTTP request.
	req, err := http.NewRequest(op.Method, fullURL, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// Auth injection: header mode.
	if !op.NoAuth && group.AuthMode == "header" {
		req.Header.Set(group.AuthHeader, group.AuthPrefix+token)
	}

	// Header parameters.
	for _, param := range op.Parameters {
		if param.In != "header" {
			continue
		}
		val, _ := cmd.Flags().GetString("header-" + param.Name)
		if val != "" {
			req.Header.Set(param.Name, val)
		}
	}

	// Verbose output.
	if verbose {
		fmt.Fprintf(os.Stderr, "%s %s\n", req.Method, req.URL.String())
		for key, vals := range req.Header {
			for _, val := range vals {
				if key == group.AuthHeader && !op.NoAuth {
					masked := group.AuthPrefix
					if len(masked) > 7 {
						masked = masked[:7]
					}
					fmt.Fprintf(os.Stderr, "%s: %s***\n", key, masked)
				} else {
					fmt.Fprintf(os.Stderr, "%s: %s\n", key, val)
				}
			}
		}
		fmt.Fprintln(os.Stderr)
	}

	// Execute request.
	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "deadline exceeded") || strings.Contains(err.Error(), "timeout") {
			fmt.Fprintf(os.Stderr, "Error: request timed out after %ds\n", timeout)
			os.Exit(3)
		}
		fmt.Fprintf(os.Stderr, "Error: could not reach %s\n", req.URL.Host)
		os.Exit(2)
		return nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	return handleResponse(resp.StatusCode, respBody)
}

func buildBodyFromFields(cmd *cobra.Command, fields []defs.BodyField) []byte {
	bodyMap := make(map[string]interface{})
	hasFields := false

	for _, field := range fields {
		if !cmd.Flags().Changed(field.Name) {
			continue
		}
		switch field.Type {
		case "int":
			if val, err := cmd.Flags().GetInt(field.Name); err == nil {
				bodyMap[field.Name] = val
				hasFields = true
			}
		case "bool":
			if val, err := cmd.Flags().GetBool(field.Name); err == nil {
				bodyMap[field.Name] = val
				hasFields = true
			}
		case "float":
			if val, err := cmd.Flags().GetFloat64(field.Name); err == nil {
				bodyMap[field.Name] = val
				hasFields = true
			}
		default:
			if val, err := cmd.Flags().GetString(field.Name); err == nil && val != "" {
				bodyMap[field.Name] = val
				hasFields = true
			}
		}
	}

	if !hasFields {
		return nil
	}
	data, err := json.Marshal(bodyMap)
	if err != nil {
		return nil
	}
	return data
}

func handleResponse(statusCode int, body []byte) error {
	isSuccess := statusCode >= 200 && statusCode < 300

	var out io.Writer = os.Stdout
	if outputFile != "" && isSuccess {
		f, err := os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer f.Close()
		out = f
	}

	if isSuccess {
		if len(body) == 0 {
			fmt.Fprintf(out, "OK (%d)\n", statusCode)
			return nil
		}
		if rawOutput {
			_, _ = out.Write(body)
			fmt.Fprintln(out)
		} else {
			var prettyJSON bytes.Buffer
			if err := json.Indent(&prettyJSON, body, "", "  "); err != nil {
				_, _ = out.Write(body)
				fmt.Fprintln(out)
			} else {
				fmt.Fprintln(out, prettyJSON.String())
			}
		}
		return nil
	}

	// Error response.
	if json.Valid(body) {
		var prettyJSON bytes.Buffer
		if err := json.Indent(&prettyJSON, body, "", "  "); err == nil {
			fmt.Fprintln(os.Stderr, prettyJSON.String())
		} else {
			fmt.Fprintln(os.Stderr, string(body))
		}
	} else {
		fmt.Fprintln(os.Stderr, string(body))
	}

	os.Exit(1)
	return nil
}

func getFlagValue(cmd *cobra.Command, param *defs.Parameter) string {
	flagName := param.Name
	if param.In == "header" {
		flagName = "header-" + param.Name
	}

	switch param.Type {
	case "int":
		val, err := cmd.Flags().GetInt(flagName)
		if err != nil {
			return ""
		}
		if !cmd.Flags().Changed(flagName) {
			return ""
		}
		return strconv.Itoa(val)
	case "bool":
		val, err := cmd.Flags().GetBool(flagName)
		if err != nil {
			return ""
		}
		if !cmd.Flags().Changed(flagName) {
			return ""
		}
		return strconv.FormatBool(val)
	case "float":
		val, err := cmd.Flags().GetFloat64(flagName)
		if err != nil {
			return ""
		}
		if !cmd.Flags().Changed(flagName) {
			return ""
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	default:
		val, _ := cmd.Flags().GetString(flagName)
		return val
	}
}
