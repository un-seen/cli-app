package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/hedwigai/cli/internal/codegen"
	"github.com/hedwigai/cli/internal/config"
	"github.com/hedwigai/cli/internal/defs"
	"github.com/hedwigai/cli/internal/spec"
)

func main() {
	configPath := "config.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Build error: %v\n", err)
		os.Exit(1)
	}

	cacheDir := filepath.Join(".cache", "specs")
	var groups []defs.SpecGroup

	// Sort spec names for deterministic output.
	specNames := make([]string, 0, len(cfg.Specs))
	for name := range cfg.Specs {
		specNames = append(specNames, name)
	}
	sort.Strings(specNames)

	for _, name := range specNames {
		source := cfg.Specs[name]

		if source == "" {
			fmt.Fprintf(os.Stderr, "Build error: spec %q has empty source\n", name)
			os.Exit(1)
		}

		fmt.Printf("Resolving spec %q from %s\n", name, source)
		data, err := spec.FetchSpec(source, cacheDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Build error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("  Fetched %d bytes\n", len(data))

		group, err := spec.ParseSpec(data, name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Build error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("  Parsed %d operations in group %q\n", len(group.Operations), group.Name)

		groups = append(groups, *group)
	}

	input := codegen.GenerateInput{
		Groups:     groups,
		AuthEnvVar: cfg.Auth.EnvVar,
		BinaryName: cfg.BinaryName,
	}

	if err := codegen.Generate(input, "generated"); err != nil {
		fmt.Fprintf(os.Stderr, "Build error: %v\n", err)
		os.Exit(1)
	}

	total := 0
	for _, g := range groups {
		total += len(g.Operations)
	}
	fmt.Printf("Generated %d operations across %d spec group(s)\n", total, len(groups))
}
