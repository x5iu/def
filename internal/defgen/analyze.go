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
	Value      string // Either ${param} or 'literal' or number or column reference

	// For foreign key subqueries
	IsSubquery      bool
	ForeignKeyCol   string // e.g., "user_id"
	SubqueryTable   string // e.g., "users"
	SubqueryColumn  string // e.g., "name"
	SubqueryValue   string // e.g., "${username}" or column reference
	SubqueryIDField string // e.g., "id" (the primary key of referenced table)

	// For IS NULL / IS NOT NULL checks
	IsNull bool // true if this is an IS NULL or IS NOT NULL check

	// For function operands
	LeftIsFunc   bool   // true if left side is a function call
	LeftFuncExpr string // Formatted function expression, e.g., "COUNT(id)"

	RightIsFunc   bool   // true if right side is a function call
	RightFuncExpr string // Formatted function expression

	// For boolean combination nodes (Kind == AnalyzedFilterAnd or AnalyzedFilterOr)
	Children []*AnalyzedFilter
}

type filterQualifier struct {
	Default string            // default qualifier for unresolved vars
	Vars    map[string]string // explicit var -> qualifier mapping
}

// AnalyzeFilter analyzes a filter expression tree and produces an AnalyzedFilter tree.
func AnalyzeFilter(pkg *Package, filter *FilterExpr) (*AnalyzedFilter, error) {
	return analyzeFilterWithQualifier(pkg, filter, nil)
}

func analyzeFilterWithQualifier(pkg *Package, filter *FilterExpr, qualifier *filterQualifier) (*AnalyzedFilter, error) {
	if filter == nil {
		return nil, nil
	}

	switch filter.Kind {
	case FilterAnd:
		children := make([]*AnalyzedFilter, 0, len(filter.Children))
		for _, child := range filter.Children {
			analyzed, err := analyzeFilterWithQualifier(pkg, child, qualifier)
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
			analyzed, err := analyzeFilterWithQualifier(pkg, child, qualifier)
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
		return analyzeLeafFilter(pkg, filter, true, qualifier)

	case FilterComparison:
		return analyzeLeafFilter(pkg, filter, false, qualifier)

	default:
		return nil, fmt.Errorf("unsupported filter kind: %v", filter.Kind)
	}
}

// analyzeLeafFilter analyzes a comparison or IN filter expression.
func analyzeLeafFilter(pkg *Package, filter *FilterExpr, isIn bool, qualifier *filterQualifier) (*AnalyzedFilter, error) {
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
		analyzed.LeftFuncExpr = formatFuncOperandWithQualifier(pkg, filter.Left, qualifier)
	} else if filter.Left.IsField {
		path := filter.Left.FieldPath
		if len(path) < 2 {
			return nil, fmt.Errorf("invalid field path in filter: expected at least 2 elements, got %d", len(path))
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
			binding, err := lookupTableByType(pkg, path[0].Type)
			if err != nil {
				return nil, err
			}

			// Find the foreign key info
			fkMatched := false
			for _, fk := range binding.ForeignKeys {
				if fk.FieldName == path[fkIndex].FieldName {
					fkMatched = true
					fkCol := fk.KeyColumn
					if q := qualifierForVar(qualifier, path[0].VarName); q != "" {
						fkCol = q + "." + fkCol
					}
					analyzed.ForeignKeyCol = fkCol

					// Get the referenced type's table
					refBinding, err := lookupTableByType(pkg, fk.RefType)
					if err != nil {
						return nil, err
					}
					if refBinding.PrimaryKey == nil {
						return nil, fmt.Errorf("referenced table %s has no primary key", refBinding.TableName)
					}

					analyzed.SubqueryTable = refBinding.TableName
					analyzed.SubqueryIDField = refBinding.PrimaryKey.DBName

					// Get the field column name from the last element
					lastFieldName := path[len(path)-1].FieldName
					for _, f := range refBinding.Fields {
						if f.GoName == lastFieldName {
							analyzed.SubqueryColumn = f.DBName
							break
						}
					}
					if analyzed.SubqueryColumn == "" {
						return nil, fmt.Errorf("field %s not found in referenced table %s", lastFieldName, refBinding.TableName)
					}
					break
				}
			}
			if !fkMatched {
				return nil, fmt.Errorf("foreign key field %s not found on table %s", path[fkIndex].FieldName, binding.TableName)
			}
		} else {
			// Simple field access (e.g., user.ID)
			analyzed.ColumnName = resolveColumnNameFromPathWithQualifier(pkg, path, qualifier)
			if analyzed.ColumnName == "" {
				return nil, fmt.Errorf("failed to resolve field path in filter")
			}
		}
	}

	// Analyze the right side
	if filter.Right.IsFunc {
		// Right side is a function call
		analyzed.RightIsFunc = true
		analyzed.RightFuncExpr = formatFuncOperandWithQualifier(pkg, filter.Right, qualifier)
	} else if filter.Right.IsField {
		value := resolveColumnNameFromPathWithQualifier(pkg, filter.Right.FieldPath, qualifier)
		if value == "" {
			return nil, fmt.Errorf("failed to resolve right-hand field in filter")
		}
		if analyzed.IsSubquery {
			analyzed.SubqueryValue = value
		} else {
			analyzed.Value = value
		}
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
	} else if filter.Right.IsNil {
		analyzed.IsNull = true
		switch filter.Op {
		case token.EQL:
			analyzed.Operator = "IS NULL"
		case token.NEQ:
			analyzed.Operator = "IS NOT NULL"
		default:
			return nil, fmt.Errorf("nil comparison only supports == and !=")
		}
	}

	if analyzed.IsSubquery {
		if analyzed.ForeignKeyCol == "" || analyzed.SubqueryTable == "" || analyzed.SubqueryIDField == "" || analyzed.SubqueryColumn == "" {
			return nil, fmt.Errorf("incomplete subquery analysis for filter")
		}
		if !analyzed.IsNull && analyzed.SubqueryValue == "" && !analyzed.RightIsFunc {
			return nil, fmt.Errorf("missing subquery comparison value")
		}
	} else {
		if !analyzed.LeftIsFunc && analyzed.ColumnName == "" {
			return nil, fmt.Errorf("missing column name in filter")
		}
		if !analyzed.IsNull && !analyzed.RightIsFunc && analyzed.Value == "" {
			return nil, fmt.Errorf("missing right-hand value in filter")
		}
	}

	return analyzed, nil
}

// formatFuncOperand formats a function operand for SQL.
func formatFuncOperand(pkg *Package, op FilterOperand) string {
	return formatFuncOperandWithQualifier(pkg, op, nil)
}

func formatFuncOperandWithQualifier(pkg *Package, op FilterOperand, qualifier *filterQualifier) string {
	var args []string
	for _, arg := range op.FuncArgs {
		args = append(args, formatFilterFuncArgWithQualifier(pkg, arg, qualifier))
	}
	return fmt.Sprintf("%s(%s)", op.FuncName, strings.Join(args, ", "))
}

// formatFilterFuncArg formats a single function argument for SQL in filter context.
func formatFilterFuncArg(pkg *Package, arg FuncArg) string {
	return formatFilterFuncArgWithQualifier(pkg, arg, nil)
}

func formatFilterFuncArgWithQualifier(pkg *Package, arg FuncArg, qualifier *filterQualifier) string {
	switch {
	case arg.IsField:
		return resolveColumnNameFromPathWithQualifier(pkg, arg.FieldPath, qualifier)
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
	return resolveColumnNameFromPathWithQualifier(pkg, fieldPath, nil)
}

func resolveColumnNameFromPathWithQualifier(pkg *Package, fieldPath []FieldPathElement, qualifier *filterQualifier) string {
	if len(fieldPath) < 2 {
		return ""
	}

	binding, err := lookupTableByType(pkg, fieldPath[0].Type)
	if err != nil {
		return ""
	}

	// Find the field in the last element
	lastElem := fieldPath[len(fieldPath)-1]
	for _, field := range binding.Fields {
		if field.GoName == lastElem.FieldName {
			col := field.DBName
			if q := qualifierForVar(qualifier, fieldPath[0].VarName); q != "" {
				return q + "." + col
			}
			return col
		}
	}

	return ""
}

func qualifierForVar(qualifier *filterQualifier, varName string) string {
	if qualifier == nil {
		return ""
	}
	if qualifier.Vars != nil {
		if q, ok := qualifier.Vars[varName]; ok {
			return q
		}
	}
	return qualifier.Default
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
