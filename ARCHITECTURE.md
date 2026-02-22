# Architecture & Implementation Guide

This document explains the internal architecture of nvim-go-sigrefactor for developers who want to understand or contribute to the project.

## Overview

The plugin follows a **two-component architecture**:

```
┌─────────────────────────────────────────────────────────────┐
│                      Neovim                                 │
│  ┌─────────────────────────────────────────────────────┐   │
│  │              Lua Plugin                              │   │
│  │  • UI (floating window)                             │   │
│  │  • User interaction                                 │   │
│  │  • Edit application                                 │   │
│  └──────────────────────┬──────────────────────────────┘   │
│                         │ JSON over stdio                   │
│  ┌──────────────────────▼──────────────────────────────┐   │
│  │           Go CLI (gosigrefactor)                     │   │
│  │  • Semantic analysis                                │   │
│  │  • Type checking                                    │   │
│  │  • Refactoring logic                                │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

**Why this split?**

- **Go CLI**: Direct access to `go/ast`, `go/types`, `go/parser` - the same tools gopls uses. Full type-checked analysis via `golang.org/x/tools/go/packages`.
- **Lua Plugin**: Native Neovim integration, async operations, responsive UI.

## Directory Structure

```
nvim-go-sigrefactor/
├── cmd/gosigrefactor/          # CLI entry point
│   └── main.go
├── internal/
│   ├── analyzer/               # Signature analysis
│   │   ├── analyzer.go         # Core analysis logic
│   │   ├── types.go            # Data structures
│   │   └── *_test.go           # Tests
│   └── refactor/               # Refactoring logic
│       ├── refactor.go         # Core refactoring
│       └── *_test.go           # Tests
├── lua/go-sigrefactor/         # Neovim plugin
│   ├── init.lua                # Plugin entry point
│   ├── cli.lua                 # Go CLI integration
│   ├── ui.lua                  # Floating window UI
│   └── config.lua              # Configuration
├── plugin/
│   └── go-sigrefactor.lua      # Plugin registration
└── testdata/                   # Test fixtures
```

## Core Algorithms

### 1. Package Loading

The foundation of accurate refactoring is loading type-checked packages:

```go
cfg := &packages.Config{
    Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
          packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports,
    Dir:   moduleRoot,
    Tests: true,  // Include test files
}
pkgs, err := packages.Load(cfg, "./...")
```

**Key considerations:**

- **Module Root Discovery**: Walk up directory tree to find `go.mod` to ensure all packages in the module are loaded
- **Test Files**: Set `Tests: true` to include `_test.go` files
- **Testdata Handling**: The `./...` pattern skips `testdata` directories, so we explicitly load them if the target file is there

### 2. Function Discovery

Finding the function at a cursor position:

```go
func findFuncAtOffset(pkg *packages.Package, filename string, offset int) *ast.FuncDecl {
    for _, file := range pkg.Syntax {
        if pkg.Fset.Position(file.Pos()).Filename != filename {
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
        return result
    }
    return nil
}
```

**Algorithm**: Linear scan through AST nodes, checking if the byte offset falls within the node's range.

### 3. Usage Finding

Finding all call sites of a function across the codebase:

```go
func findUsages(funcObj types.Object, pkgs []*packages.Package) []Usage {
    var usages []Usage

    funcName := funcObj.Name()
    funcPkgPath := funcObj.Pkg().Path()

    for _, pkg := range pkgs {
        for ident, useObj := range pkg.TypesInfo.Uses {
            // Match by object identity (same package)
            if useObj == funcObj {
                usages = append(usages, makeUsage(ident, pkg))
                continue
            }

            // Match by name + package path (cross-package, including tests)
            if useObj.Name() == funcName &&
               useObj.Pkg() != nil &&
               useObj.Pkg().Path() == funcPkgPath {
                usages = append(usages, makeUsage(ident, pkg))
            }
        }
    }
    return usages
}
```

**Why multiple matching strategies?**

When `Tests: true` is set, `packages.Load` creates separate packages for test files. The function object in the test package is a different instance from the main package, so we can't rely solely on object identity (`useObj == funcObj`). We also match by:

1. **Object identity** - Same package (fastest)
2. **Name + Package path** - Cross-package references
3. **Source position** - Internal test packages (same file)

### 4. Interface Implementation Discovery

Finding all types that implement an interface:

```go
func findImplementations(pkgs []*packages.Package, iface *types.Interface) []*types.Named {
    var impls []*types.Named

    for _, pkg := range pkgs {
        for _, obj := range pkg.TypesInfo.Defs {
            named, ok := obj.Type().(*types.Named)
            if !ok {
                continue
            }

            // Skip interfaces themselves
            if _, ok := named.Underlying().(*types.Interface); ok {
                continue
            }

            // Check both value and pointer receivers
            if types.Implements(named, iface) ||
               types.Implements(types.NewPointer(named), iface) {
                impls = append(impls, named)
            }
        }
    }
    return impls
}
```

**Algorithm**: Iterate through all defined types in all packages, using `types.Implements()` to check interface satisfaction.

### 5. Parameter Mapping

When reordering/modifying parameters, we need to map old positions to new positions:

```go
func buildParamMapping(current []paramInfo, newParams []Parameter) map[int]int {
    mapping := make(map[int]int)  // old index -> new index
    usedOld := make(map[int]bool)

    // First pass: match by exact name
    for newIdx, newParam := range newParams {
        for oldIdx, oldParam := range current {
            if usedOld[oldIdx] {
                continue
            }
            if oldParam.Name == newParam.Name {
                mapping[oldIdx] = newIdx
                usedOld[oldIdx] = true
                break
            }
        }
    }

    // Second pass: match by type for renamed params at same position
    for newIdx, newParam := range newParams {
        if _, mapped := findOldIdxForNewIdx(mapping, newIdx); mapped {
            continue
        }
        if newIdx < len(current) && !usedOld[newIdx] {
            if current[newIdx].Type == newParam.Type {
                mapping[newIdx] = newIdx
                usedOld[newIdx] = true
            }
        }
    }

    return mapping
}
```

**Algorithm**:
1. First, match parameters by name (handles reordering)
2. Then, match by type at the same position (handles renaming)
3. Unmapped new parameters are considered additions
4. Unmapped old parameters are considered deletions

### 6. Call Site Transformation

Transforming call site arguments based on the parameter mapping:

```go
func transformCallSite(call *ast.CallExpr, mapping map[int]int, spec RefactorSpec) string {
    // Extract current arguments as source text
    currentArgs := make([]string, len(call.Args))
    for i, arg := range call.Args {
        var buf bytes.Buffer
        printer.Fprint(&buf, fset, arg)
        currentArgs[i] = buf.String()
    }

    // Build new arguments based on mapping
    newArgs := make([]string, len(spec.NewParams))
    for newIdx, param := range spec.NewParams {
        for oldIdx, mappedNewIdx := range mapping {
            if mappedNewIdx == newIdx && oldIdx < len(currentArgs) {
                newArgs[newIdx] = currentArgs[oldIdx]
                break
            }
        }

        // Use default value for new parameters
        if newArgs[newIdx] == "" {
            newArgs[newIdx] = spec.DefaultValues[param.Name]
        }
    }

    return strings.Join(newArgs, ", ")
}
```

### 7. Edit Generation

The plugin generates LSP-compatible workspace edits:

```go
type WorkspaceEdit struct {
    Changes map[string][]TextEdit `json:"changes"`
}

type TextEdit struct {
    Range   Range  `json:"range"`
    NewText string `json:"newText"`
}

type Range struct {
    Start Position `json:"start"`
    End   Position `json:"end"`
}
```

**Important**: Edits are sorted by offset in descending order before application. This ensures earlier edits don't shift the positions of later edits.

## Lua Plugin Architecture

### Async CLI Communication

```lua
function M.execute(cmd, args, callback)
    vim.system(full_args, { text = true }, function(result)
        vim.schedule(function()
            local parsed = vim.json.decode(result.stdout)
            callback(parsed, nil)
        end)
    end)
end
```

Uses `vim.system()` for non-blocking CLI execution with JSON communication.

### Edit Application

```lua
-- Edits are sorted by offset descending
table.sort(file_edits, function(a, b)
    return a.range.start.offset > b.range.start.offset
end)

-- Apply edits from end to start
for _, edit in ipairs(file_edits) do
    local start_offset = edit.range.start.offset
    local end_offset = edit.range["end"].offset
    content = content:sub(1, start_offset) .. edit.newText .. content:sub(end_offset + 1)
end
```

**Note**: Go offsets are 0-indexed, Lua strings are 1-indexed. The `sub(1, offset)` call gets the first `offset` bytes, which correctly handles the 0-indexed offset.

### Focus Management

The UI uses autocmds to handle focus:

```lua
vim.api.nvim_create_autocmd("WinLeave", {
    buffer = state.bufnr,
    callback = function()
        if state.in_input then return end  -- Don't close during vim.ui.input
        vim.defer_fn(function()
            M.close()
        end, 50)
    end,
})
```

## Testing

### Unit Tests

```bash
make test
```

Tests cover:
- Simple function analysis
- Method analysis (with receivers)
- Interface method analysis
- Usage finding
- Implementation finding
- Parameter reordering
- Parameter renaming
- Adding/removing parameters
- Interface refactoring
- Variadic functions
- Generic functions

### Manual Testing

1. Open a Go project in Neovim
2. Navigate to a function
3. Run `:GoChangeSignature`
4. Modify parameters
5. Preview and apply

## Contributing

### Adding New Features

1. **Add tests first** (TDD)
2. Implement in Go CLI (`internal/`)
3. Update Lua UI if needed (`lua/go-sigrefactor/`)
4. Update documentation

### Code Style

- Go: Follow standard Go conventions, run `make lint`
- Lua: Use 2-space indentation, prefer local functions

### Common Tasks

**Adding a new refactoring operation:**

1. Add the operation to `RefactorSpec` in `internal/analyzer/types.go`
2. Implement the transformation in `internal/refactor/refactor.go`
3. Add UI support in `lua/go-sigrefactor/ui.lua`

**Improving accuracy:**

1. Add a failing test case in `internal/analyzer/analyzer_test.go` or `internal/refactor/refactor_test.go`
2. Debug using the CLI: `./bin/gosigrefactor analyze --file=... --offset=...`
3. Fix the issue
4. Verify all tests pass

## Known Limitations

1. **Embedded interfaces**: Partial support
2. **Method expressions** (`Type.Method`): Not yet supported
3. **Cross-module refactoring**: Only works within a single Go module
4. **Undo**: No built-in undo (use git)

## Future Improvements

- [ ] Change return types with automatic error handling updates
- [ ] Support for method expressions
- [ ] Integration with gopls for better accuracy
- [ ] Undo/redo support
- [ ] Batch refactoring across multiple functions
