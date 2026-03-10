package mcpbridge

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/hedwigai/cli/internal/config"
	"github.com/hedwigai/cli/internal/defs"
	"github.com/hedwigai/cli/internal/spec"
)

// MCPInstance holds everything needed to serve one MCP server.
type MCPInstance struct {
	Slug       string           // filename without extension: "admin"
	BinaryName string           // from config: "hai-admin"
	AuthEnvVar string           // from config: "HEDWIGAI_AUTH_TOKEN"
	Groups     []defs.SpecGroup // parsed from OpenAPI specs
}

// LoadInstancesFromDir reads all *.yaml files from dir and returns an MCPInstance per config.
// Spec fetching happens in parallel across configs. Any failure is fatal.
func LoadInstancesFromDir(dir string) ([]MCPInstance, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading config dir %s: %w", dir, err)
	}

	var yamlFiles []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml") {
			yamlFiles = append(yamlFiles, e.Name())
		}
	}
	sort.Strings(yamlFiles)

	if len(yamlFiles) == 0 {
		return nil, fmt.Errorf("no YAML config files found in %s", dir)
	}

	cacheDir := filepath.Join(dir, "..", ".cache", "specs")

	type result struct {
		instance MCPInstance
		err      error
	}

	results := make([]result, len(yamlFiles))
	var wg sync.WaitGroup

	for idx, filename := range yamlFiles {
		wg.Add(1)
		go func(i int, fname string) {
			defer wg.Done()
			inst, err := loadInstance(filepath.Join(dir, fname), cacheDir)
			results[i] = result{instance: inst, err: err}
		}(idx, filename)
	}
	wg.Wait()

	instances := make([]MCPInstance, 0, len(yamlFiles))
	for _, r := range results {
		if r.err != nil {
			return nil, r.err
		}
		instances = append(instances, r.instance)
	}

	return instances, nil
}

func loadInstance(configPath, cacheDir string) (MCPInstance, error) {
	slug := strings.TrimSuffix(filepath.Base(configPath), filepath.Ext(configPath))

	cfg, err := config.Load(configPath)
	if err != nil {
		return MCPInstance{}, fmt.Errorf("loading config %s: %w", configPath, err)
	}

	// Sort spec names for deterministic ordering.
	specNames := make([]string, 0, len(cfg.Specs))
	for name := range cfg.Specs {
		specNames = append(specNames, name)
	}
	sort.Strings(specNames)

	var groups []defs.SpecGroup
	for _, name := range specNames {
		source := cfg.Specs[name]
		data, err := spec.FetchSpec(source, cacheDir)
		if err != nil {
			return MCPInstance{}, fmt.Errorf("fetching spec %q for %s: %w", name, slug, err)
		}
		group, err := spec.ParseSpec(data, name)
		if err != nil {
			return MCPInstance{}, fmt.Errorf("parsing spec %q for %s: %w", name, slug, err)
		}
		groups = append(groups, *group)
	}

	return MCPInstance{
		Slug:       slug,
		BinaryName: cfg.BinaryName,
		AuthEnvVar: cfg.Auth.EnvVar,
		Groups:     groups,
	}, nil
}
