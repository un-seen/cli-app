package command

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hedwigai/cli/generated"
	"github.com/hedwigai/cli/internal/defs"
)

var (
	version    string
	commitHash string

	// Global flags.
	rawOutput  bool
	verbose    bool
	timeout    int
	baseURL    string
	outputFile string
)

// Execute builds the CLI command tree and runs it.
func Execute(ver, commit string) error {
	version = ver
	commitHash = commit

	rootCmd := buildRootCommand()
	return rootCmd.Execute()
}

func buildRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           generated.BinaryName,
		Short:         fmt.Sprintf("%s CLI", generated.BinaryName),
		SilenceUsage: true,
	}

	rootCmd.PersistentFlags().BoolVar(&rawOutput, "raw", false, "Output raw JSON without pretty-printing")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Show request details (URL, method, headers)")
	rootCmd.PersistentFlags().IntVar(&timeout, "timeout", 30, "Request timeout in seconds")
	rootCmd.PersistentFlags().StringVar(&baseURL, "base-url", "", "Override spec's server URL")
	rootCmd.PersistentFlags().StringVar(&outputFile, "output", "", "Write response body to file instead of stdout")

	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%s version %s (commit %s)\n", generated.BinaryName, version, commitHash)
		},
	})

	groups := generated.SpecGroups
	singleSpec := len(groups) == 1

	for i := range groups {
		group := &groups[i]
		if singleSpec {
			addTagCommands(rootCmd, group)
		} else {
			groupCmd := &cobra.Command{
				Use:   group.Name,
				Short: fmt.Sprintf("%s commands", group.Name),
			}
			addTagCommands(groupCmd, group)
			rootCmd.AddCommand(groupCmd)
		}
	}

	return rootCmd
}

func addTagCommands(parent *cobra.Command, group *defs.SpecGroup) {
	tagOps := make(map[string][]*defs.Operation)
	for i := range group.Operations {
		op := &group.Operations[i]
		tagOps[op.Tag] = append(tagOps[op.Tag], op)
	}

	tags := make([]string, 0, len(tagOps))
	for tag := range tagOps {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	// If there is only one tag and it matches the parent (group) name,
	// add operations directly to the parent to avoid redundant nesting.
	if len(tags) == 1 && tags[0] == parent.Use {
		for _, op := range tagOps[tags[0]] {
			parent.AddCommand(buildOperationCommand(op, group))
		}
		return
	}

	for _, tag := range tags {
		ops := tagOps[tag]
		tagCmd := &cobra.Command{
			Use:   tag,
			Short: fmt.Sprintf("%s operations", tag),
		}

		for _, op := range ops {
			opCmd := buildOperationCommand(op, group)
			tagCmd.AddCommand(opCmd)
		}

		parent.AddCommand(tagCmd)
	}
}

func buildOperationCommand(op *defs.Operation, group *defs.SpecGroup) *cobra.Command {
	cmd := &cobra.Command{
		Use:   op.OperationID,
		Short: op.Summary,
		Long:  op.Description,
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeOperation(cmd, op, group)
		},
	}

	for i := range op.Parameters {
		addParameterFlag(cmd, &op.Parameters[i])
	}

	if op.HasBody {
		cmd.Flags().String("body", "", "Request body (JSON string)")
		cmd.Flags().String("body-file", "", "Path to file containing request body")

		for i := range op.BodyFields {
			addBodyFieldFlag(cmd, &op.BodyFields[i])
		}
	}

	return cmd
}

func addParameterFlag(cmd *cobra.Command, param *defs.Parameter) {
	flagName := param.Name
	if param.In == "header" {
		flagName = "header-" + param.Name
	}

	desc := param.Description
	if len(param.Enum) > 0 {
		desc += fmt.Sprintf(" (allowed: %s)", strings.Join(param.Enum, ", "))
	}

	switch param.Type {
	case "int":
		def := 0
		if param.Default != "" {
			def, _ = strconv.Atoi(param.Default)
		}
		cmd.Flags().Int(flagName, def, desc)
	case "bool":
		def := param.Default == "true"
		cmd.Flags().Bool(flagName, def, desc)
	case "float":
		def := 0.0
		if param.Default != "" {
			def, _ = strconv.ParseFloat(param.Default, 64)
		}
		cmd.Flags().Float64(flagName, def, desc)
	default:
		cmd.Flags().String(flagName, param.Default, desc)
	}

	if param.Required {
		_ = cmd.MarkFlagRequired(flagName)
	}
}

func addBodyFieldFlag(cmd *cobra.Command, field *defs.BodyField) {
	switch field.Type {
	case "int":
		cmd.Flags().Int(field.Name, 0, field.Description)
	case "bool":
		cmd.Flags().Bool(field.Name, false, field.Description)
	case "float":
		cmd.Flags().Float64(field.Name, 0, field.Description)
	default:
		cmd.Flags().String(field.Name, "", field.Description)
	}

	if field.Required {
		_ = cmd.MarkFlagRequired(field.Name)
	}
}
