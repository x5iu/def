package defgen

import (
	"fmt"
	"go/token"
	"strings"
)

// GenerateSQL generates a SQL statement for a query method.
func GenerateSQL(pkg *Package, method *QueryMethod) (string, error) {
	// Determine the table from the return type
	binding, err := lookupTableByType(pkg, method.ReturnType.Type)
	if err != nil {
		return "", fmt.Errorf("could not determine table for method %s: %w", method.Name, err)
	}
	tableName := binding.TableName

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

	// Add LIMIT clause
	if method.Limit != nil {
		if method.Limit.IsParam {
			sql += fmt.Sprintf(" LIMIT ${%s}", method.Limit.ParamName)
		} else {
			sql += fmt.Sprintf(" LIMIT %d", method.Limit.Value)
		}
	}

	// Add OFFSET clause
	if method.Offset != nil {
		if method.Offset.IsParam {
			sql += fmt.Sprintf(" OFFSET ${%s}", method.Offset.ParamName)
		} else {
			sql += fmt.Sprintf(" OFFSET %d", method.Offset.Value)
		}
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

	binding, err := lookupTableByType(pkg, fieldPath[0].Type)
	if err != nil {
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
		if filter.IsNull {
			if filter.IsSubquery {
				return fmt.Sprintf("%s IN (SELECT %s FROM %s WHERE %s %s)",
					filter.ForeignKeyCol,
					filter.SubqueryIDField,
					filter.SubqueryTable,
					filter.SubqueryColumn,
					filter.Operator,
				)
			}
			var leftSide string
			if filter.LeftIsFunc {
				leftSide = filter.LeftFuncExpr
			} else {
				leftSide = filter.ColumnName
			}
			return fmt.Sprintf("%s %s", leftSide, filter.Operator)
		}
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
	binding, err := lookupTableByTargetType(pkg, method.TargetType)
	if err != nil {
		return "", fmt.Errorf("could not determine table for method %s: %w", method.Name, err)
	}
	tableName := binding.TableName

	var sql string

	// Entity mode: INSERT INTO table (col1, col2) VALUES (${param.Field1}, ${param.Field2})
	if method.EntityParam != nil {
		var columns []string
		var values []string
		for _, field := range binding.Fields {
			columns = append(columns, field.DBName)
			values = append(values, fmt.Sprintf("${%s.%s}", method.EntityParam.Name, field.GoName))
		}

		sql = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			tableName,
			strings.Join(columns, ", "),
			strings.Join(values, ", "))
	} else {
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

		sql = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			tableName,
			strings.Join(columns, ", "),
			strings.Join(values, ", "))
	}

	// Append RETURNING clause if needed
	sql += generateReturningClause(pkg, method)

	return sql, nil
}

// generateUpdateSQL generates an UPDATE SQL statement.
func generateUpdateSQL(pkg *Package, method *MutationMethod) (string, error) {
	// Determine the table name
	binding, err := lookupTableByTargetType(pkg, method.TargetType)
	if err != nil {
		return "", fmt.Errorf("could not determine table for method %s: %w", method.Name, err)
	}
	if len(method.Filters) == 0 {
		return "", fmt.Errorf("def.Update requires at least one Filter expression in method %s", method.Name)
	}
	tableName := binding.TableName

	var sql string

	// Entity mode: UPDATE table SET col1 = ${param.Field1}, col2 = ${param.Field2} WHERE ...
	if method.EntityParam != nil {
		// Build SET clause from all fields (skip primary key)
		var setClause []string
		for _, field := range binding.Fields {
			if field.IsPrimaryKey {
				continue
			}
			setClause = append(setClause, fmt.Sprintf("%s = ${%s.%s}",
				field.DBName, method.EntityParam.Name, field.GoName))
		}

		if len(setClause) == 0 {
			return "", fmt.Errorf("no columns to update for method %s", method.Name)
		}

		sql = fmt.Sprintf("UPDATE %s SET %s", tableName, strings.Join(setClause, ", "))

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
	} else {
		// Field mode: UPDATE table SET col1 = val1, col2 = val2 WHERE ...
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

		sql = fmt.Sprintf("UPDATE %s SET %s", tableName, strings.Join(setClause, ", "))

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
	}

	// Append RETURNING clause if needed
	sql += generateReturningClause(pkg, method)

	return sql, nil
}

// generateDeleteSQL generates a DELETE SQL statement.
func generateDeleteSQL(pkg *Package, method *MutationMethod) (string, error) {
	// Determine the table name
	binding, err := lookupTableByTargetType(pkg, method.TargetType)
	if err != nil {
		return "", fmt.Errorf("could not determine table for method %s: %w", method.Name, err)
	}
	tableName := binding.TableName

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

	// Append RETURNING clause if needed
	sql += generateReturningClause(pkg, method)

	return sql, nil
}

// resolveSetColumnName resolves a SetExpr's field path to a column name.
func resolveSetColumnName(pkg *Package, set SetExpr) string {
	if len(set.FieldPath) < 2 {
		return ""
	}

	binding, err := lookupTableByType(pkg, set.FieldPath[0].Type)
	if err != nil {
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

// generateReturningClause generates the RETURNING clause for PostgreSQL.
// Returns empty string if no RETURNING is needed.
func generateReturningClause(pkg *Package, method *MutationMethod) string {
	// No ReturnType means return sql.Result, so no RETURNING
	if method.ReturnType == nil {
		return ""
	}

	// If explicit columns are specified, use them
	if len(method.ReturningCols) > 0 {
		return " RETURNING " + buildSelectClause(pkg, method.ReturningCols)
	}

	// Otherwise, return all columns (RETURNING *)
	return " RETURNING *"
}

// FormatSQL formats a SQL statement with proper line breaks and indentation.
// Rules:
// - SELECT clause on its own line
// - FROM clause on its own line
// - WHERE clause on its own line
// - AND/OR at the start of new lines with indentation
// - Subqueries stay on the same line
// - INSERT/UPDATE/DELETE are formatted similarly
func FormatSQL(sql string) string {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return ""
	}

	var result strings.Builder

	switch {
	case strings.HasPrefix(sql, "SELECT "):
		result.WriteString(formatSelectSQL(sql))
	case strings.HasPrefix(sql, "INSERT INTO "):
		result.WriteString(formatInsertSQL(sql))
	case strings.HasPrefix(sql, "UPDATE "):
		result.WriteString(formatUpdateSQL(sql))
	case strings.HasPrefix(sql, "DELETE FROM "):
		result.WriteString(formatDeleteSQL(sql))
	default:
		result.WriteString(sql)
	}

	return result.String()
}

// formatSelectSQL formats a SELECT SQL statement.
func formatSelectSQL(sql string) string {
	var result strings.Builder

	// Find FROM position (not inside subquery)
	fromPos := findKeywordOutsideSubquery(sql, " FROM ")
	if fromPos == -1 {
		return sql
	}

	// SELECT clause
	result.WriteString(strings.TrimSpace(sql[:fromPos]))
	result.WriteString("\n")

	// FROM clause
	remaining := sql[fromPos+1:] // skip the leading space
	wherePos := findKeywordOutsideSubquery(remaining, " WHERE ")

	if wherePos == -1 {
		// No WHERE clause, check for LIMIT/OFFSET
		limitPos := findKeywordOutsideSubquery(remaining, " LIMIT ")
		if limitPos == -1 {
			result.WriteString(strings.TrimSpace(remaining))
			return result.String()
		}
		// FROM ... part
		result.WriteString(strings.TrimSpace(remaining[:limitPos]))
		result.WriteString("\n")
		// LIMIT/OFFSET clause
		result.WriteString(strings.TrimSpace(remaining[limitPos+1:]))
		return result.String()
	}

	// FROM ... part
	result.WriteString(strings.TrimSpace(remaining[:wherePos]))
	result.WriteString("\n")

	// WHERE clause (may include LIMIT/OFFSET)
	whereClause := strings.TrimSpace(remaining[wherePos+1:]) // skip the leading space

	// Check for LIMIT in where clause
	limitPos := findKeywordOutsideSubquery(whereClause, " LIMIT ")
	if limitPos == -1 {
		result.WriteString(formatWhereClause(whereClause))
		return result.String()
	}

	// WHERE ... part
	result.WriteString(formatWhereClause(strings.TrimSpace(whereClause[:limitPos])))
	result.WriteString("\n")
	// LIMIT/OFFSET clause
	result.WriteString(strings.TrimSpace(whereClause[limitPos+1:]))

	return result.String()
}

// formatInsertSQL formats an INSERT SQL statement with each field on its own line.
func formatInsertSQL(sql string) string {
	var result strings.Builder

	// Check for VALUES
	valuesPos := strings.Index(sql, " VALUES ")
	if valuesPos == -1 {
		return sql
	}

	insertPart := sql[:valuesPos]   // "INSERT INTO table (col1, col2, col3)"
	valuesPart := sql[valuesPos+8:] // "(val1, val2, val3)" or "(val1, val2, val3) RETURNING *"

	// Extract RETURNING clause if present
	var returningClause string
	returningPos := strings.Index(valuesPart, " RETURNING ")
	if returningPos != -1 {
		returningClause = valuesPart[returningPos:] // " RETURNING *" or " RETURNING id, name"
		valuesPart = valuesPart[:returningPos]      // "(val1, val2, val3)"
	}

	// Parse columns from insertPart
	openParen := strings.Index(insertPart, "(")
	if openParen == -1 {
		return sql
	}

	tablePart := strings.TrimSpace(insertPart[:openParen])     // "INSERT INTO table"
	columnsPart := insertPart[openParen+1 : len(insertPart)-1] // "col1, col2, col3"
	columns := strings.Split(columnsPart, ", ")

	// Parse values
	valuesPart = strings.TrimSpace(valuesPart)
	valuesPart = valuesPart[1 : len(valuesPart)-1] // Remove parentheses
	values := splitValues(valuesPart)              // Handle nested ${...}

	// Build formatted output
	result.WriteString(tablePart)
	result.WriteString(" (\n")
	for i, col := range columns {
		result.WriteString("    ")
		result.WriteString(strings.TrimSpace(col))
		if i < len(columns)-1 {
			result.WriteString(",")
		}
		result.WriteString("\n")
	}
	result.WriteString(") VALUES (\n")
	for i, val := range values {
		result.WriteString("    ")
		result.WriteString(strings.TrimSpace(val))
		if i < len(values)-1 {
			result.WriteString(",")
		}
		result.WriteString("\n")
	}
	result.WriteString(")")

	// Append RETURNING clause if present
	if returningClause != "" {
		result.WriteString("\n")
		result.WriteString(strings.TrimSpace(returningClause))
	}

	return result.String()
}

// splitValues splits VALUES content, handling nested ${...} expressions.
func splitValues(s string) []string {
	var values []string
	var current strings.Builder
	depth := 0

	for _, c := range s {
		if c == '{' {
			depth++
			current.WriteRune(c)
		} else if c == '}' {
			depth--
			current.WriteRune(c)
		} else if c == ',' && depth == 0 {
			values = append(values, current.String())
			current.Reset()
		} else {
			current.WriteRune(c)
		}
	}
	if current.Len() > 0 {
		values = append(values, current.String())
	}
	return values
}

// formatUpdateSQL formats an UPDATE SQL statement.
func formatUpdateSQL(sql string) string {
	var result strings.Builder

	// Extract RETURNING clause if present
	var returningClause string
	returningPos := strings.Index(sql, " RETURNING ")
	if returningPos != -1 {
		returningClause = sql[returningPos:]
		sql = sql[:returningPos]
	}

	// Find SET position
	setPos := strings.Index(sql, " SET ")
	if setPos == -1 {
		return sql + returningClause
	}

	// UPDATE table
	result.WriteString(strings.TrimSpace(sql[:setPos]))
	result.WriteString("\n")

	remaining := sql[setPos+1:] // skip the leading space

	// Find WHERE position
	wherePos := findKeywordOutsideSubquery(remaining, " WHERE ")
	if wherePos == -1 {
		// No WHERE clause
		result.WriteString(strings.TrimSpace(remaining))
	} else {
		// SET ... part
		result.WriteString(strings.TrimSpace(remaining[:wherePos]))
		result.WriteString("\n")

		// WHERE clause
		whereClause := strings.TrimSpace(remaining[wherePos+1:])
		result.WriteString(formatWhereClause(whereClause))
	}

	// Append RETURNING clause if present
	if returningClause != "" {
		result.WriteString("\n")
		result.WriteString(strings.TrimSpace(returningClause))
	}

	return result.String()
}

// formatDeleteSQL formats a DELETE SQL statement.
func formatDeleteSQL(sql string) string {
	var result strings.Builder

	// Extract RETURNING clause if present
	var returningClause string
	returningPos := strings.Index(sql, " RETURNING ")
	if returningPos != -1 {
		returningClause = sql[returningPos:]
		sql = sql[:returningPos]
	}

	// Find WHERE position
	wherePos := findKeywordOutsideSubquery(sql, " WHERE ")
	if wherePos == -1 {
		result.WriteString(strings.TrimSpace(sql))
	} else {
		// DELETE FROM table
		result.WriteString(strings.TrimSpace(sql[:wherePos]))
		result.WriteString("\n")

		// WHERE clause
		whereClause := strings.TrimSpace(sql[wherePos+1:])
		result.WriteString(formatWhereClause(whereClause))
	}

	// Append RETURNING clause if present
	if returningClause != "" {
		result.WriteString("\n")
		result.WriteString(strings.TrimSpace(returningClause))
	}

	return result.String()
}

// formatWhereClause formats a WHERE clause with proper AND/OR line breaks.
func formatWhereClause(where string) string {
	if !strings.HasPrefix(where, "WHERE ") {
		return where
	}

	var result strings.Builder
	result.WriteString("WHERE ")

	// Get the condition part after "WHERE "
	condition := where[6:]

	// Split by AND/OR at the top level (not inside subqueries or parentheses)
	parts := splitConditions(condition)

	for i, part := range parts {
		if i == 0 {
			result.WriteString(strings.TrimSpace(part.condition))
		} else {
			result.WriteString("\n")
			result.WriteString("  ") // indentation
			result.WriteString(part.connector)
			result.WriteString(" ")
			result.WriteString(strings.TrimSpace(part.condition))
		}
	}

	return result.String()
}

type conditionPart struct {
	connector string // "AND" or "OR"
	condition string
}

// splitConditions splits a WHERE condition by AND/OR at the top level.
func splitConditions(condition string) []conditionPart {
	var parts []conditionPart
	var current strings.Builder
	depth := 0
	inSingleQuote := false
	pendingConnector := ""
	i := 0

	flush := func() {
		cond := strings.TrimSpace(current.String())
		if cond == "" {
			current.Reset()
			return
		}
		parts = append(parts, conditionPart{
			connector: pendingConnector,
			condition: cond,
		})
		current.Reset()
		pendingConnector = ""
	}

	for i < len(condition) {
		c := condition[i]

		if inSingleQuote {
			current.WriteByte(c)
			// SQL single-quote escaping: '' within a string literal.
			if c == '\'' {
				if i+1 < len(condition) && condition[i+1] == '\'' {
					current.WriteByte(condition[i+1])
					i += 2
					continue
				}
				inSingleQuote = false
			}
			i++
			continue
		}

		if c == '\'' {
			inSingleQuote = true
			current.WriteByte(c)
			i++
		} else if c == '(' {
			depth++
			current.WriteByte(c)
			i++
		} else if c == ')' {
			depth--
			current.WriteByte(c)
			i++
		} else if depth == 0 {
			// Check for AND/OR at top level
			remaining := condition[i:]
			if strings.HasPrefix(remaining, " AND ") {
				flush()
				pendingConnector = "AND"
				i += 5 // len(" AND ")
			} else if strings.HasPrefix(remaining, " OR ") {
				flush()
				pendingConnector = "OR"
				i += 4 // len(" OR ")
			} else {
				current.WriteByte(c)
				i++
			}
		} else {
			current.WriteByte(c)
			i++
		}
	}

	flush()

	return parts
}

// findKeywordOutsideSubquery finds a keyword position that is not inside a subquery.
func findKeywordOutsideSubquery(sql, keyword string) int {
	depth := 0
	for i := 0; i < len(sql); i++ {
		if sql[i] == '(' {
			depth++
		} else if sql[i] == ')' {
			depth--
		} else if depth == 0 && strings.HasPrefix(sql[i:], keyword) {
			return i
		}
	}
	return -1
}
