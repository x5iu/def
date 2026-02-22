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

	// Append ON CONFLICT clause if specified
	sql += generateOnConflictClause(pkg, method)

	// Append RETURNING clause if needed
	sql += generateReturningClause(pkg, method)

	return sql, nil
}

// generateOnConflictClause generates the ON CONFLICT clause for PostgreSQL upsert.
// Returns empty string if no ON CONFLICT is specified.
func generateOnConflictClause(pkg *Package, method *MutationMethod) string {
	if len(method.ConflictColumns) == 0 {
		return ""
	}

	cols := buildSelectClause(pkg, method.ConflictColumns)

	switch method.ConflictAction {
	case "nothing":
		return fmt.Sprintf(" ON CONFLICT (%s) DO NOTHING", cols)
	case "update":
		var setClauses []string
		for _, set := range method.ConflictSets {
			colName := resolveSetColumnName(pkg, set)
			if colName == "" {
				continue
			}
			setClauses = append(setClauses, fmt.Sprintf("%s = %s", colName, formatSetValue(set.Value)))
		}
		return fmt.Sprintf(" ON CONFLICT (%s) DO UPDATE SET %s", cols, strings.Join(setClauses, ", "))
	}
	return ""
}

func generateWithClausesSQL(pkg *Package, withClauses []WithClause) (string, error) {
	if len(withClauses) == 0 {
		return "", nil
	}

	parts := make([]string, 0, len(withClauses))
	for _, clause := range withClauses {
		sql, err := generateWithClauseSQL(pkg, clause)
		if err != nil {
			return "", err
		}
		parts = append(parts, sql)
	}
	return "WITH " + strings.Join(parts, ", "), nil
}

func generateWithClauseSQL(pkg *Package, clause WithClause) (string, error) {
	binding, err := lookupTableByTargetType(pkg, clause.TargetType)
	if err != nil {
		return "", fmt.Errorf("could not determine source table for WITH %q: %w", clause.Name, err)
	}

	selectClause := "*"
	if len(clause.Columns) > 0 {
		selectClause = buildSelectClause(pkg, clause.Columns)
	}

	var sql strings.Builder
	sql.WriteString("SELECT ")
	sql.WriteString(selectClause)
	sql.WriteString(" FROM ")
	sql.WriteString(binding.TableName)

	conditions, err := buildFilterConditions(pkg, clause.Filters, nil)
	if err != nil {
		return "", err
	}
	if len(conditions) > 0 {
		sql.WriteString(" WHERE ")
		sql.WriteString(strings.Join(conditions, " AND "))
	}

	if len(clause.OrderBy) > 0 {
		var orderBy []string
		for _, item := range clause.OrderBy {
			part := formatColumnExpr(pkg, item.Column)
			if item.Desc {
				part += " DESC"
			} else {
				part += " ASC"
			}
			orderBy = append(orderBy, part)
		}
		sql.WriteString(" ORDER BY ")
		sql.WriteString(strings.Join(orderBy, ", "))
	}

	if clause.Limit != nil {
		if clause.Limit.IsParam {
			sql.WriteString(fmt.Sprintf(" LIMIT ${%s}", clause.Limit.ParamName))
		} else {
			sql.WriteString(fmt.Sprintf(" LIMIT %d", clause.Limit.Value))
		}
	}
	if clause.Offset != nil {
		if clause.Offset.IsParam {
			sql.WriteString(fmt.Sprintf(" OFFSET ${%s}", clause.Offset.ParamName))
		} else {
			sql.WriteString(fmt.Sprintf(" OFFSET %d", clause.Offset.Value))
		}
	}

	if clause.ForUpdateSkipLocked {
		sql.WriteString(" FOR UPDATE SKIP LOCKED")
	}

	return fmt.Sprintf("%s AS (%s)", clause.Name, sql.String()), nil
}

func buildFilterConditions(pkg *Package, filters []*FilterExpr, qualifier *filterQualifier) ([]string, error) {
	var conditions []string
	for _, filter := range filters {
		analyzed, err := analyzeFilterWithQualifier(pkg, filter, qualifier)
		if err != nil {
			return nil, err
		}
		if analyzed == nil {
			continue
		}

		cond := formatFilterTree(analyzed)
		if cond != "" {
			conditions = append(conditions, cond)
		}
	}
	return conditions, nil
}

func buildUpdateFilterQualifier(tableName string, method *MutationMethod) *filterQualifier {
	if len(method.WithClauses) == 0 && len(method.FromSources) == 0 {
		return nil
	}

	qualifier := &filterQualifier{
		Default: tableName,
		Vars:    make(map[string]string),
	}

	for _, clause := range method.WithClauses {
		if clause.Name == "" {
			continue
		}
		qualifier.Vars[clause.Name] = clause.Name
	}

	for _, source := range method.FromSources {
		qualifierName, varName := parseFromSourceQualifier(source)
		if qualifierName == "" {
			continue
		}
		qualifier.Vars[qualifierName] = qualifierName
		if varName != "" {
			qualifier.Vars[varName] = qualifierName
		}
	}

	return qualifier
}

func parseFromSourceQualifier(source string) (qualifierName, varName string) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", ""
	}

	parts := strings.Fields(source)
	if len(parts) == 0 {
		return "", ""
	}

	qualifierName = parts[0]
	qualifierName = strings.TrimSuffix(qualifierName, ",")

	if len(parts) >= 3 && strings.EqualFold(parts[len(parts)-2], "AS") {
		varName = parts[len(parts)-1]
	} else if len(parts) >= 2 {
		varName = parts[len(parts)-1]
	}

	// No explicit alias: fallback to right-most segment (schema.table -> table)
	if varName == "" {
		varName = qualifierName
		if dot := strings.LastIndex(varName, "."); dot != -1 && dot+1 < len(varName) {
			varName = varName[dot+1:]
		}
	}

	varName = strings.Trim(varName, `"`)
	return qualifierName, varName
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

	var withPrefix string
	if len(method.WithClauses) > 0 {
		withPrefix, err = generateWithClausesSQL(pkg, method.WithClauses)
		if err != nil {
			return "", err
		}
	}

	var setClause []string

	// Entity mode: UPDATE table SET col1 = ${param.Field1}, col2 = ${param.Field2} WHERE ...
	if method.EntityParam != nil {
		// Build SET clause from all fields (skip primary key)
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
	} else {
		// Field mode: UPDATE table SET col1 = val1, col2 = val2 WHERE ...
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
	}

	var sql strings.Builder
	if withPrefix != "" {
		sql.WriteString(withPrefix)
		sql.WriteString(" ")
	}
	sql.WriteString(fmt.Sprintf("UPDATE %s SET %s", tableName, strings.Join(setClause, ", ")))

	if len(method.FromSources) > 0 {
		sql.WriteString(" FROM ")
		sql.WriteString(strings.Join(method.FromSources, ", "))
	}

	qualifier := buildUpdateFilterQualifier(tableName, method)
	conditions, err := buildFilterConditions(pkg, method.Filters, qualifier)
	if err != nil {
		return "", err
	}
	if len(conditions) > 0 {
		sql.WriteString(" WHERE ")
		sql.WriteString(strings.Join(conditions, " AND "))
	}

	// Append RETURNING clause if needed
	sql.WriteString(generateReturningClause(pkg, method))

	return sql.String(), nil
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
	if value.ExprSQL != "" {
		return value.ExprSQL
	}
	if value.IsParam {
		return fmt.Sprintf("${%s}", value.ParamName)
	}
	if value.IsLiteral {
		return formatLiteral(value.LiteralValue, value.LiteralKind)
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
	case strings.HasPrefix(sql, "WITH "):
		result.WriteString(formatWithSQL(sql))
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

// formatWithSQL formats a WITH statement and delegates formatting of inner SELECT
// and the trailing main statement.
func formatWithSQL(sql string) string {
	var result strings.Builder

	// Find main statement after WITH clauses.
	mainPos := -1
	inSingleQuote := false
	depth := 0
	for i := 0; i < len(sql); i++ {
		c := sql[i]
		if inSingleQuote {
			if c == '\'' {
				if i+1 < len(sql) && sql[i+1] == '\'' {
					i++
					continue
				}
				inSingleQuote = false
			}
			continue
		}
		switch c {
		case '\'':
			inSingleQuote = true
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 && strings.HasPrefix(sql[i:], " UPDATE ") {
				mainPos = i + 1 // skip the leading space before UPDATE
			}
			if depth == 0 && strings.HasPrefix(sql[i:], " SELECT ") {
				mainPos = i + 1
			}
			if depth == 0 && strings.HasPrefix(sql[i:], " DELETE ") {
				mainPos = i + 1
			}
			if depth == 0 && strings.HasPrefix(sql[i:], " INSERT ") {
				mainPos = i + 1
			}
		}
		if mainPos != -1 {
			break
		}
	}
	if mainPos == -1 {
		return sql
	}

	withBody := strings.TrimSpace(sql[len("WITH "):mainPos])
	mainSQL := strings.TrimSpace(sql[mainPos:])
	cteClauses := splitTopLevelCSV(withBody)
	if len(cteClauses) == 0 {
		return sql
	}

	result.WriteString("WITH ")
	for i, clause := range cteClauses {
		clause = strings.TrimSpace(clause)
		asPos := findKeywordOutsideSubquery(clause, " AS ")
		if asPos == -1 {
			// Fallback for unexpected shapes.
			result.WriteString(clause)
		} else {
			name := strings.TrimSpace(clause[:asPos])
			queryPart := strings.TrimSpace(clause[asPos+4:])
			if !strings.HasPrefix(queryPart, "(") || !strings.HasSuffix(queryPart, ")") {
				result.WriteString(clause)
			} else {
				inner := strings.TrimSpace(queryPart[1 : len(queryPart)-1])
				result.WriteString(name)
				result.WriteString(" AS (\n")
				innerFormatted := FormatSQL(inner)
				for _, line := range strings.Split(innerFormatted, "\n") {
					result.WriteString("    ")
					result.WriteString(line)
					result.WriteString("\n")
				}
				result.WriteString(")")
			}
		}

		if i < len(cteClauses)-1 {
			result.WriteString(",\n")
		}
	}

	result.WriteString("\n")
	result.WriteString(FormatSQL(mainSQL))
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
	orderPos := findKeywordOutsideSubquery(remaining, " ORDER BY ")
	limitPos := findKeywordOutsideSubquery(remaining, " LIMIT ")

	fromEnd := len(remaining)
	for _, pos := range []int{wherePos, orderPos, limitPos} {
		if pos != -1 && pos < fromEnd {
			fromEnd = pos
		}
	}

	// FROM ... part
	result.WriteString(strings.TrimSpace(remaining[:fromEnd]))

	// WHERE clause
	if wherePos != -1 {
		whereEnd := len(remaining)
		for _, pos := range []int{orderPos, limitPos} {
			if pos != -1 && pos > wherePos && pos < whereEnd {
				whereEnd = pos
			}
		}
		result.WriteString("\n")
		whereClause := strings.TrimSpace(remaining[wherePos+1 : whereEnd]) // skip the leading space
		result.WriteString(formatWhereClause(whereClause))
	}

	// ORDER BY clause
	if orderPos != -1 {
		orderEnd := len(remaining)
		if limitPos != -1 && limitPos > orderPos {
			orderEnd = limitPos
		}
		result.WriteString("\n")
		result.WriteString(strings.TrimSpace(remaining[orderPos+1 : orderEnd])) // skip the leading space
	}

	// LIMIT/OFFSET and trailing lock clauses
	if limitPos != -1 {
		result.WriteString("\n")
		result.WriteString(strings.TrimSpace(remaining[limitPos+1:])) // skip the leading space
	}

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
	valuesPart := sql[valuesPos+8:] // "(val1, val2, val3)" or "(val1, val2, val3) ON CONFLICT ... RETURNING *"

	// Extract ON CONFLICT and RETURNING clauses from valuesPart.
	// Order in raw SQL: VALUES (...) ON CONFLICT (...) DO ... RETURNING ...
	var onConflictClause string
	var returningClause string

	onConflictPos := findKeywordOutsideSubquery(valuesPart, " ON CONFLICT ")
	if onConflictPos != -1 {
		onConflictClause = valuesPart[onConflictPos:]
		valuesPart = valuesPart[:onConflictPos]

		// Extract RETURNING from the ON CONFLICT tail
		returningPos := findKeywordOutsideSubquery(onConflictClause, " RETURNING ")
		if returningPos != -1 {
			returningClause = onConflictClause[returningPos:]
			onConflictClause = onConflictClause[:returningPos]
		}
	} else {
		// No ON CONFLICT — check for RETURNING directly
		returningPos := findKeywordOutsideSubquery(valuesPart, " RETURNING ")
		if returningPos != -1 {
			returningClause = valuesPart[returningPos:]
			valuesPart = valuesPart[:returningPos]
		}
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

	// Append ON CONFLICT clause if present
	if onConflictClause != "" {
		result.WriteString("\n")
		result.WriteString(strings.TrimSpace(onConflictClause))
	}

	// Append RETURNING clause if present
	if returningClause != "" {
		result.WriteString("\n")
		result.WriteString(strings.TrimSpace(returningClause))
	}

	return result.String()
}

// splitTopLevelCSV splits a comma-separated list at top level, keeping commas
// inside parentheses and quoted strings intact.
func splitTopLevelCSV(s string) []string {
	var parts []string
	var current strings.Builder
	depth := 0
	braceDepth := 0
	inSingleQuote := false
	inDoubleQuote := false

	flush := func() {
		part := strings.TrimSpace(current.String())
		if part != "" {
			parts = append(parts, part)
		}
		current.Reset()
	}

	for i := 0; i < len(s); i++ {
		c := s[i]

		if inSingleQuote {
			current.WriteByte(c)
			if c == '\'' {
				if i+1 < len(s) && s[i+1] == '\'' {
					current.WriteByte(s[i+1])
					i++
					continue
				}
				inSingleQuote = false
			}
			continue
		}
		if inDoubleQuote {
			current.WriteByte(c)
			if c == '"' {
				if i+1 < len(s) && s[i+1] == '"' {
					current.WriteByte(s[i+1])
					i++
					continue
				}
				inDoubleQuote = false
			}
			continue
		}

		switch c {
		case '\'':
			inSingleQuote = true
			current.WriteByte(c)
		case '"':
			inDoubleQuote = true
			current.WriteByte(c)
		case '(':
			depth++
			current.WriteByte(c)
		case ')':
			if depth > 0 {
				depth--
			}
			current.WriteByte(c)
		case '{':
			braceDepth++
			current.WriteByte(c)
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
			current.WriteByte(c)
		case ',':
			if depth == 0 && braceDepth == 0 {
				flush()
			} else {
				current.WriteByte(c)
			}
		default:
			current.WriteByte(c)
		}
	}

	flush()
	return parts
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

// splitSetAssignments splits SET assignments by top-level commas.
// It keeps commas inside parentheses and quoted strings intact.
func splitSetAssignments(s string) []string {
	var assignments []string
	var current strings.Builder
	depth := 0
	braceDepth := 0
	inSingleQuote := false
	inDoubleQuote := false

	flush := func() {
		assignment := strings.TrimSpace(current.String())
		if assignment != "" {
			assignments = append(assignments, assignment)
		}
		current.Reset()
	}

	for i := 0; i < len(s); i++ {
		c := s[i]

		if inSingleQuote {
			current.WriteByte(c)
			// SQL single-quote escaping: ''.
			if c == '\'' {
				if i+1 < len(s) && s[i+1] == '\'' {
					current.WriteByte(s[i+1])
					i++
					continue
				}
				inSingleQuote = false
			}
			continue
		}

		if inDoubleQuote {
			current.WriteByte(c)
			// SQL double-quote escaping for identifiers: "".
			if c == '"' {
				if i+1 < len(s) && s[i+1] == '"' {
					current.WriteByte(s[i+1])
					i++
					continue
				}
				inDoubleQuote = false
			}
			continue
		}

		switch c {
		case '\'':
			inSingleQuote = true
			current.WriteByte(c)
		case '"':
			inDoubleQuote = true
			current.WriteByte(c)
		case '(':
			depth++
			current.WriteByte(c)
		case ')':
			if depth > 0 {
				depth--
			}
			current.WriteByte(c)
		case '{':
			braceDepth++
			current.WriteByte(c)
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
			current.WriteByte(c)
		case ',':
			if depth == 0 && braceDepth == 0 {
				flush()
			} else {
				current.WriteByte(c)
			}
		default:
			current.WriteByte(c)
		}
	}

	flush()
	return assignments
}

// formatSetClause formats the SET clause with one assignment per line when
// there are multiple assignments, improving readability for wide UPDATEs.
func formatSetClause(setClause string) string {
	setClause = strings.TrimSpace(setClause)
	if !strings.HasPrefix(setClause, "SET ") {
		return setClause
	}

	assignments := splitSetAssignments(strings.TrimSpace(setClause[4:]))
	if len(assignments) == 0 {
		return "SET"
	}
	if len(assignments) == 1 {
		return "SET " + assignments[0]
	}

	var result strings.Builder
	result.WriteString("SET\n")
	for i, assignment := range assignments {
		result.WriteString("    ")
		result.WriteString(assignment)
		if i < len(assignments)-1 {
			result.WriteString(",")
			result.WriteString("\n")
		}
	}
	return result.String()
}

// formatUpdateSQL formats an UPDATE SQL statement.
func formatUpdateSQL(sql string) string {
	var result strings.Builder

	// Extract RETURNING clause if present
	var returningClause string
	returningPos := findKeywordOutsideSubquery(sql, " RETURNING ")
	if returningPos != -1 {
		returningClause = sql[returningPos:]
		sql = sql[:returningPos]
	}

	// Find SET position
	setPos := findKeywordOutsideSubquery(sql, " SET ")
	if setPos == -1 {
		return sql + returningClause
	}

	// UPDATE table
	result.WriteString(strings.TrimSpace(sql[:setPos]))
	result.WriteString("\n")

	remaining := strings.TrimSpace(sql[setPos+1:]) // skip the leading space

	// Find WHERE position
	wherePos := findKeywordOutsideSubquery(remaining, " WHERE ")
	setAndFromPart := remaining
	var whereClause string
	if wherePos != -1 {
		setAndFromPart = strings.TrimSpace(remaining[:wherePos])
		whereClause = strings.TrimSpace(remaining[wherePos+1:])
	}

	// Find FROM position after SET
	fromPos := findKeywordOutsideSubquery(setAndFromPart, " FROM ")
	setPart := setAndFromPart
	var fromClause string
	if fromPos != -1 {
		setPart = strings.TrimSpace(setAndFromPart[:fromPos])
		fromClause = strings.TrimSpace(setAndFromPart[fromPos+1:]) // "FROM ..."
	}

	result.WriteString(formatSetClause(setPart))

	if fromClause != "" {
		result.WriteString("\n")
		result.WriteString(fromClause)
	}

	if whereClause != "" {
		result.WriteString("\n")
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

// findKeywordOutsideSubquery finds a keyword position that is not inside a
// parenthesized expression or SQL string literal.
func findKeywordOutsideSubquery(sql, keyword string) int {
	depth := 0
	inSingleQuote := false

	for i := 0; i < len(sql); i++ {
		if inSingleQuote {
			if sql[i] == '\'' {
				// SQL single-quote escaping: '' within a string literal.
				if i+1 < len(sql) && sql[i+1] == '\'' {
					i++
				} else {
					inSingleQuote = false
				}
			}
			continue
		}

		if sql[i] == '\'' {
			inSingleQuote = true
		} else if sql[i] == '(' {
			depth++
		} else if sql[i] == ')' {
			depth--
		} else if depth == 0 && strings.HasPrefix(sql[i:], keyword) {
			return i
		}
	}
	return -1
}
