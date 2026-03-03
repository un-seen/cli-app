package spec

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// FetchSpec retrieves an OpenAPI spec from a source string.
// If the source starts with http:// or https://, it is fetched as a URL.
// Otherwise it is read as a local file path.
func FetchSpec(source, cacheDir string) ([]byte, error) {
	if source == "" {
		return nil, fmt.Errorf("no spec source defined")
	}
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return fetchFromURL(source, cacheDir)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("failed to read spec file %s: %w", source, err)
	}
	return data, nil
}

func fetchFromURL(url, cacheDir string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch spec from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("spec URL %s returned HTTP %d", url, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response from %s: %w", url, err)
	}

	if cacheDir != "" {
		cacheSpec(url, data, cacheDir)
	}

	return data, nil
}

func cacheSpec(url string, data []byte, cacheDir string) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return
	}
	hash := sha256.Sum256([]byte(url))
	name := fmt.Sprintf("%x", hash[:8])
	_ = os.WriteFile(filepath.Join(cacheDir, name), data, 0644)
}
