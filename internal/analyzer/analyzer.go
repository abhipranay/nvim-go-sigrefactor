package analyzer

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Analyzer provides semantic analysis of Go signatures
type Analyzer struct {
	pkgs  []*packages.Package
	fset  *token.FileSet
	loaded bool
}

// New creates a new Analyzer
func New() *Analyzer {
	return &Analyzer{}
}

// Analyze analyzes the signature at the given file and offset
func (a *Analyzer) Analyze(filename string, offset int) (*AnalysisResult, error) {
	absPath, err := filepath.Abs(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	dir := filepath.Dir(absPath)

	if err := a.loadPackages(dir); err != nil {
		return nil, fmt.Errorf("failed to load packages: %w", err)
	}

	// Find the function/method at offset
	funcDecl, pkg, file := a.findFuncAtOffset(absPath, offset)
	if funcDecl == nil {
		// Try to find interface method
		ifaceMethod, iface, methodPkg := a.findInterfaceMethodAtOffset(absPath, offset)
		if ifaceMethod != nil {
			return a.analyzeInterfaceMethod(ifaceMethod, iface, methodPkg, absPath)
		}
		return nil, fmt.Errorf("no function found at offset %d", offset)
	}

	sig := a.extractSignature(funcDecl, pkg, absPath)

	// Find usages
	usages := a.findUsages(funcDecl, pkg, file)

	return &AnalysisResult{
		Signature: sig,
		Usages:    usages,
	}, nil
}

func (a *Analyzer) loadPackages(dir string) error {
	if a.loaded {
		return nil
	}

	// Find module root (where go.mod is)
	moduleRoot := findModuleRoot(dir)
	if moduleRoot == "" {
		moduleRoot = dir
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports,
		Dir:   moduleRoot,
		Tests: true, // Include test files
	}

	// Load all packages from module root
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return err
	}

	// Also load the specific directory if it's not already included
	// (handles testdata directories which are skipped by ./...)
	dirIncluded := false
	for _, pkg := range pkgs {
		// Check both regular files and test files
		allFiles := append(pkg.GoFiles, pkg.OtherFiles...)
		for _, f := range allFiles {
			if filepath.Dir(f) == dir {
				dirIncluded = true
				break
			}
		}
		// Also check syntax files (includes test files)
		for _, f := range pkg.Syntax {
			pos := pkg.Fset.Position(f.Pos())
			if filepath.Dir(pos.Filename) == dir {
				dirIncluded = true
				break
			}
		}
		if dirIncluded {
			break
		}
	}

	if !dirIncluded {
		// Load the specific directory (including test files)
		dirCfg := &packages.Config{
			Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
				packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports,
			Dir:   dir,
			Tests: true,
		}
		dirPkgs, dirErr := packages.Load(dirCfg, ".")
		if dirErr == nil {
			pkgs = append(pkgs, dirPkgs...)
		}
	}

	// Check for errors in packages
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			// Log but don't fail - partial analysis is still useful
			for _, e := range pkg.Errors {
				fmt.Printf("package error: %v\n", e)
			}
		}
	}

	a.pkgs = pkgs
	if len(pkgs) > 0 {
		a.fset = pkgs[0].Fset
	}
	a.loaded = true

	return nil
}

// findModuleRoot walks up the directory tree to find go.mod
func findModuleRoot(dir string) string {
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			return ""
		}
		dir = parent
	}
}

func (a *Analyzer) findFuncAtOffset(filename string, offset int) (*ast.FuncDecl, *packages.Package, *ast.File) {
	for _, pkg := range a.pkgs {
		for _, file := range pkg.Syntax {
			pos := pkg.Fset.Position(file.Pos())
			if pos.Filename != filename {
				continue
			}

			var result *ast.FuncDecl
			ast.Inspect(file, func(n ast.Node) bool {
				if fd, ok := n.(*ast.FuncDecl); ok {
					start := pkg.Fset.Position(fd.Pos()).Offset
					end := pkg.Fset.Position(fd.End()).Offset
					if offset >= start && offset <= end {
						result = fd
						return false
					}
				}
				return true
			})

			if result != nil {
				return result, pkg, file
			}
		}
	}
	return nil, nil, nil
}

func (a *Analyzer) findInterfaceMethodAtOffset(filename string, offset int) (*ast.Field, *ast.TypeSpec, *packages.Package) {
	for _, pkg := range a.pkgs {
		for _, file := range pkg.Syntax {
			pos := pkg.Fset.Position(file.Pos())
			if pos.Filename != filename {
				continue
			}

			var resultField *ast.Field
			var resultIface *ast.TypeSpec

			ast.Inspect(file, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSpec)
				if !ok {
					return true
				}

				iface, ok := ts.Type.(*ast.InterfaceType)
				if !ok {
					return true
				}

				for _, method := range iface.Methods.List {
					if _, ok := method.Type.(*ast.FuncType); !ok {
						continue
					}

					start := pkg.Fset.Position(method.Pos()).Offset
					end := pkg.Fset.Position(method.End()).Offset
					if offset >= start && offset <= end {
						resultField = method
						resultIface = ts
						return false
					}
				}

				return true
			})

			if resultField != nil {
				return resultField, resultIface, pkg
			}
		}
	}
	return nil, nil, nil
}

func (a *Analyzer) analyzeInterfaceMethod(method *ast.Field, iface *ast.TypeSpec, pkg *packages.Package, _ string) (*AnalysisResult, error) {
	funcType := method.Type.(*ast.FuncType)

	sig := Signature{
		Name:          method.Names[0].Name,
		IsInterface:   true,
		InterfaceName: iface.Name.Name,
		Position:      a.positionFromNode(method, pkg),
	}

	sig.Params = a.extractParams(funcType.Params, pkg)
	sig.Returns = a.extractParams(funcType.Results, pkg)

	// Find implementations
	implementations := a.findImplementations(iface.Name.Name, method.Names[0].Name, pkg)

	// Find usages (interface method calls)
	usages := a.findInterfaceMethodUsages(iface.Name.Name, method.Names[0].Name, pkg)

	return &AnalysisResult{
		Signature:       sig,
		Usages:          usages,
		Implementations: implementations,
	}, nil
}

func (a *Analyzer) extractSignature(fd *ast.FuncDecl, pkg *packages.Package, _ string) Signature {
	sig := Signature{
		Name:     fd.Name.Name,
		Position: a.positionFromNode(fd, pkg),
	}

	// Extract receiver
	if fd.Recv != nil && len(fd.Recv.List) > 0 {
		recv := fd.Recv.List[0]
		sig.Receiver = &Receiver{}

		if len(recv.Names) > 0 {
			sig.Receiver.Name = recv.Names[0].Name
		}

		sig.Receiver.Type, sig.Receiver.Pointer = a.extractReceiverType(recv.Type)
	}

	// Extract parameters
	sig.Params = a.extractParams(fd.Type.Params, pkg)

	// Extract returns
	sig.Returns = a.extractParams(fd.Type.Results, pkg)

	// Extract doc comment
	if fd.Doc != nil {
		sig.Doc = fd.Doc.Text()
	}

	return sig
}

func (a *Analyzer) extractReceiverType(expr ast.Expr) (string, bool) {
	switch t := expr.(type) {
	case *ast.StarExpr:
		typeName, _ := a.extractReceiverType(t.X)
		return typeName, true
	case *ast.Ident:
		return t.Name, false
	default:
		return types.ExprString(expr), false
	}
}

func (a *Analyzer) extractParams(fields *ast.FieldList, _ *packages.Package) []Parameter {
	if fields == nil {
		return nil
	}

	var params []Parameter
	for _, field := range fields.List {
		typeStr := types.ExprString(field.Type)

		// Check for variadic
		variadic := false
		if _, ok := field.Type.(*ast.Ellipsis); ok {
			variadic = true
		}

		if len(field.Names) == 0 {
			// Unnamed parameter/return
			params = append(params, Parameter{
				Type:     typeStr,
				Variadic: variadic,
			})
		} else {
			for _, name := range field.Names {
				params = append(params, Parameter{
					Name:     name.Name,
					Type:     typeStr,
					Variadic: variadic,
				})
			}
		}
	}

	return params
}

func (a *Analyzer) positionFromNode(node ast.Node, pkg *packages.Package) Position {
	startPos := pkg.Fset.Position(node.Pos())
	endPos := pkg.Fset.Position(node.End())

	return Position{
		Filename: startPos.Filename,
		Line:     startPos.Line,
		Column:   startPos.Column,
		Offset:   startPos.Offset,
		EndLine:  endPos.Line,
		EndCol:   endPos.Column,
	}
}

func (a *Analyzer) findUsages(fd *ast.FuncDecl, pkg *packages.Package, _ *ast.File) []Usage {
	var usages []Usage

	// Get the object for the function
	obj := pkg.TypesInfo.Defs[fd.Name]
	if obj == nil {
		return usages
	}

	// Get function identity info for matching across packages
	funcName := obj.Name()
	funcPkgPath := obj.Pkg().Path()
	funcPos := pkg.Fset.Position(fd.Pos())

	// Search all packages for usages
	for _, searchPkg := range a.pkgs {
		for ident, useObj := range searchPkg.TypesInfo.Uses {
			// First try direct object comparison (same package)
			if useObj == obj {
				usage := Usage{
					Position: a.positionFromIdent(ident, searchPkg),
					Kind:     "call",
				}
				usage.InFunc = a.findEnclosingFuncName(ident, searchPkg)
				usages = append(usages, usage)
				continue
			}

			// For test packages, match by name and package path
			if useObj.Name() != funcName {
				continue
			}

			// Check if it's a function from the same package
			if useObj.Pkg() != nil && useObj.Pkg().Path() == funcPkgPath {
				usage := Usage{
					Position: a.positionFromIdent(ident, searchPkg),
					Kind:     "call",
				}
				usage.InFunc = a.findEnclosingFuncName(ident, searchPkg)
				usages = append(usages, usage)
				continue
			}

			// Also match by position for internal test packages
			if fn, ok := useObj.(*types.Func); ok {
				usePos := searchPkg.Fset.Position(fn.Pos())
				if usePos.Filename == funcPos.Filename && usePos.Offset == funcPos.Offset {
					usage := Usage{
						Position: a.positionFromIdent(ident, searchPkg),
						Kind:     "call",
					}
					usage.InFunc = a.findEnclosingFuncName(ident, searchPkg)
					usages = append(usages, usage)
				}
			}
		}
	}

	return usages
}

func (a *Analyzer) findEnclosingFuncName(ident *ast.Ident, pkg *packages.Package) string {
	for _, file := range pkg.Syntax {
		var enclosingFunc string
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.FuncDecl:
				start := pkg.Fset.Position(node.Pos()).Offset
				end := pkg.Fset.Position(node.End()).Offset
				identOffset := pkg.Fset.Position(ident.Pos()).Offset
				if identOffset >= start && identOffset <= end {
					enclosingFunc = node.Name.Name
				}
			}
			return true
		})
		if enclosingFunc != "" {
			return enclosingFunc
		}
	}
	return ""
}

func (a *Analyzer) positionFromIdent(ident *ast.Ident, pkg *packages.Package) Position {
	pos := pkg.Fset.Position(ident.Pos())
	endPos := pkg.Fset.Position(ident.End())

	return Position{
		Filename: pos.Filename,
		Line:     pos.Line,
		Column:   pos.Column,
		Offset:   pos.Offset,
		EndLine:  endPos.Line,
		EndCol:   endPos.Column,
	}
}

func (a *Analyzer) findImplementations(interfaceName, methodName string, pkg *packages.Package) []Implementation {
	var implementations []Implementation

	// Find the interface type
	var ifaceType *types.Interface
	for _, searchPkg := range a.pkgs {
		obj := searchPkg.Types.Scope().Lookup(interfaceName)
		if obj == nil {
			continue
		}
		named, ok := obj.Type().(*types.Named)
		if !ok {
			continue
		}
		iface, ok := named.Underlying().(*types.Interface)
		if !ok {
			continue
		}
		ifaceType = iface
		break
	}

	if ifaceType == nil {
		return implementations
	}

	// Find all types that implement the interface
	for _, searchPkg := range a.pkgs {
		for ident, obj := range searchPkg.TypesInfo.Defs {
			if obj == nil {
				continue
			}

			named, ok := obj.Type().(*types.Named)
			if !ok {
				continue
			}

			// Skip interfaces themselves
			if _, ok := named.Underlying().(*types.Interface); ok {
				continue
			}

			// Check if type implements interface (directly or via pointer)
			isPointer := false
			implements := types.Implements(named, ifaceType)
			if !implements {
				ptrType := types.NewPointer(named)
				implements = types.Implements(ptrType, ifaceType)
				if implements {
					isPointer = true
				}
			}

			if !implements {
				continue
			}

			// Find the method on this type
			methodSig := a.findMethodOnType(named, methodName, searchPkg, ident.Name)
			if methodSig.Name != "" {
				implementations = append(implementations, Implementation{
					TypeName:  ident.Name,
					Method:    methodSig,
					IsPointer: isPointer,
				})
			}
		}
	}

	// Deduplicate by type name
	seen := make(map[string]bool)
	var unique []Implementation
	for _, impl := range implementations {
		if seen[impl.TypeName] {
			continue
		}
		seen[impl.TypeName] = true
		unique = append(unique, impl)
	}

	return unique
}

func (a *Analyzer) findMethodOnType(named *types.Named, methodName string, pkg *packages.Package, typeName string) Signature {
	// Find the method in the AST
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv == nil {
				continue
			}

			if fd.Name.Name != methodName {
				continue
			}

			// Check receiver type
			recvType, _ := a.extractReceiverType(fd.Recv.List[0].Type)
			if recvType != typeName {
				continue
			}

			return a.extractSignature(fd, pkg, "")
		}
	}

	return Signature{}
}

func (a *Analyzer) findInterfaceMethodUsages(interfaceName, methodName string, pkg *packages.Package) []Usage {
	var usages []Usage

	for _, searchPkg := range a.pkgs {
		for _, file := range searchPkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}

				if sel.Sel.Name != methodName {
					return true
				}

				// Check if the receiver type is the interface
				tv, ok := searchPkg.TypesInfo.Types[sel.X]
				if !ok {
					return true
				}

				typeStr := tv.Type.String()
				if strings.Contains(typeStr, interfaceName) {
					usage := Usage{
						Position: a.positionFromNode(call, searchPkg),
						Kind:     "call",
						InFunc:   a.findEnclosingFuncNameForNode(call, searchPkg),
					}
					usages = append(usages, usage)
				}

				return true
			})
		}
	}

	return usages
}

func (a *Analyzer) findEnclosingFuncNameForNode(node ast.Node, pkg *packages.Package) string {
	nodePos := pkg.Fset.Position(node.Pos()).Offset

	for _, file := range pkg.Syntax {
		var enclosingFunc string
		ast.Inspect(file, func(n ast.Node) bool {
			fd, ok := n.(*ast.FuncDecl)
			if !ok {
				return true
			}
			start := pkg.Fset.Position(fd.Pos()).Offset
			end := pkg.Fset.Position(fd.End()).Offset
			if nodePos >= start && nodePos <= end {
				enclosingFunc = fd.Name.Name
			}
			return true
		})
		if enclosingFunc != "" {
			return enclosingFunc
		}
	}
	return ""
}
