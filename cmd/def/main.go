package main

import (
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/x5iu/def/internal/defgen"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("def: ")
	log.SetOutput(os.Stderr)

	rootCmd := newRootCommand()
	if err := rootCmd.Execute(); err != nil {
		log.Printf("%v", err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "def",
		Short: "SQL query code generator for Go",
		Long: `def is a code generation tool similar to Google Wire that scans Go code
with def.Query, def.Create, def.Update, def.Delete definitions and generates
interface definitions with SQL comments.

It parses struct definitions with 'db' and 'foreign_key' tags, reads table
bindings from def.Init + def.BindTable[T]("table") calls, and analyzes
expressions to generate SQL statements (SELECT, INSERT, UPDATE, DELETE).`,
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	var outputPath string
	var buildTags string
	var interfaceName string
	var defcCmd string
	var defcFeatures string
	var defcGenerate bool
	var txIsolation string
	var withTx bool
	var withTxFnType string

	generateCmd := &cobra.Command{
		Use:   "generate [patterns...]",
		Short: "Generate SQL interface code",
		Long: `Generate interface definitions with SQL comments from def.Query, def.Create,
def.Update, def.Delete definitions in Go source files.

Examples:
  def generate ./...              Generate code for all packages
  def generate ./internal/repo    Generate code for specific package
  def generate .                  Generate code for current package (default)
  def generate -o query_gen.go .  Generate to custom output file
  def generate --tags def .       Include files with //go:build def
  def generate --defc "go tool defc" .  Customize defc command
  def generate --tx-isolation serializable .  Add WithTx isolation metadata
  def generate --tx .  Always generate WithTx method
  def generate --tx --tx-type TxStore .  Customize WithTx fn argument type
  def generate --tags def --defc-generate --defc-features "sqlx/rebind,sqlx/in" -o store.go .
                                        Generate intermediate + implementation in one step

Supported expressions:
  Query (SELECT):
    - def.Query(def.Filter(user.ID == id))
    - def.Column(user.Name), def.Column(def.Count(user.ID))
    - def.Limit(10), def.Offset(20), def.Limit(pageSize)

  Create (INSERT):
    - def.Create(user)                          // entity mode
    - def.Create(def.Set(user.Name, name), ...) // field mode

  Update:
    - def.Update(user, def.Filter(user.ID == user.ID))  // entity mode
    - def.Update(def.Set(user.Name, name), def.Filter(user.ID == id))
    - def.Update(def.Set(user.Count, user.Count+1), def.Set(user.UpdatedAt, def.Func[any]("now")), def.Filter(user.ID == id))

  Delete:
    - def.Delete(def.Filter(user.ID == id))

  Filter expressions:
    - Comparison: user.ID == id, user.Age > 18
    - Literals: user.Status == "active"
    - IN query: def.In(user.ID, ids)
    - Boolean: a && b, a || b, (a && b) || c
    - Foreign key: project.User.Name == name (generates subquery)`,
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

			mappedIsolation, err := defgen.ParseTxIsolationFlag(txIsolation)
			if err != nil {
				log.Fatalf("generate failed: %v", err)
			}

			opts := &defgen.GenerateOptions{
				Output:        outputPath,
				Tags:          buildTags,
				InterfaceName: interfaceName,
				DefcCmd:       defcCmd,
				DefcFeatures:  defcFeatures,
				DefcGenerate:  defcGenerate,
				TxIsolation:   mappedIsolation,
				WithTx:        withTx,
				WithTxFnType:  withTxFnType,
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
	generateCmd.Flags().StringVar(&buildTags, "tags", "", "build tags for parsing source files (e.g., \"def\" to include files with //go:build def)")
	generateCmd.Flags().StringVarP(&interfaceName, "interface", "T", "", "name of the generated interface")
	generateCmd.Flags().StringVar(&defcCmd, "defc", "", "defc command for //go:generate directive (e.g., \"go tool defc\", \"defc\", default: \"go run -mod=mod github.com/x5iu/defc@latest\")")
	generateCmd.Flags().StringVar(&defcFeatures, "defc-features", "", "additional defc features to include in //go:generate directive (e.g., \"sqlx/rebind,sqlx/in\")")
	generateCmd.Flags().BoolVar(&defcGenerate, "defc-generate", false, "directly invoke defc to generate the implementation file, instead of emitting a //go:generate directive")
	generateCmd.Flags().StringVar(&txIsolation, "tx-isolation", "", "transaction isolation level for WithTx comment ("+strings.Join(defgen.SupportedTxIsolationValues(), ", ")+")")
	generateCmd.Flags().BoolVar(&withTx, "tx", false, "always generate WithTx method when source interface doesn't declare it")
	generateCmd.Flags().StringVar(&withTxFnType, "tx-type", "", "override fn argument type in generated WithTx signature (requires source WithTx or --tx)")
	if err := generateCmd.MarkFlagFilename("output", "go"); err != nil {
		log.Fatalf("failed to mark flag filename: %v", err)
	}

	rootCmd.AddCommand(generateCmd)

	return rootCmd
}
