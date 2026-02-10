package defgen

import (
	"bytes"
	"fmt"
	"go/format"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"github.com/x5iu/defc/gen"
	"golang.org/x/tools/imports"
)

// GenerateOptions contains options for code generation.
type GenerateOptions struct {
	// Output specifies the output file path. If empty, defaults to "def_gen.go" in the package directory.
	Output string
	// Tags specifies build tags for parsing source files (e.g., "def" to include files with //go:build def).
	Tags string
	// InterfaceName specifies the name of the generated interface. If empty, it will try to find a matching interface.
	InterfaceName string
	// DefcCmd specifies the defc command used in the generated //go:generate directive.
	// Examples: "go tool defc", "go run -mod=mod github.com/x5iu/defc@latest", "/usr/local/bin/defc".
	// If empty, defaults to "go run -mod=mod github.com/x5iu/defc@latest".
	DefcCmd string
	// DefcFeatures specifies additional defc features to include in the generated //go:generate directive.
	// These are appended to the auto-detected features (e.g., "sqlx/callback").
	// Example: "sqlx/rebind,sqlx/in".
	DefcFeatures string
	// DefcGenerate, when true, directly invokes defc code generation after writing
	// the intermediate file, instead of emitting a //go:generate directive.
	DefcGenerate bool
}

// Generate generates code for packages matching the given pattern.
func Generate(wd, pattern string, opts *GenerateOptions) error {
	if opts == nil {
		opts = &GenerateOptions{}
	}

	pkgs, err := Load(wd, pattern, opts.Tags)
	if err != nil {
		return err
	}
	if len(pkgs) == 0 {
		return nil
	}

	// When generating for multiple packages (e.g., ./...), absolute output paths would cause overwrites.
	if len(pkgs) > 1 && opts.Output != "" && filepath.IsAbs(opts.Output) {
		return fmt.Errorf("absolute output path is not supported when pattern matches multiple packages: %s", opts.Output)
	}

	for _, pkg := range pkgs {
		// Parse def-related definitions
		if err := Parse(pkg); err != nil {
			return fmt.Errorf("%s: %w", pkg.PkgPath, err)
		}

		// If no methods found, nothing to generate for this package.
		if len(pkg.Methods) == 0 && len(pkg.MutationMethods) == 0 {
			continue
		}

		outputPath, err := determineOutputPath(pkg, opts.Output)
		if err != nil {
			return err
		}

		// Generate the code
		code, err := generateCode(pkg, opts)
		if err != nil {
			return err
		}

		// Fix imports and format using goimports.
		code, err = imports.Process(outputPath, code, &imports.Options{
			Comments:  true,
			TabIndent: true,
			TabWidth:  8,
		})
		if err != nil {
			return fmt.Errorf("goimports failed for %s: %w", outputPath, err)
		}

		// Write the file
		if err := os.WriteFile(outputPath, code, 0o644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}

		// Directly invoke defc code generation if requested
		if opts.DefcGenerate {
			if err := invokeDefc(outputPath, code, pkg, opts); err != nil {
				return fmt.Errorf("defc generate failed for %s: %w", outputPath, err)
			}
		}
	}

	return nil
}

// determineOutputPath determines the output file path.
func determineOutputPath(pkg *Package, outputOpt string) (string, error) {
	if outputOpt == "" {
		return filepath.Join(pkg.Dir, "def_gen.go"), nil
	}
	if filepath.IsAbs(outputOpt) {
		return outputOpt, nil
	}
	return filepath.Join(pkg.Dir, outputOpt), nil
}

// generateCode generates the code content.
func generateCode(pkg *Package, opts *GenerateOptions) ([]byte, error) {
	var buf bytes.Buffer

	// Analyze relations
	if err := AnalyzeRelations(pkg); err != nil {
		return nil, fmt.Errorf("failed to analyze relations: %w", err)
	}

	// Add build tag if tags are specified (negated to exclude generated file when source files are included)
	if opts != nil && opts.Tags != "" {
		buildConstraint := generateBuildConstraint(opts.Tags)
		buf.WriteString(buildConstraint)
		buf.WriteString("\n\n")
	}

	// Write package declaration
	buf.WriteString(fmt.Sprintf("package %s\n\n", pkg.PkgName))

	// Collect imports
	imports := collectImports(pkg)
	if len(imports) > 0 {
		buf.WriteString("import (\n")
		for _, imp := range imports {
			buf.WriteString(fmt.Sprintf("\t%q\n", imp))
		}
		buf.WriteString(")\n\n")
	}

	// Generate slice type aliases for Callback support
	if len(pkg.SliceTypeAliases) > 0 {
		buf.WriteString("// Slice type aliases for Callback support\n")
		for _, alias := range pkg.SliceTypeAliases {
			buf.WriteString(fmt.Sprintf("type %s []%s\n", alias.AliasName, alias.ElemType))
		}
		buf.WriteString("\n")
	}

	// Generate cache utilities for avoiding circular references
	if len(pkg.CallbackMethods) > 0 {
		buf.WriteString(generateCacheUtilities())
	}

	// Determine interface name
	var interfaceInfo *InterfaceInfo
	if opts != nil && opts.InterfaceName != "" {
		// Use user-specified interface name
		interfaceInfo = &InterfaceInfo{
			Name: opts.InterfaceName,
		}
	} else {
		// Find the interface that matches our methods
		interfaceInfo = findMatchingInterface(pkg)
		if interfaceInfo == nil {
			// No matching interface found, return error
			return nil, fmt.Errorf("no matching interface found for methods; use -T to specify interface name")
		}
	}

	// Generate go:generate directive for defc (skipped when DefcGenerate is true)
	if opts == nil || !opts.DefcGenerate {
		defcCmd := "go run -mod=mod github.com/x5iu/defc@latest"
		if opts != nil && opts.DefcCmd != "" {
			defcCmd = opts.DefcCmd
		}
		var extraFeatures string
		if opts != nil {
			extraFeatures = opts.DefcFeatures
		}
		defcOutput := strings.ToLower(interfaceInfo.Name) + "_impl.go"
		features := buildDefcFeatures(len(pkg.CallbackMethods) > 0, extraFeatures)
		if features != "" {
			buf.WriteString(fmt.Sprintf("//go:generate %s generate --features %s -T %s -o %s\n\n",
				defcCmd, features, interfaceInfo.Name, defcOutput))
		} else {
			buf.WriteString(fmt.Sprintf("//go:generate %s generate -T %s -o %s\n\n",
				defcCmd, interfaceInfo.Name, defcOutput))
		}
	}

	// Generate interface
	buf.WriteString(fmt.Sprintf("type %s interface {\n", interfaceInfo.Name))

	// Generate public query methods with SQL comments
	for _, method := range pkg.Methods {
		sql, err := GenerateSQL(pkg, method)
		if err != nil {
			return nil, fmt.Errorf("failed to generate SQL for method %s: %w", method.Name, err)
		}

		// Write method comment with SQL - first line uses //, SQL uses /* */
		formattedSQL := FormatSQL(sql)
		buf.WriteString(fmt.Sprintf("\t// %s query constbind\n", method.Name))
		buf.WriteString(fmt.Sprintf("\t/* %s */\n", formattedSQL))

		// Write method signature
		sig := generateMethodSignature(pkg, method)
		buf.WriteString(fmt.Sprintf("\t%s\n\n", sig))
	}

	// Generate mutation methods (Create/Update/Delete) with SQL comments
	for _, method := range pkg.MutationMethods {
		sql, err := GenerateMutationSQL(pkg, method)
		if err != nil {
			return nil, fmt.Errorf("failed to generate SQL for method %s: %w", method.Name, err)
		}

		// Write method comment with SQL - first line uses //, SQL uses /* */
		formattedSQL := FormatSQL(sql)
		// Use "query constbind" when RETURNING is used, otherwise "exec constbind"
		if method.ReturnType != nil {
			buf.WriteString(fmt.Sprintf("\t// %s query constbind\n", method.Name))
		} else {
			buf.WriteString(fmt.Sprintf("\t// %s exec constbind\n", method.Name))
		}
		buf.WriteString(fmt.Sprintf("\t/* %s */\n", formattedSQL))

		// Write method signature
		sig := generateMutationMethodSignature(pkg, method)
		buf.WriteString(fmt.Sprintf("\t%s\n\n", sig))
	}

	// Generate private relation methods
	for _, rm := range pkg.RelationMethods {
		buf.WriteString(generateRelationMethodComment(rm))
		buf.WriteString(generateRelationMethodSignature(pkg, rm))
		buf.WriteString("\n")
	}

	buf.WriteString("}\n\n")

	// Generate Callback methods
	for _, cb := range pkg.CallbackMethods {
		buf.WriteString(generateCallbackMethod(cb, interfaceInfo.Name))
	}

	// Generate slice Callback methods
	for _, alias := range pkg.SliceTypeAliases {
		buf.WriteString(generateSliceCallbackMethod(alias, interfaceInfo.Name))
	}

	// Format the code
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		// Return unformatted if formatting fails
		return buf.Bytes(), nil
	}

	return formatted, nil
}

// collectImports collects necessary imports for the generated code.
func collectImports(pkg *Package) []string {
	imports := make(map[string]bool)

	// Check if we need context
	for _, method := range pkg.Methods {
		for _, param := range method.Params {
			if param.Name == "ctx" {
				imports["context"] = true
			}
		}
		// Also check the original interface methods
		for _, iface := range pkg.Interfaces {
			for _, m := range iface.Methods {
				for _, p := range m.Params {
					if isContextType(p.Type) {
						imports["context"] = true
					}
				}
			}
		}
	}

	// Always include context if methods use it (based on interface definition)
	imports["context"] = true

	// Add database/sql if we have mutation methods that return sql.Result (no RETURNING)
	for _, method := range pkg.MutationMethods {
		if method.ReturnType == nil {
			imports["database/sql"] = true
			break
		}
	}

	// Add fmt and reflect if we have callback methods (for cache utilities)
	if len(pkg.CallbackMethods) > 0 {
		imports["errors"] = true
		imports["fmt"] = true
		imports["reflect"] = true
	}

	var result []string
	for imp := range imports {
		result = append(result, imp)
	}
	return result
}

// findMatchingInterface finds an interface that matches the generated methods.
func findMatchingInterface(pkg *Package) *InterfaceInfo {
	// Create a set of method names
	methodNames := make(map[string]bool)
	for _, m := range pkg.Methods {
		methodNames[m.Name] = true
	}
	for _, m := range pkg.MutationMethods {
		methodNames[m.Name] = true
	}

	// Find an interface that has all these methods
	for _, iface := range pkg.Interfaces {
		hasAll := true
		for name := range methodNames {
			found := false
			for _, m := range iface.Methods {
				if m.Name == name {
					found = true
					break
				}
			}
			if !found {
				hasAll = false
				break
			}
		}
		if hasAll && len(iface.Methods) > 0 {
			return iface
		}
	}

	return nil
}

// generateMethodSignature generates a method signature string.
func generateMethodSignature(pkg *Package, method *QueryMethod) string {
	var sb strings.Builder

	sb.WriteString(method.Name)
	sb.WriteString("(")

	// Parameters - need to include ctx context.Context at the start
	params := []string{"ctx context.Context"}
	for _, p := range method.Params {
		params = append(params, fmt.Sprintf("%s %s", p.Name, formatType(pkg, p.Type)))
	}
	sb.WriteString(strings.Join(params, ", "))
	sb.WriteString(")")

	// Return type
	sb.WriteString(" ")
	if method.ReturnType.Type != nil {
		sb.WriteString("(")
		sb.WriteString(formatType(pkg, method.ReturnType.Type))
		sb.WriteString(", error)")
	} else {
		sb.WriteString("error")
	}

	return sb.String()
}

// generateMutationMethodSignature generates a method signature for mutation methods.
func generateMutationMethodSignature(pkg *Package, method *MutationMethod) string {
	var sb strings.Builder

	sb.WriteString(method.Name)
	sb.WriteString("(")

	// Parameters - need to include ctx context.Context at the start
	params := []string{"ctx context.Context"}
	for _, p := range method.Params {
		params = append(params, fmt.Sprintf("%s %s", p.Name, formatType(pkg, p.Type)))
	}
	sb.WriteString(strings.Join(params, ", "))
	sb.WriteString(")")

	// Return type depends on whether RETURNING is used
	if method.ReturnType != nil {
		// Use the actual return type when RETURNING is used
		sb.WriteString(" (")
		sb.WriteString(formatType(pkg, method.ReturnType.Type))
		sb.WriteString(", error)")
	} else {
		// Default: (sql.Result, error)
		sb.WriteString(" (sql.Result, error)")
	}

	return sb.String()
}

// formatType formats a type for output, using short names for types in the same package.
func formatType(pkg *Package, t types.Type) string {
	if t == nil {
		return ""
	}
	// Use a qualifier that omits the current package
	qualifier := func(p *types.Package) string {
		if p == pkg.TypesPkg {
			return ""
		}
		return p.Name()
	}
	return types.TypeString(t, qualifier)
}

// generateCacheUtilities generates the cache helper functions for avoiding circular references.
func generateCacheUtilities() string {
	return `// Cache utilities for avoiding circular references in Callback
type callbackCache map[string]any

type callbackCacheKey struct{}

var ErrCallbackCacheRequired = errors.New("callback requires WithCache context; see WithCache()")

// WithCache creates a new context with an empty cache for Callback methods.
func WithCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, callbackCacheKey{}, make(callbackCache))
}

func setCache[T any](ctx context.Context, identifier string, value T) {
	cachemap, ok := ctx.Value(callbackCacheKey{}).(callbackCache)
	if !ok {
		return
	}
	cachekey := fmt.Sprintf("%s:%s", reflect.TypeFor[T]().String(), identifier)
	cachemap[cachekey] = value
}

func requireCache[T any](ctx context.Context, identifier string) (v T, ok bool, err error) {
	cachemap, cacheOK := ctx.Value(callbackCacheKey{}).(callbackCache)
	if !cacheOK {
		return v, false, ErrCallbackCacheRequired
	}
	cachekey := fmt.Sprintf("%s:%s", reflect.TypeFor[T]().String(), identifier)
	value, found := cachemap[cachekey]
	if !found {
		return v, false, nil
	}
	return value.(T), true, nil
}

`
}

// generateRelationMethodComment generates the SQL comment for a relation method.
func generateRelationMethodComment(rm *RelationMethod) string {
	var sb strings.Builder

	// Build the first line with method name and options using //
	sb.WriteString(fmt.Sprintf("\t// %s query constbind", rm.MethodName))
	if rm.IsSlice {
		// Add WRAP option for slice return types
		sb.WriteString(fmt.Sprintf(" WRAP=(*%ss)", rm.RefTypeName))
	}
	sb.WriteString("\n")

	// Build and format the SQL using /* */
	sql := fmt.Sprintf("SELECT * FROM %s WHERE %s = ${%s}",
		rm.RefTableName, rm.WhereColumn, rm.ParamName)
	formattedSQL := FormatSQL(sql)
	sb.WriteString(fmt.Sprintf("\t/* %s */\n", formattedSQL))

	return sb.String()
}

// generateRelationMethodSignature generates the method signature for a relation method.
func generateRelationMethodSignature(pkg *Package, rm *RelationMethod) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\t%s(ctx context.Context, %s %s) (",
		rm.MethodName, rm.ParamName, formatType(pkg, rm.ParamType)))
	if rm.IsSlice {
		sb.WriteString(fmt.Sprintf("[]%s", formatType(pkg, rm.RefType.(*types.Slice).Elem())))
	} else {
		sb.WriteString(formatType(pkg, rm.RefType))
	}
	sb.WriteString(", error)\n")
	return sb.String()
}

// generateCallbackMethod generates a Callback method for a struct.
func generateCallbackMethod(cb *CallbackMethod, interfaceName string) string {
	var sb strings.Builder

	// Method signature
	receiverName := strings.ToLower(cb.StructName[:1])
	sb.WriteString(fmt.Sprintf("// %s Callback - loads related data\n", cb.StructName))
	sb.WriteString(fmt.Sprintf("func (%s %s) Callback(ctx context.Context, q %s) error {\n",
		receiverName, cb.StructTypeName, interfaceName))

	// Fail fast when callback cache context is missing.
	sb.WriteString("\tif _, ok := ctx.Value(callbackCacheKey{}).(callbackCache); !ok {\n")
	sb.WriteString("\t\treturn ErrCallbackCacheRequired\n")
	sb.WriteString("\t}\n\n")

	// Cache self first to prevent circular references
	if cb.IDField != nil {
		sb.WriteString(fmt.Sprintf("\tsetCache(ctx, fmt.Sprintf(\"%s:%%v\", %s.%s), %s)\n\n",
			cb.IDField.DBName, receiverName, cb.IDField.GoName, receiverName))
	}

	// Load each related field
	for _, field := range cb.Fields {
		if field.IsSlice {
			// One-to-many: check cache then query
			aliasName := field.AliasTypeName
			if aliasName == "" {
				aliasName = field.FieldName
			}
			sb.WriteString(fmt.Sprintf("\tif cached, ok, err := requireCache[%s](ctx, fmt.Sprintf(\"%s:%%v\", %s.%s)); err != nil {\n",
				aliasName, field.CacheKey, receiverName, field.KeyFieldName))
			sb.WriteString("\t\treturn err\n")
			sb.WriteString("\t} else if ok {\n")
			if field.FieldIsAlias {
				sb.WriteString(fmt.Sprintf("\t\t%s.%s = cached\n", receiverName, field.FieldName))
			} else {
				sb.WriteString(fmt.Sprintf("\t\t%s.%s = %s(cached)\n", receiverName, field.FieldName, field.SliceType))
			}
			sb.WriteString("\t} else {\n")
			sb.WriteString("\t\tvar err error\n")
			if field.FieldIsAlias {
				sb.WriteString(fmt.Sprintf("\t\tvar tmp %s\n", field.SliceType))
				sb.WriteString(fmt.Sprintf("\t\ttmp, err = q.%s(ctx, %s.%s)\n",
					field.MethodName, receiverName, field.KeyFieldName))
				sb.WriteString("\t\tif err != nil {\n")
				sb.WriteString("\t\t\treturn err\n")
				sb.WriteString("\t\t}\n")
				sb.WriteString(fmt.Sprintf("\t\t%s.%s = %s(tmp)\n", receiverName, field.FieldName, aliasName))
			} else {
				sb.WriteString(fmt.Sprintf("\t\t%s.%s, err = q.%s(ctx, %s.%s)\n",
					receiverName, field.FieldName, field.MethodName, receiverName, field.KeyFieldName))
				sb.WriteString("\t\tif err != nil {\n")
				sb.WriteString("\t\t\treturn err\n")
				sb.WriteString("\t\t}\n")
			}
			sb.WriteString(fmt.Sprintf("\t\tsetCache(ctx, fmt.Sprintf(\"%s:%%v\", %s.%s), %s(%s.%s))\n",
				field.CacheKey, receiverName, field.KeyFieldName, aliasName, receiverName, field.FieldName))
			sb.WriteString("\t}\n\n")
		} else {
			// Many-to-one: check cache then query
			refTypeName := field.RefTypeName
			sb.WriteString(fmt.Sprintf("\tif cached, ok, err := requireCache[*%s](ctx, fmt.Sprintf(\"%s:%%v\", %s.%s)); err != nil {\n",
				refTypeName, field.CacheKey, receiverName, field.KeyFieldName))
			sb.WriteString("\t\treturn err\n")
			sb.WriteString("\t} else if ok {\n")
			sb.WriteString(fmt.Sprintf("\t\t%s.%s = cached\n", receiverName, field.FieldName))
			sb.WriteString("\t} else {\n")
			sb.WriteString("\t\tvar err error\n")
			sb.WriteString(fmt.Sprintf("\t\t%s.%s, err = q.%s(ctx, %s.%s)\n",
				receiverName, field.FieldName, field.MethodName, receiverName, field.KeyFieldName))
			sb.WriteString("\t\tif err != nil {\n")
			sb.WriteString("\t\t\treturn err\n")
			sb.WriteString("\t\t}\n")
			sb.WriteString(fmt.Sprintf("\t\tsetCache(ctx, fmt.Sprintf(\"%s:%%v\", %s.%s), %s.%s)\n",
				field.CacheKey, receiverName, field.KeyFieldName, receiverName, field.FieldName))
			sb.WriteString("\t}\n\n")
		}
	}

	sb.WriteString("\treturn nil\n")
	sb.WriteString("}\n\n")

	return sb.String()
}

// generateSliceCallbackMethod generates a Callback method for a slice type alias.
func generateSliceCallbackMethod(alias *SliceTypeAlias, interfaceName string) string {
	var sb strings.Builder

	// Method signature
	receiverName := strings.ToLower(alias.AliasName[:1])
	sb.WriteString(fmt.Sprintf("// %s Callback - iterates and calls each element's Callback\n", alias.AliasName))
	sb.WriteString(fmt.Sprintf("func (%s *%s) Callback(ctx context.Context, q %s) error {\n",
		receiverName, alias.AliasName, interfaceName))

	sb.WriteString(fmt.Sprintf("\tfor _, item := range *%s {\n", receiverName))
	sb.WriteString("\t\tif err := item.Callback(ctx, q); err != nil {\n")
	sb.WriteString("\t\t\treturn err\n")
	sb.WriteString("\t\t}\n")
	sb.WriteString("\t}\n")
	sb.WriteString("\treturn nil\n")
	sb.WriteString("}\n\n")

	return sb.String()
}

// invokeDefc directly calls the defc code generation library to produce the final
// implementation file (e.g., store_impl.go) from the intermediate file (e.g., store.go).
func invokeDefc(intermediateFile string, intermediateCode []byte, pkg *Package, opts *GenerateOptions) error {
	// Determine interface name
	interfaceName := opts.InterfaceName
	if interfaceName == "" {
		iface := findMatchingInterface(pkg)
		if iface != nil {
			interfaceName = iface.Name
		}
	}
	if interfaceName == "" {
		return fmt.Errorf("cannot determine interface name for defc; use -T flag")
	}

	// Detect interface position in generated file
	pkgName, _, pos, err := gen.DetectTargetDecl(intermediateFile, intermediateCode, interfaceName)
	if err != nil {
		return fmt.Errorf("failed to detect interface %s: %w", interfaceName, err)
	}

	// Build features list
	features := buildDefcFeatures(len(pkg.CallbackMethods) > 0, opts.DefcFeatures)
	var feats []string
	if features != "" {
		feats = strings.Split(features, ",")
	}
	// Align invokeDefc with defc CLI default behavior (since defc v1.37.0),
	// where sqlx/future is enabled unless defc itself is built with `legacy`.
	if !containsFeature(feats, "sqlx/future") {
		feats = append(feats, "sqlx/future")
	}

	// Invoke defc code generation
	var buf bytes.Buffer
	builder := gen.NewCliBuilder(gen.ModeSqlx).
		WithPkg(pkgName).
		WithPwd(filepath.Dir(intermediateFile)).
		WithFile(intermediateFile, intermediateCode).
		WithPos(pos).
		WithFeats(feats)

	if err := builder.Build(&buf); err != nil {
		return fmt.Errorf("defc build failed: %w", err)
	}

	// Write output file
	outputPath := filepath.Join(filepath.Dir(intermediateFile), strings.ToLower(interfaceName)+"_impl.go")
	result, err := imports.Process(outputPath, buf.Bytes(), &imports.Options{
		Comments:  true,
		TabIndent: true,
		TabWidth:  8,
	})
	if err != nil {
		// Fall back to unformatted output
		result = buf.Bytes()
	}

	return os.WriteFile(outputPath, result, 0o644)
}

// buildDefcFeatures merges auto-detected features (sqlx/callback when hasCallback is true)
// with user-specified extra features into a comma-separated string for the --features flag.
// Returns empty string when no features are needed.
func buildDefcFeatures(hasCallback bool, extra string) string {
	var parts []string
	if hasCallback {
		parts = append(parts, "sqlx/callback")
	}
	if extra != "" {
		parts = append(parts, extra)
	}
	return strings.Join(parts, ",")
}

func containsFeature(features []string, target string) bool {
	for _, feature := range features {
		if strings.TrimSpace(feature) == target {
			return true
		}
	}
	return false
}

// generateBuildConstraint generates a negated build constraint from tags.
// Input: "def" -> Output: "//go:build !def"
// Input: "def,test" -> Output: "//go:build !def && !test"
// Input: "def test" -> Output: "//go:build !def && !test"
func generateBuildConstraint(tags string) string {
	var tagList []string
	for _, tag := range strings.FieldsFunc(tags, func(r rune) bool {
		return r == ',' || r == ' '
	}) {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			tagList = append(tagList, "!"+tag)
		}
	}
	return "//go:build " + strings.Join(tagList, " && ")
}
