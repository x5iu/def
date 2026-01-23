package defgen

import (
	"fmt"
	"go/token"
	"strings"
)

// GenerateSQL generates a SQL statement for a query method.
func GenerateSQL(pkg *Package, method *QueryMethod) (string, error) {
	// Determine the table from the return type
	tableName := ""
	if method.ReturnType.StructName != "" {
		binding, ok := pkg.Tables[method.ReturnType.StructName]
		if ok {
			tableName = binding.TableName
		}
	}

	if tableName == "" {
		return "", fmt.Errorf("could not determine table for method %s", method.Name)
	}

	// Build SELECT clause
	selectClause := "*"
	if len(method.Columns) > 0 {
		selectClause = buildSelectClause(pkg, method.Columns)
	}

	// Build WHERE clause
	var conditions []string
	for _, filter := range method.Filters {
		analyzed, err := AnalyzeFilter(pkg, filter)
		if err != nil {
			return "", err
		}
		if analyzed == nil {
			continue
		}

		cond := formatFilterTree(analyzed)
		if cond != "" {
			conditions = append(conditions, cond)
		}
	}

	// Build the SQL
	sql := fmt.Sprintf("SELECT %s FROM %s", selectClause, tableName)
	if len(conditions) > 0 {
		sql += " WHERE " + strings.Join(conditions, " AND ")
	}

	return sql, nil
}

// buildSelectClause builds the SELECT clause from column expressions.
func buildSelectClause(pkg *Package, columns []ColumnExpr) string {
	var parts []string
	for _, col := range columns {
		parts = append(parts, formatColumnExpr(pkg, col))
	}
	return strings.Join(parts, ", ")
}

// formatColumnExpr formats a single column expression.
func formatColumnExpr(pkg *Package, col ColumnExpr) string {
	if !col.IsFunc {
		// Plain field reference
		return resolveColumnName(pkg, col.FieldPath)
	}

	// Function call
	var args []string
	for _, arg := range col.FuncArgs {
		args = append(args, formatFuncArg(pkg, arg))
	}
	return fmt.Sprintf("%s(%s)", col.FuncName, strings.Join(args, ", "))
}

// formatFuncArg formats a single function argument.
func formatFuncArg(pkg *Package, arg FuncArg) string {
	switch {
	case arg.IsField:
		return resolveColumnName(pkg, arg.FieldPath)
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

// resolveColumnName resolves a field path to a column name.
func resolveColumnName(pkg *Package, fieldPath []FieldPathElement) string {
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

// formatFilterTree recursively formats an analyzed filter tree as a SQL condition.
func formatFilterTree(filter *AnalyzedFilter) string {
	if filter == nil {
		return ""
	}

	switch filter.Kind {
	case AnalyzedFilterAnd:
		if len(filter.Children) == 0 {
			return ""
		}
		var parts []string
		for _, child := range filter.Children {
			part := formatFilterTree(child)
			if part != "" {
				parts = append(parts, part)
			}
		}
		if len(parts) == 0 {
			return ""
		}
		if len(parts) == 1 {
			return parts[0]
		}
		return "(" + strings.Join(parts, " AND ") + ")"

	case AnalyzedFilterOr:
		if len(filter.Children) == 0 {
			return ""
		}
		var parts []string
		for _, child := range filter.Children {
			part := formatFilterTree(child)
			if part != "" {
				parts = append(parts, part)
			}
		}
		if len(parts) == 0 {
			return ""
		}
		if len(parts) == 1 {
			return parts[0]
		}
		return "(" + strings.Join(parts, " OR ") + ")"

	case AnalyzedFilterIn:
		if filter.IsSubquery {
			value := filter.SubqueryValue
			if value == "" {
				value = filter.Value
			}
			return fmt.Sprintf("%s IN (SELECT %s FROM %s WHERE %s IN (%s))",
				filter.ForeignKeyCol,
				filter.SubqueryIDField,
				filter.SubqueryTable,
				filter.SubqueryColumn,
				value,
			)
		}
		// Format: column IN (${param})
		return fmt.Sprintf("%s IN (%s)", filter.ColumnName, filter.Value)

	case AnalyzedFilterComparison:
		if filter.IsSubquery {
			value := filter.SubqueryValue
			if value == "" && filter.RightIsFunc {
				value = filter.RightFuncExpr
			}
			return fmt.Sprintf("%s IN (SELECT %s FROM %s WHERE %s %s %s)",
				filter.ForeignKeyCol,
				filter.SubqueryIDField,
				filter.SubqueryTable,
				filter.SubqueryColumn,
				filter.Operator,
				value,
			)
		}

		// Build left side
		var leftSide string
		if filter.LeftIsFunc {
			leftSide = filter.LeftFuncExpr
		} else {
			leftSide = filter.ColumnName
		}

		// Build right side
		var rightSide string
		if filter.RightIsFunc {
			rightSide = filter.RightFuncExpr
		} else {
			rightSide = filter.Value
		}

		return fmt.Sprintf("%s %s %s", leftSide, filter.Operator, rightSide)

	default:
		return ""
	}
}
