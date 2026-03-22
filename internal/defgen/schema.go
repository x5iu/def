package defgen

import (
	"fmt"
	"go/ast"
	"go/types"
	"reflect"
	"strings"
)

// parseStructTags parses db and foreign_key tags from a struct type.
func parseStructTags(structType *types.Struct) (fields []FieldInfo, foreignKeys []ForeignKeyInfo) {
	for i := range structType.NumFields() {
		field := structType.Field(i)
		tag := structType.Tag(i)

		// Parse struct tags
		st := reflect.StructTag(tag)
		dbTag := st.Get("db")
		fkTag := st.Get("foreign_key")

		// Skip if no db tag
		if dbTag == "" {
			continue
		}

		// Handle db:"-" - this is a non-database field
		if dbTag == "-" {
			// Check if it has a foreign_key tag
			if fkTag != "" {
				inverseTag := st.Get("inverse")
				fk := ForeignKeyInfo{
					FieldName: field.Name(),
					KeyColumn: fkTag,
					RefType:   field.Type(),
					Inverse:   inverseTag,
				}
				foreignKeys = append(foreignKeys, fk)
			}
			continue
		}

		// Parse primary_key and auto_increment tags
		pkTag := st.Get("primary_key")
		aiTag := st.Get("auto_increment")

		// Regular database field
		fi := FieldInfo{
			GoName:          field.Name(),
			DBName:          dbTag,
			Type:            field.Type(),
			IsPrimaryKey:    pkTag == "true",
			IsAutoIncrement: aiTag == "true",
		}
		fields = append(fields, fi)
	}

	return fields, foreignKeys
}

// getTypeName extracts the type name from a types.Type.
func getTypeName(t types.Type) string {
	switch ty := t.(type) {
	case *types.Pointer:
		return getTypeName(ty.Elem())
	case *types.Slice:
		return getTypeName(ty.Elem())
	case *types.Named:
		return ty.Obj().Name()
	default:
		return ""
	}
}

// getTypeKey extracts a stable, package-qualified key from a types.Type.
// Example: "*entity.User" -> "example.com/project/entity.User".
func getTypeKey(t types.Type) string {
	switch ty := t.(type) {
	case *types.Pointer:
		return getTypeKey(ty.Elem())
	case *types.Slice:
		return getTypeKey(ty.Elem())
	case *types.Named:
		obj := ty.Obj()
		if obj == nil {
			return ""
		}
		if obj.Pkg() == nil {
			return obj.Name()
		}
		return obj.Pkg().Path() + "." + obj.Name()
	default:
		return ""
	}
}

func typeStringForLookup(t types.Type) string {
	if t == nil {
		return "<nil>"
	}
	return types.TypeString(t, func(p *types.Package) string {
		if p == nil {
			return ""
		}
		return p.Path()
	})
}

// lookupTableByType resolves a table binding from a concrete type.
func lookupTableByType(pkg *Package, t types.Type) (*TableBinding, error) {
	key := getTypeKey(t)
	if key == "" {
		return nil, fmt.Errorf("table binding not found for type %s", typeStringForLookup(t))
	}
	if binding, ok := pkg.Tables[key]; ok {
		return binding, nil
	}

	return nil, fmt.Errorf("table binding not found for type %s", typeStringForLookup(t))
}

// lookupTableByTargetType resolves a target type key to a table binding.
func lookupTableByTargetType(pkg *Package, targetType string) (*TableBinding, error) {
	if targetType == "" {
		return nil, fmt.Errorf("empty target type")
	}
	if binding, ok := pkg.Tables[targetType]; ok {
		return binding, nil
	}
	return nil, fmt.Errorf("table binding not found for target type %q", targetType)
}

// parseAllStructs scans the package and collects all struct definitions with their tags.
// This builds the initial schema information before table bindings are processed.
func parseAllStructs(pkg *Package) map[string]*structInfo {
	structs := make(map[string]*structInfo)

	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}

			// Only process struct types
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}

			name := ts.Name.Name
			obj := pkg.TypesPkg.Scope().Lookup(name)
			if obj == nil {
				return true
			}

			named, ok := obj.Type().(*types.Named)
			if !ok {
				return true
			}

			underlying, ok := named.Underlying().(*types.Struct)
			if !ok {
				return true
			}

			typeKey := getTypeKey(named)
			if typeKey == "" {
				return true
			}

			fields, foreignKeys := parseStructTags(underlying)
			structs[typeKey] = &structInfo{
				name:        name,
				typ:         named,
				structType:  underlying,
				fields:      fields,
				foreignKeys: foreignKeys,
				astNode:     st,
			}

			return true
		})
	}

	return structs
}

// structInfo holds parsed information about a struct definition.
type structInfo struct {
	name        string
	typ         *types.Named
	structType  *types.Struct
	fields      []FieldInfo
	foreignKeys []ForeignKeyInfo
	astNode     *ast.StructType
}

// getStructInfoFromType dynamically parses struct info from a types.Type.
// This supports external package types without requiring pre-built struct maps.
func getStructInfoFromType(t types.Type) *structInfo {
	// Handle pointer types
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}

	named, ok := t.(*types.Named)
	if !ok {
		return nil
	}

	underlying, ok := named.Underlying().(*types.Struct)
	if !ok {
		return nil
	}

	fields, foreignKeys := parseStructTags(underlying)
	return &structInfo{
		name:        named.Obj().Name(),
		typ:         named,
		structType:  underlying,
		fields:      fields,
		foreignKeys: foreignKeys,
	}
}

// getFieldByName finds a field in the struct by its Go name.
func (s *structInfo) getFieldByName(name string) (FieldInfo, bool) {
	for _, f := range s.fields {
		if f.GoName == name {
			return f, true
		}
	}
	return FieldInfo{}, false
}

// getForeignKeyByName finds a foreign key by its Go field name.
func (s *structInfo) getForeignKeyByName(name string) (ForeignKeyInfo, bool) {
	for _, fk := range s.foreignKeys {
		if fk.FieldName == name {
			return fk, true
		}
	}
	return ForeignKeyInfo{}, false
}

// parseInterfaceDefs scans the package for interface definitions.
func parseInterfaceDefs(pkg *Package) {
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}

			it, ok := ts.Type.(*ast.InterfaceType)
			if !ok {
				return true
			}

			name := ts.Name.Name
			info := &InterfaceInfo{
				Name: name,
				Pos:  ts.Pos(),
			}

			// Parse interface methods
			for _, method := range it.Methods.List {
				if len(method.Names) == 0 {
					continue
				}

				methodName := method.Names[0].Name
				ft, ok := method.Type.(*ast.FuncType)
				if !ok {
					continue
				}

				// Parse parameters
				var params []ParamInfo
				var paramTypes []types.Type
				if ft.Params != nil {
					for _, param := range ft.Params.List {
						paramType := pkg.TypesInfo.TypeOf(param.Type)
						if len(param.Names) == 0 {
							params = append(params, ParamInfo{
								Name: "",
								Type: paramType,
							})
							paramTypes = append(paramTypes, paramType)
							continue
						}
						for _, name := range param.Names {
							params = append(params, ParamInfo{
								Name: name.Name,
								Type: paramType,
							})
							paramTypes = append(paramTypes, paramType)
						}
					}
				}

				// Parse return type
				var returnType ReturnTypeInfo
				var resultTypes []types.Type
				if ft.Results != nil && len(ft.Results.List) > 0 {
					firstResult := ft.Results.List[0]
					resultType := pkg.TypesInfo.TypeOf(firstResult.Type)
					returnType = analyzeReturnType(resultType)
					for _, result := range ft.Results.List {
						resultType := pkg.TypesInfo.TypeOf(result.Type)
						if len(result.Names) == 0 {
							resultTypes = append(resultTypes, resultType)
							continue
						}
						for range result.Names {
							resultTypes = append(resultTypes, resultType)
						}
					}
				}

				// Build signature string
				sig := buildSignature(pkg, ft)

				info.Methods = append(info.Methods, InterfaceMethod{
					Name:        methodName,
					Signature:   sig,
					Params:      params,
					ParamTypes:  paramTypes,
					ResultTypes: resultTypes,
					ReturnType:  returnType,
				})
			}

			pkg.Interfaces[name] = info
			return true
		})
	}
}

// analyzeReturnType analyzes a return type to determine if it's a slice and extract element type.
func analyzeReturnType(t types.Type) ReturnTypeInfo {
	info := ReturnTypeInfo{Type: t}

	switch ty := t.(type) {
	case *types.Slice:
		info.IsSlice = true
		info.ElemType = ty.Elem()
		info.StructName = getTypeName(ty.Elem())
	case *types.Pointer:
		info.IsSlice = false
		info.ElemType = ty.Elem()
		info.StructName = getTypeName(ty.Elem())
	case *types.Named:
		info.StructName = ty.Obj().Name()
		info.ElemType = t
	}

	return info
}

// buildSignature builds a method signature string from an ast.FuncType.
func buildSignature(pkg *Package, ft *ast.FuncType) string {
	var sb strings.Builder
	sb.WriteString("(")

	// Parameters
	if ft.Params != nil {
		for i, param := range ft.Params.List {
			if i > 0 {
				sb.WriteString(", ")
			}
			// Write parameter names
			for j, name := range param.Names {
				if j > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(name.Name)
			}
			sb.WriteString(" ")
			sb.WriteString(types.ExprString(param.Type))
		}
	}

	sb.WriteString(")")

	// Results
	if ft.Results != nil && len(ft.Results.List) > 0 {
		sb.WriteString(" ")
		if len(ft.Results.List) == 1 && len(ft.Results.List[0].Names) == 0 {
			sb.WriteString(types.ExprString(ft.Results.List[0].Type))
		} else {
			sb.WriteString("(")
			for i, result := range ft.Results.List {
				if i > 0 {
					sb.WriteString(", ")
				}
				for j, name := range result.Names {
					if j > 0 {
						sb.WriteString(", ")
					}
					sb.WriteString(name.Name)
				}
				if len(result.Names) > 0 {
					sb.WriteString(" ")
				}
				sb.WriteString(types.ExprString(result.Type))
			}
			sb.WriteString(")")
		}
	}

	return sb.String()
}
