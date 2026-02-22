package refactor

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hellofresh/nvim-go-sigrefactor/internal/analyzer"
	"golang.org/x/tools/go/packages"
)

// Refactorer performs signature refactoring
type Refactorer struct {
	pkgs   []*packages.Package
	fset   *token.FileSet
	loaded bool
}

// New creates a new Refactorer
func New() *Refactorer {
	return &Refactorer{}
}

// Refactor applies the refactoring spec to the function at the given offset
func (r *Refactorer) Refactor(filename string, offset int, spec analyzer.RefactorSpec) (*analyzer.WorkspaceEdit, error) {
	absPath, err := filepath.Abs(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	dir := filepath.Dir(absPath)

	if err := r.loadPackages(dir); err != nil {
		return nil, fmt.Errorf("failed to load packages: %w", err)
	}

	// Find the function/method at offset
	funcDecl, pkg, file := r.findFuncAtOffset(absPath, offset)
	if funcDecl == nil {
		// Try to find interface method
		ifaceMethod, iface, methodPkg := r.findInterfaceMethodAtOffset(absPath, offset)
		if ifaceMethod != nil {
			return r.refactorInterfaceMethod(ifaceMethod, iface, methodPkg, spec)
		}
		return nil, fmt.Errorf("no function found at offset %d", offset)
	}

	return r.refactorFunction(funcDecl, pkg, file, spec)
}

func (r *Refactorer) loadPackages(dir string) error {
	if r.loaded {
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
		for _, f := range pkg.GoFiles {
			if filepath.Dir(f) == dir {
				dirIncluded = true
				break
			}
		}
		if dirIncluded {
			break
		}
	}

	if !dirIncluded {
		// Load the specific directory
		dirCfg := &packages.Config{
			Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
				packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports,
			Dir:   dir,
			Tests: true,
		}
		dirPkgs, err := packages.Load(dirCfg, ".")
		if err == nil {
			pkgs = append(pkgs, dirPkgs...)
		}
	}

	r.pkgs = pkgs
	if len(pkgs) > 0 {
		r.fset = pkgs[0].Fset
	}
	r.loaded = true

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

func (r *Refactorer) findFuncAtOffset(filename string, offset int) (*ast.FuncDecl, *packages.Package, *ast.File) {
	for _, pkg := range r.pkgs {
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

func (r *Refactorer) findInterfaceMethodAtOffset(filename string, offset int) (*ast.Field, *ast.TypeSpec, *packages.Package) {
	for _, pkg := range r.pkgs {
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

func (r *Refactorer) refactorFunction(fd *ast.FuncDecl, pkg *packages.Package, file *ast.File, spec analyzer.RefactorSpec) (*analyzer.WorkspaceEdit, error) {
	edits := &analyzer.WorkspaceEdit{
		Changes: make(map[string][]analyzer.TextEdit),
	}

	// Get current signature info
	currentParams := r.extractParamInfo(fd.Type.Params, pkg)

	// Build parameter mapping (old index -> new index)
	paramMapping := r.buildParamMapping(currentParams, spec.NewParams)

	// 1. Edit function signature
	sigEdit := r.createSignatureEdit(fd, pkg, spec)
	filename := pkg.Fset.Position(fd.Pos()).Filename
	edits.Changes[filename] = append(edits.Changes[filename], sigEdit)

	// 2. Edit parameter usages inside function body
	if fd.Body != nil {
		bodyEdits := r.createParamRenameEdits(fd, pkg, currentParams, spec.NewParams)
		edits.Changes[filename] = append(edits.Changes[filename], bodyEdits...)
	}

	// 3. Find and edit all call sites
	callEdits := r.findAndEditCallSites(fd, pkg, paramMapping, spec)
	for fname, fileEdits := range callEdits {
		edits.Changes[fname] = append(edits.Changes[fname], fileEdits...)
	}

	// Deduplicate edits (same file + same offset range)
	for fname := range edits.Changes {
		edits.Changes[fname] = deduplicateEdits(edits.Changes[fname])
	}

	// Sort edits by position (descending) within each file
	for fname := range edits.Changes {
		sortEditsByOffset(edits.Changes[fname])
	}

	return edits, nil
}

// deduplicateEdits removes duplicate edits with the same offset range
func deduplicateEdits(edits []analyzer.TextEdit) []analyzer.TextEdit {
	seen := make(map[string]bool)
	var result []analyzer.TextEdit

	for _, edit := range edits {
		key := fmt.Sprintf("%d-%d", edit.Range.Start.Offset, edit.Range.End.Offset)
		if !seen[key] {
			seen[key] = true
			result = append(result, edit)
		}
	}

	return result
}

func (r *Refactorer) refactorInterfaceMethod(method *ast.Field, iface *ast.TypeSpec, pkg *packages.Package, spec analyzer.RefactorSpec) (*analyzer.WorkspaceEdit, error) {
	edits := &analyzer.WorkspaceEdit{
		Changes: make(map[string][]analyzer.TextEdit),
	}

	funcType := method.Type.(*ast.FuncType)
	currentParams := r.extractParamInfo(funcType.Params, pkg)
	paramMapping := r.buildParamMapping(currentParams, spec.NewParams)

	// 1. Edit interface method
	ifaceEdit := r.createInterfaceMethodEdit(method, pkg, spec)
	filename := pkg.Fset.Position(method.Pos()).Filename
	edits.Changes[filename] = append(edits.Changes[filename], ifaceEdit)

	// 2. Find and edit all implementations
	implEdits := r.findAndEditImplementations(iface.Name.Name, method.Names[0].Name, pkg, spec, currentParams)
	for fname, fileEdits := range implEdits {
		edits.Changes[fname] = append(edits.Changes[fname], fileEdits...)
	}

	// 3. Find and edit all call sites
	callEdits := r.findAndEditInterfaceCallSites(iface.Name.Name, method.Names[0].Name, pkg, paramMapping, spec)
	for fname, fileEdits := range callEdits {
		edits.Changes[fname] = append(edits.Changes[fname], fileEdits...)
	}

	// Deduplicate edits (same file + same offset range)
	for fname := range edits.Changes {
		edits.Changes[fname] = deduplicateEdits(edits.Changes[fname])
	}

	// Sort edits
	for fname := range edits.Changes {
		sortEditsByOffset(edits.Changes[fname])
	}

	return edits, nil
}

type paramInfo struct {
	Name string
	Type string
}

func (r *Refactorer) extractParamInfo(params *ast.FieldList, pkg *packages.Package) []paramInfo {
	if params == nil {
		return nil
	}

	var result []paramInfo
	for _, field := range params.List {
		typeStr := types.ExprString(field.Type)
		if len(field.Names) == 0 {
			result = append(result, paramInfo{Type: typeStr})
		} else {
			for _, name := range field.Names {
				result = append(result, paramInfo{Name: name.Name, Type: typeStr})
			}
		}
	}
	return result
}

func (r *Refactorer) buildParamMapping(current []paramInfo, newParams []analyzer.Parameter) map[int]int {
	mapping := make(map[int]int)
	usedOld := make(map[int]bool)

	// First pass: match by exact name
	for newIdx, newParam := range newParams {
		for oldIdx, oldParam := range current {
			if usedOld[oldIdx] {
				continue
			}
			if oldParam.Name != "" && oldParam.Name == newParam.Name {
				mapping[oldIdx] = newIdx
				usedOld[oldIdx] = true
				break
			}
		}
	}

	// Second pass: match by type for remaining params (position-based rename case)
	for newIdx, newParam := range newParams {
		if _, hasMapped := r.findOldIdxForNewIdx(mapping, newIdx); hasMapped {
			continue
		}
		// Check if there's an old param at same position with same type
		if newIdx < len(current) && !usedOld[newIdx] {
			oldParam := current[newIdx]
			if oldParam.Type == newParam.Type {
				mapping[newIdx] = newIdx
				usedOld[newIdx] = true
			}
		}
	}

	return mapping
}

func (r *Refactorer) findOldIdxForNewIdx(mapping map[int]int, newIdx int) (int, bool) {
	for oldIdx, mappedNewIdx := range mapping {
		if mappedNewIdx == newIdx {
			return oldIdx, true
		}
	}
	return -1, false
}

func (r *Refactorer) createSignatureEdit(fd *ast.FuncDecl, pkg *packages.Package, spec analyzer.RefactorSpec) analyzer.TextEdit {
	// Build new parameter list string
	var params []string
	for _, param := range spec.NewParams {
		if param.Variadic {
			params = append(params, fmt.Sprintf("%s ...%s", param.Name, strings.TrimPrefix(param.Type, "...")))
		} else {
			params = append(params, fmt.Sprintf("%s %s", param.Name, param.Type))
		}
	}

	// Build new return list string
	var returns string
	if len(spec.NewReturns) == 1 && spec.NewReturns[0].Name == "" {
		returns = spec.NewReturns[0].Type
	} else if len(spec.NewReturns) > 0 {
		var retParts []string
		for _, ret := range spec.NewReturns {
			if ret.Name != "" {
				retParts = append(retParts, fmt.Sprintf("%s %s", ret.Name, ret.Type))
			} else {
				retParts = append(retParts, ret.Type)
			}
		}
		returns = "(" + strings.Join(retParts, ", ") + ")"
	}

	// Get positions
	startPos := pkg.Fset.Position(fd.Type.Params.Pos())
	var endPos token.Position
	if fd.Type.Results != nil {
		endPos = pkg.Fset.Position(fd.Type.Results.End())
	} else {
		endPos = pkg.Fset.Position(fd.Type.Params.End())
	}

	newText := "(" + strings.Join(params, ", ") + ")"
	if returns != "" {
		newText += " " + returns
	}

	return analyzer.TextEdit{
		Range: analyzer.Range{
			Start: analyzer.Position{
				Filename: startPos.Filename,
				Line:     startPos.Line,
				Column:   startPos.Column,
				Offset:   startPos.Offset,
			},
			End: analyzer.Position{
				Filename: endPos.Filename,
				Line:     endPos.Line,
				Column:   endPos.Column,
				Offset:   endPos.Offset,
			},
		},
		NewText: newText,
	}
}

func (r *Refactorer) createInterfaceMethodEdit(method *ast.Field, pkg *packages.Package, spec analyzer.RefactorSpec) analyzer.TextEdit {
	funcType := method.Type.(*ast.FuncType)

	// Build new parameter list
	var params []string
	for _, param := range spec.NewParams {
		if param.Variadic {
			params = append(params, fmt.Sprintf("%s ...%s", param.Name, strings.TrimPrefix(param.Type, "...")))
		} else {
			params = append(params, fmt.Sprintf("%s %s", param.Name, param.Type))
		}
	}

	// Build new return list
	var returns string
	if len(spec.NewReturns) == 1 && spec.NewReturns[0].Name == "" {
		returns = spec.NewReturns[0].Type
	} else if len(spec.NewReturns) > 0 {
		var retParts []string
		for _, ret := range spec.NewReturns {
			if ret.Name != "" {
				retParts = append(retParts, fmt.Sprintf("%s %s", ret.Name, ret.Type))
			} else {
				retParts = append(retParts, ret.Type)
			}
		}
		returns = "(" + strings.Join(retParts, ", ") + ")"
	}

	startPos := pkg.Fset.Position(funcType.Params.Pos())
	var endPos token.Position
	if funcType.Results != nil {
		endPos = pkg.Fset.Position(funcType.Results.End())
	} else {
		endPos = pkg.Fset.Position(funcType.Params.End())
	}

	newText := "(" + strings.Join(params, ", ") + ")"
	if returns != "" {
		newText += " " + returns
	}

	return analyzer.TextEdit{
		Range: analyzer.Range{
			Start: analyzer.Position{
				Filename: startPos.Filename,
				Line:     startPos.Line,
				Column:   startPos.Column,
				Offset:   startPos.Offset,
			},
			End: analyzer.Position{
				Filename: endPos.Filename,
				Line:     endPos.Line,
				Column:   endPos.Column,
				Offset:   endPos.Offset,
			},
		},
		NewText: newText,
	}
}

func (r *Refactorer) createParamRenameEdits(fd *ast.FuncDecl, pkg *packages.Package, currentParams []paramInfo, newParams []analyzer.Parameter) []analyzer.TextEdit {
	var edits []analyzer.TextEdit

	// Build param mapping first (old index -> new index) using name matching
	paramMapping := r.buildParamMapping(currentParams, newParams)

	// Build rename map using the correct mapping
	renames := make(map[string]string)
	for oldIdx, newIdx := range paramMapping {
		oldName := currentParams[oldIdx].Name
		newName := newParams[newIdx].Name
		if oldName != "" && newName != "" && oldName != newName {
			renames[oldName] = newName
		}
	}

	if len(renames) == 0 {
		return edits
	}

	// Find parameter objects
	paramObjs := make(map[string]types.Object)
	for _, field := range fd.Type.Params.List {
		for _, name := range field.Names {
			if obj := pkg.TypesInfo.Defs[name]; obj != nil {
				paramObjs[name.Name] = obj
			}
		}
	}

	// Find usages in body
	if fd.Body != nil {
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			ident, ok := n.(*ast.Ident)
			if !ok {
				return true
			}

			newName, shouldRename := renames[ident.Name]
			if !shouldRename {
				return true
			}

			// Check if this identifier refers to the parameter
			if obj := pkg.TypesInfo.Uses[ident]; obj != nil {
				if paramObj, exists := paramObjs[ident.Name]; exists && obj == paramObj {
					pos := pkg.Fset.Position(ident.Pos())
					endPos := pkg.Fset.Position(ident.End())
					edits = append(edits, analyzer.TextEdit{
						Range: analyzer.Range{
							Start: analyzer.Position{
								Filename: pos.Filename,
								Line:     pos.Line,
								Column:   pos.Column,
								Offset:   pos.Offset,
							},
							End: analyzer.Position{
								Filename: endPos.Filename,
								Line:     endPos.Line,
								Column:   endPos.Column,
								Offset:   endPos.Offset,
							},
						},
						NewText: newName,
					})
				}
			}

			return true
		})
	}

	return edits
}

func (r *Refactorer) findAndEditCallSites(fd *ast.FuncDecl, pkg *packages.Package, paramMapping map[int]int, spec analyzer.RefactorSpec) map[string][]analyzer.TextEdit {
	edits := make(map[string][]analyzer.TextEdit)

	// Get function object
	funcObj := pkg.TypesInfo.Defs[fd.Name]
	if funcObj == nil {
		return edits
	}

	// Get function identity info for matching across packages (including test packages)
	funcName := funcObj.Name()
	funcPkgPath := funcObj.Pkg().Path()
	funcPos := pkg.Fset.Position(fd.Pos())

	// Search all packages for call sites
	for _, searchPkg := range r.pkgs {
		for _, file := range searchPkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				// Check if this call is to our function
				var callIdent *ast.Ident
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					callIdent = fun
				case *ast.SelectorExpr:
					callIdent = fun.Sel
				default:
					return true
				}

				useObj := searchPkg.TypesInfo.Uses[callIdent]
				if useObj == nil {
					return true
				}

				// Match by object identity, name+package, or position
				isMatch := false
				if useObj == funcObj {
					isMatch = true
				} else if useObj.Name() == funcName && useObj.Pkg() != nil && useObj.Pkg().Path() == funcPkgPath {
					isMatch = true
				} else if fn, ok := useObj.(*types.Func); ok {
					usePos := searchPkg.Fset.Position(fn.Pos())
					if usePos.Filename == funcPos.Filename && usePos.Offset == funcPos.Offset {
						isMatch = true
					}
				}

				if !isMatch {
					return true
				}

				// Create edit for this call site
				edit := r.createCallSiteEdit(call, searchPkg, paramMapping, spec)
				if edit.NewText != "" {
					filename := searchPkg.Fset.Position(call.Pos()).Filename
					edits[filename] = append(edits[filename], edit)
				}

				return true
			})
		}
	}

	return edits
}

func (r *Refactorer) findAndEditInterfaceCallSites(ifaceName, methodName string, pkg *packages.Package, paramMapping map[int]int, spec analyzer.RefactorSpec) map[string][]analyzer.TextEdit {
	edits := make(map[string][]analyzer.TextEdit)

	for _, searchPkg := range r.pkgs {
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

				// Check if receiver type is the interface
				tv, ok := searchPkg.TypesInfo.Types[sel.X]
				if !ok {
					return true
				}

				typeStr := tv.Type.String()
				if !strings.Contains(typeStr, ifaceName) {
					return true
				}

				edit := r.createCallSiteEdit(call, searchPkg, paramMapping, spec)
				if edit.NewText != "" {
					filename := searchPkg.Fset.Position(call.Pos()).Filename
					edits[filename] = append(edits[filename], edit)
				}

				return true
			})
		}
	}

	return edits
}

func (r *Refactorer) createCallSiteEdit(call *ast.CallExpr, pkg *packages.Package, paramMapping map[int]int, spec analyzer.RefactorSpec) analyzer.TextEdit {
	// Extract current arguments as strings using go/printer
	currentArgs := make([]string, len(call.Args))
	for i, arg := range call.Args {
		var buf bytes.Buffer
		if err := printer.Fprint(&buf, pkg.Fset, arg); err == nil {
			currentArgs[i] = buf.String()
		}
	}

	// Build new arguments list based on mapping
	newArgs := make([]string, len(spec.NewParams))
	for newIdx, param := range spec.NewParams {
		found := false
		for oldIdx, newIdxMapped := range paramMapping {
			if newIdxMapped == newIdx && oldIdx < len(currentArgs) {
				newArgs[newIdx] = currentArgs[oldIdx]
				found = true
				break
			}
		}

		if !found {
			// Check for default value
			if defaultVal, ok := spec.DefaultValues[param.Name]; ok {
				newArgs[newIdx] = defaultVal
			}
		}
	}

	// Skip if no changes
	if len(newArgs) == len(currentArgs) {
		allSame := true
		for i, arg := range newArgs {
			if i >= len(currentArgs) || arg != currentArgs[i] {
				allSame = false
				break
			}
		}
		if allSame {
			return analyzer.TextEdit{}
		}
	}

	// Get argument list position
	var startPos, endPos token.Position
	if len(call.Args) > 0 {
		startPos = pkg.Fset.Position(call.Args[0].Pos())
		endPos = pkg.Fset.Position(call.Args[len(call.Args)-1].End())
	} else {
		// Empty args: position right after opening paren
		startPos = pkg.Fset.Position(call.Lparen + 1)
		endPos = startPos
	}

	return analyzer.TextEdit{
		Range: analyzer.Range{
			Start: analyzer.Position{
				Filename: startPos.Filename,
				Line:     startPos.Line,
				Column:   startPos.Column,
				Offset:   startPos.Offset,
			},
			End: analyzer.Position{
				Filename: endPos.Filename,
				Line:     endPos.Line,
				Column:   endPos.Column,
				Offset:   endPos.Offset,
			},
		},
		NewText: strings.Join(newArgs, ", "),
	}
}


func (r *Refactorer) findAndEditImplementations(ifaceName, methodName string, pkg *packages.Package, spec analyzer.RefactorSpec, currentParams []paramInfo) map[string][]analyzer.TextEdit {
	edits := make(map[string][]analyzer.TextEdit)

	// Find interface type
	var ifaceType *types.Interface
	for _, searchPkg := range r.pkgs {
		obj := searchPkg.Types.Scope().Lookup(ifaceName)
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
		return edits
	}

	// Find implementing types
	for _, searchPkg := range r.pkgs {
		for ident, obj := range searchPkg.TypesInfo.Defs {
			if obj == nil {
				continue
			}

			named, ok := obj.Type().(*types.Named)
			if !ok {
				continue
			}

			if _, ok := named.Underlying().(*types.Interface); ok {
				continue
			}

			implements := types.Implements(named, ifaceType)
			if !implements {
				ptrType := types.NewPointer(named)
				implements = types.Implements(ptrType, ifaceType)
			}

			if !implements {
				continue
			}

			// Find the method on this type
			for _, file := range searchPkg.Syntax {
				for _, decl := range file.Decls {
					fd, ok := decl.(*ast.FuncDecl)
					if !ok || fd.Recv == nil {
						continue
					}

					if fd.Name.Name != methodName {
						continue
					}

					recvType := r.getReceiverTypeName(fd.Recv.List[0].Type)
					if recvType != ident.Name {
						continue
					}

					// Create edit for this implementation
					edit := r.createSignatureEdit(fd, searchPkg, spec)
					filename := searchPkg.Fset.Position(fd.Pos()).Filename
					edits[filename] = append(edits[filename], edit)

					// Also edit parameter usages in body
					implParams := r.extractParamInfo(fd.Type.Params, searchPkg)
					bodyEdits := r.createParamRenameEdits(fd, searchPkg, implParams, spec.NewParams)
					edits[filename] = append(edits[filename], bodyEdits...)
				}
			}
		}
	}

	return edits
}

func (r *Refactorer) getReceiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return r.getReceiverTypeName(t.X)
	case *ast.Ident:
		return t.Name
	default:
		return ""
	}
}

func sortEditsByOffset(edits []analyzer.TextEdit) {
	sort.Slice(edits, func(i, j int) bool {
		return edits[i].Range.Start.Offset > edits[j].Range.Start.Offset
	})
}
