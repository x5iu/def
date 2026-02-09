package defgen

import (
	"go/types"
	"strings"
	"unicode"
)

// AnalyzeRelations analyzes foreign key relationships and generates:
// 1. Private query methods for loading related data
// 2. Callback methods for automatic relation loading
// 3. Slice type aliases for Callback support
func AnalyzeRelations(pkg *Package) error {
	seen := make(map[string]bool)
	callbackMap := make(map[string]*CallbackMethod)

	// First pass: analyze foreign keys in each table
	for _, table := range pkg.Tables {
		for _, fk := range table.ForeignKeys {
			// 1. Belongs-to (many-to-one): Project.User -> getUserByID
			belongsTo := analyzeBelongsTo(pkg, table, fk)
			if belongsTo != nil && !seen[belongsTo.MethodName] {
				seen[belongsTo.MethodName] = true
				pkg.RelationMethods = append(pkg.RelationMethods, belongsTo)
			}

			// 2. Has-many (one-to-many): Project.User foreign_key:user_id -> User needs getProjectsByUserID
			hasMany := analyzeHasMany(pkg, table, fk)
			if hasMany != nil && !seen[hasMany.MethodName] {
				seen[hasMany.MethodName] = true
				pkg.RelationMethods = append(pkg.RelationMethods, hasMany)

				// Create slice type alias for the has-many relation
				aliasName := hasMany.RefTypeName + "s"
				pkg.SliceTypeAliases = append(pkg.SliceTypeAliases, &SliceTypeAlias{
					AliasName: aliasName,
					ElemType:  "*" + hasMany.RefTypeName,
				})
			}

			// 3. Collect callback fields for the current table (belongs-to)
			addCallbackField(callbackMap, table, fk, belongsTo)
		}
	}

	// Second pass: add has-many callback fields to referenced tables
	for _, table := range pkg.Tables {
		for _, fk := range table.ForeignKeys {
			addHasManyCallbackField(pkg, callbackMap, table, fk)
		}
	}

	// Convert callbackMap to slice
	for _, cb := range callbackMap {
		if len(cb.Fields) > 0 {
			pkg.CallbackMethods = append(pkg.CallbackMethods, cb)
		}
	}

	return nil
}

// analyzeBelongsTo analyzes a belongs-to (many-to-one) relationship.
// e.g., Project.User foreign_key:"user_id" -> getUserByID
func analyzeBelongsTo(pkg *Package, _ *TableBinding, fk ForeignKeyInfo) *RelationMethod {
	// Get the referenced type name
	refTypeName := getTypeName(fk.RefType)
	if refTypeName == "" {
		return nil
	}

	// Find the referenced table
	refTable, ok := pkg.Tables[refTypeName]
	if !ok {
		return nil
	}

	// Find the primary key field from primary_key tag
	pkField := refTable.PrimaryKey
	if pkField == nil {
		return nil
	}

	// Generate method name: getUserByID
	methodName := "get" + refTypeName + "By" + pkField.GoName

	return &RelationMethod{
		MethodName:   methodName,
		ParamName:    lcFirst(pkField.GoName),
		ParamType:    pkField.Type,
		RefType:      fk.RefType,
		RefTypeName:  refTypeName,
		RefTableName: refTable.TableName,
		WhereColumn:  pkField.DBName,
		IsSlice:      false,
	}
}

// analyzeHasMany analyzes a has-many (one-to-many) relationship by reverse inference.
// e.g., Project.User foreign_key:"user_id" -> User needs getProjectsByUserID
func analyzeHasMany(pkg *Package, sourceTable *TableBinding, fk ForeignKeyInfo) *RelationMethod {
	// Get the referenced type name (e.g., User from Project.User)
	refTypeName := getTypeName(fk.RefType)
	if refTypeName == "" {
		return nil
	}

	// Find the referenced table (User)
	_, ok := pkg.Tables[refTypeName]
	if !ok {
		return nil
	}

	// Find the foreign key field type in the source table
	var fkFieldType types.Type
	for _, field := range sourceTable.Fields {
		if field.DBName == fk.KeyColumn {
			fkFieldType = field.Type
			break
		}
	}
	if fkFieldType == nil {
		return nil
	}

	// Generate method name: getProjectsByUserID
	// Convert user_id to UserID for the param name
	paramName := snakeToCamel(fk.KeyColumn)

	methodName := "get" + sourceTable.TypeName + "sBy" + ucFirst(paramName)

	return &RelationMethod{
		MethodName:   methodName,
		ParamName:    lcFirst(paramName),
		ParamType:    fkFieldType,
		RefType:      types.NewSlice(types.NewPointer(sourceTable.Type)),
		RefTypeName:  sourceTable.TypeName,
		RefTableName: sourceTable.TableName,
		WhereColumn:  fk.KeyColumn,
		IsSlice:      true,
	}
}

// addCallbackField adds a belongs-to callback field to the callback map.
func addCallbackField(callbackMap map[string]*CallbackMethod, table *TableBinding, fk ForeignKeyInfo, belongsTo *RelationMethod) {
	if belongsTo == nil {
		return
	}

	cb, ok := callbackMap[table.TypeName]
	if !ok {
		cb = &CallbackMethod{
			StructName:     table.TypeName,
			StructTypeName: "*" + table.TypeName,
			IDField:        table.PrimaryKey,
		}
		callbackMap[table.TypeName] = cb
	}

	// Find the key field name in the source table (e.g., UserID for foreign_key:"user_id")
	keyFieldName := ""
	for _, field := range table.Fields {
		if field.DBName == fk.KeyColumn {
			keyFieldName = field.GoName
			break
		}
	}
	if keyFieldName == "" {
		return
	}

	cb.Fields = append(cb.Fields, CallbackField{
		FieldName:    fk.FieldName,
		MethodName:   belongsTo.MethodName,
		KeyFieldName: keyFieldName,
		IsSlice:      false,
		CacheKey:     fk.KeyColumn,
		RefTypeName:  belongsTo.RefTypeName,
	})
}

// addHasManyCallbackField adds a has-many callback field to the referenced table's callback.
func addHasManyCallbackField(pkg *Package, callbackMap map[string]*CallbackMethod, sourceTable *TableBinding, fk ForeignKeyInfo) {
	// Get the referenced type name (e.g., User from Project.User)
	refTypeName := getTypeName(fk.RefType)
	if refTypeName == "" {
		return
	}

	// Find the referenced table
	refTable, ok := pkg.Tables[refTypeName]
	if !ok {
		return
	}

	// Find the plural field name in the referenced table (e.g., Projects in User)
	pluralFieldName := sourceTable.TypeName + "s"

	// Check if this field exists in the refTable and capture its type
	var hasManyFieldType types.Type
	refStruct, ok := refTable.Type.Underlying().(*types.Struct)
	if !ok {
		return
	}
	for i := 0; i < refStruct.NumFields(); i++ {
		field := refStruct.Field(i)
		if field.Name() == pluralFieldName {
			hasManyFieldType = field.Type()
			break
		}
	}
	if hasManyFieldType == nil {
		return
	}

	// Determine whether the has-many field uses the generated alias type (e.g., Projects) or a raw slice (e.g., []*Project).
	fieldIsAlias := false
	sliceType := ""
	switch t := hasManyFieldType.(type) {
	case *types.Named:
		fieldIsAlias = t.Obj().Name() == pluralFieldName
		sliceType = formatType(pkg, t.Underlying())
	case *types.Slice:
		fieldIsAlias = false
		sliceType = formatType(pkg, t)
	default:
		sliceType = formatType(pkg, hasManyFieldType)
	}

	// Get or create the callback for the referenced table
	cb, ok := callbackMap[refTypeName]
	if !ok {
		cb = &CallbackMethod{
			StructName:     refTypeName,
			StructTypeName: "*" + refTypeName,
			IDField:        refTable.PrimaryKey,
		}
		callbackMap[refTypeName] = cb
	}

	// Get the primary key field name from the referenced table
	if refTable.PrimaryKey == nil {
		return
	}
	idFieldName := refTable.PrimaryKey.GoName

	// Generate method name
	paramName := snakeToCamel(fk.KeyColumn)
	methodName := "get" + sourceTable.TypeName + "sBy" + ucFirst(paramName)

	cb.Fields = append(cb.Fields, CallbackField{
		FieldName:     pluralFieldName,
		MethodName:    methodName,
		KeyFieldName:  idFieldName,
		IsSlice:       true,
		CacheKey:      fk.KeyColumn,
		SliceType:     sliceType,
		FieldIsAlias:  fieldIsAlias,
	})
}

// lcFirst lowercases the first character of a string.
func lcFirst(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

// ucFirst uppercases the first character of a string.
func ucFirst(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// snakeToCamel converts snake_case to camelCase.
// e.g., "user_id" -> "userID", "project_name" -> "projectName"
func snakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	for i := range parts {
		if i == 0 {
			continue
		}
		// Handle common abbreviations like "id" -> "ID"
		if strings.ToLower(parts[i]) == "id" {
			parts[i] = "ID"
		} else {
			parts[i] = ucFirst(parts[i])
		}
	}
	return strings.Join(parts, "")
}
