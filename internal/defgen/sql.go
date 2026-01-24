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

// GenerateMutationSQL generates a SQL statement for a mutation method (INSERT/UPDATE/DELETE).
func GenerateMutationSQL(pkg *Package, method *MutationMethod) (string, error) {
	switch method.Kind {
	case MethodKindCreate:
		return generateInsertSQL(pkg, method)
	case MethodKindUpdate:
		return generateUpdateSQL(pkg, method)
	case MethodKindDelete:
		return generateDeleteSQL(pkg, method)
	default:
		return "", fmt.Errorf("unknown mutation kind: %v", method.Kind)
	}
}

// generateInsertSQL generates an INSERT SQL statement.
func generateInsertSQL(pkg *Package, method *MutationMethod) (string, error) {
	// Determine the table name
	tableName := ""
	if method.TargetType != "" {
		binding, ok := pkg.Tables[method.TargetType]
		if ok {
			tableName = binding.TableName
		}
	}

	if tableName == "" {
		return "", fmt.Errorf("could not determine table for method %s", method.Name)
	}

	// Entity mode: INSERT INTO table #bind(param)
	if method.EntityParam != nil {
		return fmt.Sprintf("INSERT INTO %s #bind(%s)", tableName, method.EntityParam.Name), nil
	}

	// Field mode: INSERT INTO table (col1, col2) VALUES (${val1}, ${val2})
	if len(method.Sets) == 0 {
		return "", fmt.Errorf("no columns specified for INSERT in method %s", method.Name)
	}

	var columns []string
	var values []string

	for _, set := range method.Sets {
		colName := resolveSetColumnName(pkg, set)
		if colName == "" {
			continue
		}
		columns = append(columns, colName)
		values = append(values, formatSetValue(set.Value))
	}

	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		tableName,
		strings.Join(columns, ", "),
		strings.Join(values, ", ")), nil
}

// generateUpdateSQL generates an UPDATE SQL statement.
func generateUpdateSQL(pkg *Package, method *MutationMethod) (string, error) {
	// Determine the table name
	tableName := ""
	if method.TargetType != "" {
		binding, ok := pkg.Tables[method.TargetType]
		if ok {
			tableName = binding.TableName
		}
	}

	if tableName == "" {
		return "", fmt.Errorf("could not determine table for method %s", method.Name)
	}

	// Build SET clause
	var setClause []string
	for _, set := range method.Sets {
		colName := resolveSetColumnName(pkg, set)
		if colName == "" {
			continue
		}
		setClause = append(setClause, fmt.Sprintf("%s = %s", colName, formatSetValue(set.Value)))
	}

	if len(setClause) == 0 {
		return "", fmt.Errorf("no columns specified for UPDATE in method %s", method.Name)
	}

	sql := fmt.Sprintf("UPDATE %s SET %s", tableName, strings.Join(setClause, ", "))

	// Build WHERE clause
	if len(method.Filters) > 0 {
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

		if len(conditions) > 0 {
			sql += " WHERE " + strings.Join(conditions, " AND ")
		}
	}

	return sql, nil
}

// generateDeleteSQL generates a DELETE SQL statement.
func generateDeleteSQL(pkg *Package, method *MutationMethod) (string, error) {
	// Determine the table name
	tableName := ""
	if method.TargetType != "" {
		binding, ok := pkg.Tables[method.TargetType]
		if ok {
			tableName = binding.TableName
		}
	}

	if tableName == "" {
		return "", fmt.Errorf("could not determine table for method %s", method.Name)
	}

	sql := fmt.Sprintf("DELETE FROM %s", tableName)

	// Build WHERE clause
	if len(method.Filters) > 0 {
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

		if len(conditions) > 0 {
			sql += " WHERE " + strings.Join(conditions, " AND ")
		}
	}

	return sql, nil
}

// resolveSetColumnName resolves a SetExpr's field path to a column name.
func resolveSetColumnName(pkg *Package, set SetExpr) string {
	if len(set.FieldPath) < 2 {
		return ""
	}

	// Get the type of the first element
	firstElem := set.FieldPath[0]
	typeName := getTypeName(firstElem.Type)

	binding, ok := pkg.Tables[typeName]
	if !ok {
		return ""
	}

	// Find the field in the last element
	lastElem := set.FieldPath[len(set.FieldPath)-1]
	for _, field := range binding.Fields {
		if field.GoName == lastElem.FieldName {
			return field.DBName
		}
	}

	return ""
}

// formatSetValue formats a SetValue for SQL.
func formatSetValue(value SetValue) string {
	if value.IsParam {
		return fmt.Sprintf("${%s}", value.ParamName)
	}
	if value.IsLiteral {
		if value.LiteralKind == token.STRING {
			// Remove Go quotes and add SQL quotes
			inner := value.LiteralValue[1 : len(value.LiteralValue)-1]
			return "'" + inner + "'"
		}
		return value.LiteralValue
	}
	return ""
}
