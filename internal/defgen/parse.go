package defgen

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

const defPkgPath = "github.com/x5iu/def"

// Load loads and parses a package.
func Load(wd, pattern string) (*Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedImports,
		Dir: wd,
	}

	// Handle relative path
	if !filepath.IsAbs(pattern) && pattern != "." {
		pattern = "./" + pattern
	}

	pkgs, err := packages.Load(cfg, pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to load package: %w", err)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found matching pattern: %s", pattern)
	}

	if len(pkgs[0].Errors) > 0 {
		var errMsgs []string
		for _, e := range pkgs[0].Errors {
			errMsgs = append(errMsgs, e.Error())
		}
		return nil, fmt.Errorf("package errors: %s", strings.Join(errMsgs, "; "))
	}

	loadedPkg := pkgs[0]
	pkg := &Package{
		Fset:       loadedPkg.Fset,
		PkgPath:    loadedPkg.PkgPath,
		PkgName:    loadedPkg.Name,
		Tables:     make(map[string]*TableBinding),
		Interfaces: make(map[string]*InterfaceInfo),
		TypesInfo:  loadedPkg.TypesInfo,
		Syntax:     loadedPkg.Syntax,
		TypesPkg:   loadedPkg.Types,
	}

	return pkg, nil
}

// Parse parses a loaded package for def-related definitions.
func Parse(pkg *Package) error {
	// Step 1: Parse all struct definitions with their tags
	structs := parseAllStructs(pkg)

	// Step 2: Parse interface definitions
	parseInterfaceDefs(pkg)

	// Step 3: Parse def.Init calls to get table bindings
	if err := parseTableBindings(pkg, structs); err != nil {
		return fmt.Errorf("failed to parse table bindings: %w", err)
	}

	// Step 4: Parse query methods
	if err := parseQueryMethods(pkg, structs); err != nil {
		return fmt.Errorf("failed to parse query methods: %w", err)
	}

	return nil
}

// parseTableBindings finds and parses def.Init calls with BindTable.
func parseTableBindings(pkg *Package, structs map[string]*structInfo) error {
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			// Check if this is def.Init call
			if !isDefCall(pkg, call, "Init") {
				return true
			}

			// Process each argument (should be def.BindTable[T]("table") calls)
			for _, arg := range call.Args {
				argCall, ok := arg.(*ast.CallExpr)
				if !ok {
					continue
				}

				typeName, tableName, ok := parseBindTableCall(pkg, argCall)
				if !ok {
					continue
				}

				// Find the struct info
				structInfo, ok := structs[typeName]
				if !ok {
					continue
				}

				// Create table binding
				binding := &TableBinding{
					Type:        structInfo.typ,
					TypeName:    typeName,
					TableName:   tableName,
					Fields:      structInfo.fields,
					ForeignKeys: structInfo.foreignKeys,
				}
				pkg.Tables[typeName] = binding
			}

			return true
		})
	}

	return nil
}

// parseBindTableCall parses a def.BindTable[T]("table") call.
// Returns (typeName, tableName, ok).
func parseBindTableCall(pkg *Package, call *ast.CallExpr) (string, string, bool) {
	// The function should be an IndexExpr for generic: def.BindTable[T]
	indexExpr, ok := call.Fun.(*ast.IndexExpr)
	if !ok {
		return "", "", false
	}

	// Check if it's def.BindTable
	sel, ok := indexExpr.X.(*ast.SelectorExpr)
	if !ok {
		return "", "", false
	}

	if sel.Sel.Name != "BindTable" {
		return "", "", false
	}

	// Check if it's from the def package
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", "", false
	}

	obj := pkg.TypesInfo.Uses[ident]
	if obj == nil {
		return "", "", false
	}

	pkgName, ok := obj.(*types.PkgName)
	if !ok || pkgName.Imported().Path() != defPkgPath {
		return "", "", false
	}

	// Get the type parameter
	typeIdent, ok := indexExpr.Index.(*ast.Ident)
	if !ok {
		return "", "", false
	}
	typeName := typeIdent.Name

	// Get the table name from the argument
	if len(call.Args) != 1 {
		return "", "", false
	}

	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", "", false
	}

	// Remove quotes from string
	tableName := strings.Trim(lit.Value, `"`)

	return typeName, tableName, true
}

// isDefCall checks if a call expression is a call to def.<funcName>.
func isDefCall(pkg *Package, call *ast.CallExpr, funcName string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	if sel.Sel.Name != funcName {
		return false
	}

	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}

	obj := pkg.TypesInfo.Uses[ident]
	if obj == nil {
		return false
	}

	pkgName, ok := obj.(*types.PkgName)
	if !ok {
		return false
	}

	return pkgName.Imported().Path() == defPkgPath
}

// isGenericDefCall checks if a call is a generic def function call like def.Count[int64](x).
// Returns (funcName, ok) where funcName is e.g. "Count".
func isGenericDefCall(pkg *Package, call *ast.CallExpr) (string, bool) {
	// Generic call's Fun is IndexExpr: def.Count[int64]
	indexExpr, ok := call.Fun.(*ast.IndexExpr)
	if !ok {
		return "", false
	}
	// IndexExpr.X is SelectorExpr: def.Count
	sel, ok := indexExpr.X.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	// Check if it's from the def package
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	obj := pkg.TypesInfo.Uses[ident]
	if obj == nil {
		return "", false
	}
	pkgName, ok := obj.(*types.PkgName)
	if !ok || pkgName.Imported().Path() != defPkgPath {
		return "", false
	}
	return sel.Sel.Name, true
}

// parseQueryMethods finds and parses methods containing def.Query calls.
func parseQueryMethods(pkg *Package, structs map[string]*structInfo) error {
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}

			// Must be a method (have a receiver)
			if fn.Recv == nil || len(fn.Recv.List) == 0 {
				continue
			}

			// Find def.Query call in the method body
			queryCall := findQueryCall(pkg, fn)
			if queryCall == nil {
				continue
			}

			// Parse the method
			method, err := parseQueryMethod(pkg, fn, queryCall, structs)
			if err != nil {
				return fmt.Errorf("failed to parse method %s: %w", fn.Name.Name, err)
			}

			pkg.Methods = append(pkg.Methods, method)
		}
	}

	return nil
}

// findQueryCall finds a def.Query call in a function body.
func findQueryCall(pkg *Package, fn *ast.FuncDecl) *ast.CallExpr {
	var queryCall *ast.CallExpr

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if isDefCall(pkg, call, "Query") {
			queryCall = call
			return false
		}

		return true
	})

	return queryCall
}

// parseQueryMethod parses a method with a def.Query call.
func parseQueryMethod(pkg *Package, fn *ast.FuncDecl, queryCall *ast.CallExpr, structs map[string]*structInfo) (*QueryMethod, error) {
	method := &QueryMethod{
		Name: fn.Name.Name,
		Pos:  fn.Pos(),
	}

	// Get receiver type name
	if len(fn.Recv.List) > 0 {
		recvType := fn.Recv.List[0].Type
		method.Receiver = getReceiverTypeName(recvType)
	}

	// Parse parameters (skip context.Context)
	if fn.Type.Params != nil {
		for _, param := range fn.Type.Params.List {
			paramType := pkg.TypesInfo.TypeOf(param.Type)
			// Skip context.Context
			if isContextType(paramType) {
				continue
			}
			for _, name := range param.Names {
				method.Params = append(method.Params, ParamInfo{
					Name: name.Name,
					Type: paramType,
				})
			}
		}
	}

	// Parse return type to determine target table
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		firstResult := fn.Type.Results.List[0]
		resultType := pkg.TypesInfo.TypeOf(firstResult.Type)
		method.ReturnType = analyzeReturnType(resultType)
	}

	// Parse columns and filters from def.Query call
	columns, filters, err := parseQueryArgs(pkg, queryCall, structs, method.Params)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query args: %w", err)
	}
	method.Columns = columns
	method.Filters = filters

	return method, nil
}

// getReceiverTypeName extracts the type name from a receiver expression.
func getReceiverTypeName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.StarExpr:
		return getReceiverTypeName(e.X)
	case *ast.Ident:
		return e.Name
	default:
		return ""
	}
}

// isContextType checks if a type is context.Context.
func isContextType(t types.Type) bool {
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	return named.Obj().Pkg() != nil &&
		named.Obj().Pkg().Path() == "context" &&
		named.Obj().Name() == "Context"
}

// parseQueryArgs parses def.Column and def.Filter calls within a def.Query call.
func parseQueryArgs(pkg *Package, queryCall *ast.CallExpr, structs map[string]*structInfo, params []ParamInfo) ([]ColumnExpr, []*FilterExpr, error) {
	var columns []ColumnExpr
	var filters []*FilterExpr

	for _, arg := range queryCall.Args {
		call, ok := arg.(*ast.CallExpr)
		if !ok {
			continue
		}

		// Check if it's a def.Column call
		if isDefCall(pkg, call, "Column") {
			if len(call.Args) != 1 {
				continue
			}
			col, err := parseColumnExpr(pkg, call.Args[0], structs, params)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse column: %w", err)
			}
			columns = append(columns, col)
			continue
		}

		// Check if it's a def.Filter call
		if isDefCall(pkg, call, "Filter") {
			if len(call.Args) != 1 {
				return nil, nil, fmt.Errorf("def.Filter requires exactly 1 argument")
			}
			filter, err := parseFilterExprRecursive(pkg, call.Args[0], structs, params)
			if err != nil {
				return nil, nil, err
			}
			filters = append(filters, filter)
		}
	}

	return columns, filters, nil
}

// parseColumnExpr parses a column expression inside def.Column().
func parseColumnExpr(pkg *Package, expr ast.Expr, structs map[string]*structInfo, params []ParamInfo) (ColumnExpr, error) {
	col := ColumnExpr{}

	// Check if it's a function call (aggregate or custom function)
	if call, ok := expr.(*ast.CallExpr); ok {
		// Check for built-in aggregate functions: def.Count, def.Sum, def.Avg, def.Max, def.Min
		if isDefCall(pkg, call, "Count") || isDefCall(pkg, call, "Sum") ||
			isDefCall(pkg, call, "Avg") || isDefCall(pkg, call, "Max") || isDefCall(pkg, call, "Min") {
			funcName := getDefFuncName(call)
			if len(call.Args) != 1 {
				return col, fmt.Errorf("%s requires 1 argument", funcName)
			}

			// Parse the field argument
			fieldPath, err := parseFieldPathFromExpr(pkg, call.Args[0], structs)
			if err != nil {
				return col, fmt.Errorf("failed to parse %s argument: %w", funcName, err)
			}

			col.IsFunc = true
			col.FuncName = strings.ToUpper(funcName)
			col.FuncArgs = []FuncArg{{IsField: true, FieldPath: fieldPath}}
			return col, nil
		}

		// Check for def.Func custom function
		if isDefCall(pkg, call, "Func") {
			return parseFuncExpr(pkg, call, structs, params)
		}
	}

	// Otherwise, it's a plain field reference
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		fieldPath, err := parseFieldPath(pkg, sel, structs)
		if err != nil {
			return col, fmt.Errorf("failed to parse field path: %w", err)
		}
		col.IsFunc = false
		col.FieldPath = fieldPath
		return col, nil
	}

	return col, fmt.Errorf("unsupported column expression type: %T", expr)
}

// parseFuncExpr parses a def.Func("name", args...) call.
func parseFuncExpr(pkg *Package, call *ast.CallExpr, structs map[string]*structInfo, params []ParamInfo) (ColumnExpr, error) {
	col := ColumnExpr{IsFunc: true}

	if len(call.Args) < 1 {
		return col, fmt.Errorf("def.Func requires at least 1 argument (function name)")
	}

	// First argument is the function name (string literal)
	nameLit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || nameLit.Kind != token.STRING {
		return col, fmt.Errorf("def.Func first argument must be a string literal")
	}
	col.FuncName = strings.Trim(nameLit.Value, `"`)

	// Parse remaining arguments
	for _, arg := range call.Args[1:] {
		funcArg, err := parseFuncArg(pkg, arg, structs, params)
		if err != nil {
			return col, fmt.Errorf("failed to parse function argument: %w", err)
		}
		col.FuncArgs = append(col.FuncArgs, funcArg)
	}

	return col, nil
}

// parseFuncArg parses a single argument to def.Func.
func parseFuncArg(pkg *Package, expr ast.Expr, structs map[string]*structInfo, params []ParamInfo) (FuncArg, error) {
	arg := FuncArg{}

	switch e := expr.(type) {
	case *ast.SelectorExpr:
		// Field reference: user.Name
		fieldPath, err := parseFieldPath(pkg, e, structs)
		if err != nil {
			return arg, err
		}
		arg.IsField = true
		arg.FieldPath = fieldPath
		return arg, nil

	case *ast.BasicLit:
		// Literal value: "string" or 123
		arg.IsLiteral = true
		arg.Value = e.Value
		arg.Kind = e.Kind
		return arg, nil

	case *ast.Ident:
		// Method parameter
		for _, p := range params {
			if p.Name == e.Name {
				arg.IsParam = true
				arg.Value = e.Name
				return arg, nil
			}
		}
		return arg, fmt.Errorf("unknown identifier: %s", e.Name)

	default:
		return arg, fmt.Errorf("unsupported function argument type: %T", expr)
	}
}

// getDefFuncName extracts the function name from a def.X() call.
func getDefFuncName(call *ast.CallExpr) string {
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		return sel.Sel.Name
	}
	return ""
}

// parseFieldPathFromExpr parses a field path from any expression.
func parseFieldPathFromExpr(pkg *Package, expr ast.Expr, structs map[string]*structInfo) ([]FieldPathElement, error) {
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		return parseFieldPath(pkg, sel, structs)
	}
	return nil, fmt.Errorf("expected selector expression, got %T", expr)
}

// parseFilterExprRecursive recursively parses a filter expression supporting && and ||.
func parseFilterExprRecursive(pkg *Package, expr ast.Expr, structs map[string]*structInfo, params []ParamInfo) (*FilterExpr, error) {
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		switch e.Op {
		case token.LAND: // &&
			left, err := parseFilterExprRecursive(pkg, e.X, structs, params)
			if err != nil {
				return nil, err
			}
			right, err := parseFilterExprRecursive(pkg, e.Y, structs, params)
			if err != nil {
				return nil, err
			}
			return &FilterExpr{
				Kind:     FilterAnd,
				Children: []*FilterExpr{left, right},
				Pos:      e.Pos(),
			}, nil
		case token.LOR: // ||
			left, err := parseFilterExprRecursive(pkg, e.X, structs, params)
			if err != nil {
				return nil, err
			}
			right, err := parseFilterExprRecursive(pkg, e.Y, structs, params)
			if err != nil {
				return nil, err
			}
			return &FilterExpr{
				Kind:     FilterOr,
				Children: []*FilterExpr{left, right},
				Pos:      e.Pos(),
			}, nil
		default: // ==, !=, <, >, <=, >=
			return parseComparisonExpr(pkg, e, structs, params)
		}
	case *ast.ParenExpr:
		return parseFilterExprRecursive(pkg, e.X, structs, params)
	case *ast.CallExpr:
		if isDefCall(pkg, e, "In") {
			return parseInExpr(pkg, e, structs, params)
		}
		return nil, fmt.Errorf("unsupported call expression in filter: %v", e)
	default:
		return nil, fmt.Errorf("unsupported filter expression type: %T", expr)
	}
}

// parseInExpr parses a def.In(field, values) call.
func parseInExpr(pkg *Package, call *ast.CallExpr, structs map[string]*structInfo, params []ParamInfo) (*FilterExpr, error) {
	filter := &FilterExpr{
		Kind: FilterIn,
		Pos:  call.Pos(),
	}

	if len(call.Args) < 2 {
		return filter, fmt.Errorf("def.In requires 2 arguments, got %d", len(call.Args))
	}

	// First argument is the field (e.g., user.ID)
	left, err := parseFilterOperand(pkg, call.Args[0], structs, params, true)
	if err != nil {
		return filter, fmt.Errorf("failed to parse In field: %w", err)
	}
	filter.Left = left

	// Second argument is the values (e.g., ids parameter)
	right, err := parseFilterOperand(pkg, call.Args[1], structs, params, false)
	if err != nil {
		return filter, fmt.Errorf("failed to parse In values: %w", err)
	}
	filter.Right = right

	return filter, nil
}

// parseComparisonExpr parses a binary comparison expression as a filter.
func parseComparisonExpr(pkg *Package, expr *ast.BinaryExpr, structs map[string]*structInfo, params []ParamInfo) (*FilterExpr, error) {
	filter := &FilterExpr{
		Kind: FilterComparison,
		Op:   expr.Op,
		Pos:  expr.Pos(),
	}

	// Parse left side (should be field access)
	left, err := parseFilterOperand(pkg, expr.X, structs, params, true)
	if err != nil {
		return filter, fmt.Errorf("failed to parse left operand: %w", err)
	}
	filter.Left = left

	// Parse right side (parameter or literal)
	right, err := parseFilterOperand(pkg, expr.Y, structs, params, false)
	if err != nil {
		return filter, fmt.Errorf("failed to parse right operand: %w", err)
	}
	filter.Right = right

	return filter, nil
}

// parseFilterOperand parses one side of a filter expression.
func parseFilterOperand(pkg *Package, expr ast.Expr, structs map[string]*structInfo, params []ParamInfo, expectField bool) (FilterOperand, error) {
	operand := FilterOperand{}

	switch e := expr.(type) {
	case *ast.SelectorExpr:
		// Field access: user.ID or project.User.Name
		path, err := parseFieldPath(pkg, e, structs)
		if err != nil {
			return operand, err
		}
		operand.IsField = true
		operand.FieldPath = path
		return operand, nil

	case *ast.Ident:
		// Could be a parameter reference
		for _, p := range params {
			if p.Name == e.Name {
				operand.IsParam = true
				operand.ParamName = e.Name
				return operand, nil
			}
		}
		// Not a parameter, might be something else
		return operand, fmt.Errorf("unknown identifier: %s", e.Name)

	case *ast.BasicLit:
		// Literal value
		operand.IsLiteral = true
		operand.LiteralValue = e.Value
		operand.LiteralKind = e.Kind
		return operand, nil

	case *ast.CallExpr:
		// Check for generic function call: def.Count[int64](x), def.Func[T](name, args...)
		if funcName, ok := isGenericDefCall(pkg, e); ok {
			operand.IsFunc = true

			switch funcName {
			case "Count", "Sum", "Avg", "Max", "Min":
				operand.FuncName = strings.ToUpper(funcName)
				for _, arg := range e.Args {
					funcArg, err := parseFuncArg(pkg, arg, structs, params)
					if err != nil {
						return operand, err
					}
					operand.FuncArgs = append(operand.FuncArgs, funcArg)
				}
			case "Func":
				// First argument is the function name
				if len(e.Args) < 1 {
					return operand, fmt.Errorf("def.Func requires at least 1 argument (function name)")
				}
				nameLit, ok := e.Args[0].(*ast.BasicLit)
				if !ok || nameLit.Kind != token.STRING {
					return operand, fmt.Errorf("def.Func first argument must be a string literal")
				}
				operand.FuncName = strings.Trim(nameLit.Value, `"`)
				for _, arg := range e.Args[1:] {
					funcArg, err := parseFuncArg(pkg, arg, structs, params)
					if err != nil {
						return operand, err
					}
					operand.FuncArgs = append(operand.FuncArgs, funcArg)
				}
			default:
				return operand, fmt.Errorf("unsupported def function in filter: %s", funcName)
			}
			return operand, nil
		}
		return operand, fmt.Errorf("unsupported call expression in filter operand")

	default:
		return operand, fmt.Errorf("unsupported expression type: %T", expr)
	}
}

// parseFieldPath parses a selector expression into a field path.
// For example: project.User.Name -> [project, User, Name]
func parseFieldPath(pkg *Package, sel *ast.SelectorExpr, structs map[string]*structInfo) ([]FieldPathElement, error) {
	var path []FieldPathElement

	// Build path by traversing the selector chain
	current := ast.Expr(sel)
	for {
		switch e := current.(type) {
		case *ast.SelectorExpr:
			elem := FieldPathElement{
				FieldName: e.Sel.Name,
			}
			path = append([]FieldPathElement{elem}, path...)
			current = e.X
		case *ast.Ident:
			// This is the variable at the start of the chain
			elem := FieldPathElement{
				VarName: e.Name,
			}

			// Get the type of this variable
			obj := pkg.TypesInfo.Uses[e]
			if obj != nil {
				elem.Type = obj.Type()
			}

			path = append([]FieldPathElement{elem}, path...)
			goto done
		default:
			return nil, fmt.Errorf("unexpected expression type in field path: %T", e)
		}
	}
done:

	// Now analyze the path to determine foreign keys
	if len(path) >= 2 {
		// Get the type of the first variable
		firstElem := path[0]
		typeName := getTypeName(firstElem.Type)

		structInfo, ok := structs[typeName]
		if ok {
			// Walk through the path and mark foreign keys
			currentStruct := structInfo
			for i := 1; i < len(path); i++ {
				fieldName := path[i].FieldName

				// Check if this field is a foreign key
				fk, isFk := currentStruct.getForeignKeyByName(fieldName)
				if isFk {
					path[i].IsForeignKey = true
					path[i].Type = fk.RefType

					// Get the struct for the referenced type
					refTypeName := getTypeName(fk.RefType)
					refStruct, ok := structs[refTypeName]
					if ok {
						currentStruct = refStruct
					}
				} else {
					// Regular field access
					field, ok := currentStruct.getFieldByName(fieldName)
					if ok {
						path[i].Type = field.Type
					}
				}
			}
		}
	}

	return path, nil
}
