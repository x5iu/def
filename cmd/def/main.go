package main

import (
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/x5iu/def/internal/defgen"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("def: ")
	log.SetOutput(os.Stderr)

	rootCmd := &cobra.Command{
		Use:   "def",
		Short: "SQL query code generator for Go",
		Long: `def is a code generation tool similar to Google Wire that scans Go code
with def.Query + def.Filter definitions and generates interface definitions
with SQL comments.

It parses struct definitions with 'db' and 'foreign_key' tags, reads table
bindings from def.Init + def.BindTable[T]("table") calls, and analyzes
def.Query + def.Filter expressions to generate SQL WHERE clauses.`,
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	var outputPath string
	var buildTags string

	generateCmd := &cobra.Command{
		Use:   "generate [patterns...]",
		Short: "Generate SQL query interface code",
		Long: `Generate interface definitions with SQL comments from def.Query + def.Filter
definitions in Go source files.

Examples:
  def generate ./...              Generate code for all packages
  def generate ./internal/repo    Generate code for specific package
  def generate .                  Generate code for current package (default)
  def generate -o query_gen.go .  Generate to custom output file
  def generate --tags "!test" .   Generate with build tags

Supported filter expressions:
  - Comparison: user.ID == id, user.Age > 18
  - Literals: user.Status == "active", user.Age == 18
  - IN query: def.In(user.ID, ids)
  - AND: user.Status == "active" && user.ID == id
  - OR: user.Status == "active" || user.Status == "pending"
  - Nested: (a == b && c == d) || e == f
  - Foreign key: project.User.Name == username (generates subquery)
  - Functions: def.Count[int64](t.ID) > 0, def.Func[string]("COALESCE", user.Name, "x") != "x"`,
		Aliases: []string{"gen"},
		Args:    cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
			patterns := args
			if len(patterns) == 0 {
				patterns = []string{"."}
			}

			wd, err := os.Getwd()
			if err != nil {
				log.Fatalf("failed to get working directory: %v", err)
			}

			opts := &defgen.GenerateOptions{
				Output: outputPath,
				Tags:   buildTags,
			}

			for _, pattern := range patterns {
				if err := defgen.Generate(wd, pattern, opts); err != nil {
					log.Fatalf("generate failed: %v", err)
				}
				log.Printf("generated code for %s", pattern)
			}
		},
	}

	generateCmd.Flags().StringVarP(&outputPath, "output", "o", "", "output file path (default: def_gen.go in package directory)")
	generateCmd.Flags().StringVar(&buildTags, "tags", "", "build tags to add (e.g., \"!test\" generates //go:build !test)")
	if err := generateCmd.MarkFlagFilename("output", "go"); err != nil {
		log.Fatalf("failed to mark flag filename: %v", err)
	}

	rootCmd.AddCommand(generateCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
