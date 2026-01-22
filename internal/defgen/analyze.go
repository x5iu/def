package defgen

import (
	"fmt"
	"go/token"
	"strings"
)

// AnalyzedFilterKind represents the type of an analyzed filter node.
type AnalyzedFilterKind int

const (
	AnalyzedFilterComparison AnalyzedFilterKind = iota // Leaf: a = b
	AnalyzedFilterIn                                   // Leaf: a IN (b)
	AnalyzedFilterAnd                                  // Internal: expr AND expr
	AnalyzedFilterOr                                   // Internal: expr OR expr
)

// AnalyzedFilter represents a fully analyzed filter expression tree node.
type AnalyzedFilter struct {
	Kind AnalyzedFilterKind

	// For comparison/IN nodes
	ColumnName string
	Operator   string
	Value      string // Either ${param} or 'literal' or number

	// For foreign key subqueries
	IsSubquery      bool
	ForeignKeyCol   string // e.g., "user_id"
	SubqueryTable   string // e.g., "users"
	SubqueryColumn  string // e.g., "name"
	SubqueryValue   string // e.g., "${username}"
	SubqueryIDField string // e.g., "id" (the primary key of referenced table)

	// For function operands
	LeftIsFunc   bool   // true if left side is a function call
	LeftFuncExpr string // Formatted function expression, e.g., "COUNT(id)"

	RightIsFunc   bool   // true if right side is a function call
	RightFuncExpr string // Formatted function expression

	// For boolean combination nodes (Kind == AnalyzedFilterAnd or AnalyzedFilterOr)
	Children []*AnalyzedFilter
}

// AnalyzeFilter analyzes a filter expression tree and produces an AnalyzedFilter tree.
func AnalyzeFilter(pkg *Package, filter *FilterExpr) (*AnalyzedFilter, error) {
	if filter == nil {
		return nil, nil
	}

	switch filter.Kind {
	case FilterAnd:
		children := make([]*AnalyzedFilter, 0, len(filter.Children))
		for _, child := range filter.Children {
			analyzed, err := AnalyzeFilter(pkg, child)
			if err != nil {
				return nil, err
			}
			if analyzed != nil {
				children = append(children, analyzed)
			}
		}
		return &AnalyzedFilter{
			Kind:     AnalyzedFilterAnd,
			Children: children,
		}, nil

	case FilterOr:
		children := make([]*AnalyzedFilter, 0, len(filter.Children))
		for _, child := range filter.Children {
			analyzed, err := AnalyzeFilter(pkg, child)
			if err != nil {
				return nil, err
			}
			if analyzed != nil {
				children = append(children, analyzed)
			}
		}
		return &AnalyzedFilter{
			Kind:     AnalyzedFilterOr,
			Children: children,
		}, nil

	case FilterIn:
		return analyzeLeafFilter(pkg, filter, true)

	case FilterComparison:
		return analyzeLeafFilter(pkg, filter, false)

	default:
		return nil, nil
	}
}

// analyzeLeafFilter analyzes a comparison or IN filter expression.
func analyzeLeafFilter(pkg *Package, filter *FilterExpr, isIn bool) (*AnalyzedFilter, error) {
	kind := AnalyzedFilterComparison
	if isIn {
		kind = AnalyzedFilterIn
	}

	analyzed := &AnalyzedFilter{
		Kind:     kind,
		Operator: tokenToSQLOp(filter.Op),
	}

	// Analyze the left side
	if filter.Left.IsFunc {
		// Left side is a function call
		analyzed.LeftIsFunc = true
		analyzed.LeftFuncExpr = formatFuncOperand(pkg, filter.Left)
	} else if filter.Left.IsField {
		path := filter.Left.FieldPath
		if len(path) < 2 {
			return nil, nil // Invalid path
		}

		// Check if any element in the path is a foreign key
		fkIndex := -1
		for i := 1; i < len(path); i++ {
			if path[i].IsForeignKey {
				fkIndex = i
				break
			}
		}

		if fkIndex >= 0 && fkIndex < len(path)-1 {
			// This is a foreign key traversal (e.g., project.User.Name)
			analyzed.IsSubquery = true

			// Get the foreign key column
			varTypeName := getTypeName(path[0].Type)
			binding, ok := pkg.Tables[varTypeName]
			if !ok {
				return nil, nil
			}

			// Find the foreign key info
			for _, fk := range binding.ForeignKeys {
				if fk.FieldName == path[fkIndex].FieldName {
					analyzed.ForeignKeyCol = fk.KeyColumn

					// Get the referenced type's table
					refTypeName := getTypeName(fk.RefType)
					refBinding, ok := pkg.Tables[refTypeName]
					if ok {
						analyzed.SubqueryTable = refBinding.TableName
						analyzed.SubqueryIDField = "id" // Assume primary key is "id"

						// Get the field column name from the last element
						if fkIndex+1 < len(path) {
							lastFieldName := path[len(path)-1].FieldName
							for _, f := range refBinding.Fields {
								if f.GoName == lastFieldName {
									analyzed.SubqueryColumn = f.DBName
									break
								}
							}
						}
					}
					break
				}
			}
		} else {
			// Simple field access (e.g., user.ID)
			// The last element is the field
			lastField := path[len(path)-1]

			// Find the column name
			varTypeName := getTypeName(path[0].Type)
			binding, ok := pkg.Tables[varTypeName]
			if !ok {
				return nil, nil
			}

			for _, f := range binding.Fields {
				if f.GoName == lastField.FieldName {
					analyzed.ColumnName = f.DBName
					break
				}
			}
		}
	}

	// Analyze the right side
	if filter.Right.IsFunc {
		// Right side is a function call
		analyzed.RightIsFunc = true
		analyzed.RightFuncExpr = formatFuncOperand(pkg, filter.Right)
	} else if filter.Right.IsParam {
		value := "${" + filter.Right.ParamName + "}"
		if analyzed.IsSubquery {
			analyzed.SubqueryValue = value
		} else {
			analyzed.Value = value
		}
	} else if filter.Right.IsLiteral {
		value := formatLiteral(filter.Right.LiteralValue, filter.Right.LiteralKind)
		if analyzed.IsSubquery {
			analyzed.SubqueryValue = value
		} else {
			analyzed.Value = value
		}
	}

	return analyzed, nil
}

// formatFuncOperand formats a function operand for SQL.
func formatFuncOperand(pkg *Package, op FilterOperand) string {
	var args []string
	for _, arg := range op.FuncArgs {
		args = append(args, formatFilterFuncArg(pkg, arg))
	}
	return fmt.Sprintf("%s(%s)", op.FuncName, strings.Join(args, ", "))
}

// formatFilterFuncArg formats a single function argument for SQL in filter context.
func formatFilterFuncArg(pkg *Package, arg FuncArg) string {
	switch {
	case arg.IsField:
		return resolveColumnNameFromPath(pkg, arg.FieldPath)
	case arg.IsParam:
		return fmt.Sprintf("${%s}", arg.Value)
	case arg.IsLiteral:
		if arg.Kind == token.STRING {
			// Remove the surrounding quotes and re-wrap with single quotes
			value := strings.Trim(arg.Value, `"`)
			return fmt.Sprintf("'%s'", value)
		}
		return arg.Value
	}
	return ""
}

// resolveColumnNameFromPath resolves a field path to a column name.
func resolveColumnNameFromPath(pkg *Package, fieldPath []FieldPathElement) string {
	if len(fieldPath) < 2 {
		return ""
	}

	// Get the type of the first element
	firstElem := fieldPath[0]
	typeName := getTypeName(firstElem.Type)

	binding, ok := pkg.Tables[typeName]
	if !ok {
		return ""
	}

	// Find the field in the last element
	lastElem := fieldPath[len(fieldPath)-1]
	for _, field := range binding.Fields {
		if field.GoName == lastElem.FieldName {
			return field.DBName
		}
	}

	return ""
}

// tokenToSQLOp converts a Go token to SQL operator.
func tokenToSQLOp(tok token.Token) string {
	switch tok {
	case token.EQL:
		return "="
	case token.NEQ:
		return "!="
	case token.LSS:
		return "<"
	case token.GTR:
		return ">"
	case token.LEQ:
		return "<="
	case token.GEQ:
		return ">="
	default:
		return "="
	}
}

// formatLiteral formats a literal value for SQL.
func formatLiteral(value string, kind token.Token) string {
	switch kind {
	case token.STRING:
		// Remove Go quotes and add SQL quotes
		// Value is like "active" -> 'active'
		inner := value[1 : len(value)-1] // Remove quotes
		return "'" + inner + "'"
	case token.INT, token.FLOAT:
		return value
	default:
		return value
	}
}
