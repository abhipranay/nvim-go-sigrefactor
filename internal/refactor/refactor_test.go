package refactor_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hellofresh/nvim-go-sigrefactor/internal/analyzer"
	"github.com/hellofresh/nvim-go-sigrefactor/internal/refactor"
)

func TestReorderParameters(t *testing.T) {
	// Given a function with parameters
	testDir := setupTestDir(t, map[string]string{
		"main.go": `package main

func ProcessOrder(ctx context.Context, orderID string, amount int) error {
	return nil
}

func caller() {
	_ = ProcessOrder(context.Background(), "123", 100)
}
`,
	})

	// When reordering parameters (swap orderID and amount)
	r := refactor.New()
	spec := analyzer.RefactorSpec{
		NewParams: []analyzer.Parameter{
			{Name: "ctx", Type: "context.Context"},
			{Name: "amount", Type: "int"},
			{Name: "orderID", Type: "string"},
		},
		NewReturns: []analyzer.Parameter{
			{Type: "error"},
		},
	}

	edits, err := r.Refactor(filepath.Join(testDir, "main.go"), 25, spec)

	// Then we should get correct edits for both declaration and call site
	if err != nil {
		t.Fatalf("Refactor failed: %v", err)
	}

	if len(edits.Changes) == 0 {
		t.Fatal("expected edits, got none")
	}

	// Apply edits and verify
	content := applyEdits(t, testDir, edits)

	// Verify function signature changed
	if !strings.Contains(content["main.go"], "func ProcessOrder(ctx context.Context, amount int, orderID string)") {
		t.Errorf("function signature not updated correctly:\n%s", content["main.go"])
	}

	// Verify call site arguments reordered
	if !strings.Contains(content["main.go"], `ProcessOrder(context.Background(), 100, "123")`) {
		t.Errorf("call site not updated correctly:\n%s", content["main.go"])
	}
}

func TestRenameParameter(t *testing.T) {
	// Given a function with a parameter to rename
	testDir := setupTestDir(t, map[string]string{
		"main.go": `package main

func ProcessOrder(ctx context.Context, orderID string) error {
	fmt.Println(orderID)
	return nil
}
`,
	})

	// When renaming orderID to id
	r := refactor.New()
	spec := analyzer.RefactorSpec{
		NewParams: []analyzer.Parameter{
			{Name: "ctx", Type: "context.Context"},
			{Name: "id", Type: "string"},
		},
		NewReturns: []analyzer.Parameter{
			{Type: "error"},
		},
	}

	edits, err := r.Refactor(filepath.Join(testDir, "main.go"), 25, spec)

	// Then parameter should be renamed
	if err != nil {
		t.Fatalf("Refactor failed: %v", err)
	}

	content := applyEdits(t, testDir, edits)

	if !strings.Contains(content["main.go"], "func ProcessOrder(ctx context.Context, id string)") {
		t.Errorf("parameter not renamed correctly:\n%s", content["main.go"])
	}

	// Also verify usage inside function body is renamed
	if !strings.Contains(content["main.go"], "fmt.Println(id)") {
		t.Errorf("parameter usage not renamed:\n%s", content["main.go"])
	}
}

func TestAddParameter(t *testing.T) {
	// Given a function
	testDir := setupTestDir(t, map[string]string{
		"main.go": `package main

func ProcessOrder(ctx context.Context, orderID string) error {
	return nil
}

func caller() {
	_ = ProcessOrder(context.Background(), "123")
}
`,
	})

	// When adding a new parameter with default value
	r := refactor.New()
	spec := analyzer.RefactorSpec{
		NewParams: []analyzer.Parameter{
			{Name: "ctx", Type: "context.Context"},
			{Name: "orderID", Type: "string"},
			{Name: "priority", Type: "int"},
		},
		NewReturns: []analyzer.Parameter{
			{Type: "error"},
		},
		DefaultValues: map[string]string{
			"priority": "0",
		},
	}

	edits, err := r.Refactor(filepath.Join(testDir, "main.go"), 25, spec)

	// Then new parameter should be added
	if err != nil {
		t.Fatalf("Refactor failed: %v", err)
	}

	content := applyEdits(t, testDir, edits)

	if !strings.Contains(content["main.go"], "func ProcessOrder(ctx context.Context, orderID string, priority int)") {
		t.Errorf("parameter not added correctly:\n%s", content["main.go"])
	}

	// Call site should have default value
	if !strings.Contains(content["main.go"], `ProcessOrder(context.Background(), "123", 0)`) {
		t.Errorf("default value not added to call site:\n%s", content["main.go"])
	}
}

func TestAddParameterInMiddle_CallSiteCorrect(t *testing.T) {
	// This test mimics the exact bug scenario:
	// - ResolveForDelivery(ctx, products, hfWeek, marketCode)
	// - Insert isTest at index 2
	// - Expected: ResolveForDelivery(ctx, products, false, hfWeek, marketCode)
	testDir := setupTestDir(t, map[string]string{
		"resolver.go": `package main

import "context"

type Resolver struct{}

func (r *Resolver) ResolveForDelivery(
	ctx context.Context,
	products []string,
	hfWeek string,
	marketCode string,
) error {
	return nil
}
`,
		"caller.go": `package main

import "context"

func DigestOrder() {
	r := &Resolver{}
	err := r.ResolveForDelivery(ctx, query.Order.Products, query.HFWeek, query.MarketCode)
	_ = err
}

var ctx = context.Background()
var query = struct {
	Order struct{ Products []string }
	HFWeek string
	MarketCode string
}{}
`,
	})

	// Insert isTest bool at index 2
	r := refactor.New()
	spec := analyzer.RefactorSpec{
		NewParams: []analyzer.Parameter{
			{Name: "ctx", Type: "context.Context"},
			{Name: "products", Type: "[]string"},
			{Name: "isTest", Type: "bool"},       // NEW at index 2
			{Name: "hfWeek", Type: "string"},     // Was index 2, now index 3
			{Name: "marketCode", Type: "string"}, // Was index 3, now index 4
		},
		NewReturns: []analyzer.Parameter{
			{Type: "error"},
		},
		DefaultValues: map[string]string{
			"isTest": "false",
		},
	}

	edits, err := r.Refactor(filepath.Join(testDir, "resolver.go"), 60, spec)
	if err != nil {
		t.Fatalf("Refactor failed: %v", err)
	}

	content := applyEdits(t, testDir, edits)

	// Debug: print the result
	t.Logf("caller.go content:\n%s", content["caller.go"])

	// Verify call site is correct
	expectedCall := `r.ResolveForDelivery(ctx, query.Order.Products, false, query.HFWeek, query.MarketCode)`
	if !strings.Contains(content["caller.go"], expectedCall) {
		t.Errorf("call site not updated correctly.\nExpected to contain: %s\nGot:\n%s", expectedCall, content["caller.go"])
	}

	// Verify signature is correct (parameters in single line after refactoring)
	if !strings.Contains(content["resolver.go"], "ResolveForDelivery(ctx context.Context, products []string, isTest bool, hfWeek string, marketCode string)") {
		t.Errorf("signature not updated correctly:\n%s", content["resolver.go"])
	}
}

func TestAddParameterInMiddle_MultilineCallSite(t *testing.T) {
	// Test with multiline call site arguments (like the real bug scenario)
	testDir := setupTestDir(t, map[string]string{
		"resolver.go": `package main

import "context"

type Resolver struct{}

func (r *Resolver) ResolveForDelivery(
	ctx context.Context,
	products []string,
	hfWeek string,
	marketCode string,
) error {
	return nil
}
`,
		"caller.go": `package main

import "context"

type service struct {
	resolver *Resolver
}

func (s service) DigestOrder(ctx context.Context) error {
	err := s.resolver.ResolveForDelivery(ctx, query.Order.Products, query.HFWeek, query.MarketCode)
	return err
}

var query = struct {
	Order struct{ Products []string }
	HFWeek string
	MarketCode string
}{}
`,
	})

	// Insert isTest bool at index 2
	r := refactor.New()
	spec := analyzer.RefactorSpec{
		NewParams: []analyzer.Parameter{
			{Name: "ctx", Type: "context.Context"},
			{Name: "products", Type: "[]string"},
			{Name: "isTest", Type: "bool"},       // NEW at index 2
			{Name: "hfWeek", Type: "string"},     // Was index 2, now index 3
			{Name: "marketCode", Type: "string"}, // Was index 3, now index 4
		},
		NewReturns: []analyzer.Parameter{
			{Type: "error"},
		},
		DefaultValues: map[string]string{
			"isTest": "false",
		},
	}

	edits, err := r.Refactor(filepath.Join(testDir, "resolver.go"), 60, spec)
	if err != nil {
		t.Fatalf("Refactor failed: %v", err)
	}

	content := applyEdits(t, testDir, edits)

	// Debug: print the result
	t.Logf("caller.go content:\n%s", content["caller.go"])

	// Verify call site is correct
	expectedCall := `s.resolver.ResolveForDelivery(ctx, query.Order.Products, false, query.HFWeek, query.MarketCode)`
	if !strings.Contains(content["caller.go"], expectedCall) {
		t.Errorf("call site not updated correctly.\nExpected to contain: %s\nGot:\n%s", expectedCall, content["caller.go"])
	}
}

func TestDeduplicateEdits_PreventsDuplicateCallSiteEdits(t *testing.T) {
	// This test ensures that when the same call site appears in multiple packages
	// (e.g., main package and test package due to Tests: true), we don't generate
	// duplicate edits that would corrupt the file.
	//
	// The bug manifested as: "MarketCodeketCode" instead of "MarketCode"
	// because the same edit was applied twice at the same offset.

	testDir := setupTestDir(t, map[string]string{
		"resolver.go": `package main

import "context"

type Resolver struct{}

func (r *Resolver) Process(ctx context.Context, data string, code string) error {
	return nil
}
`,
		"caller.go": `package main

import "context"

func UseResolver() {
	r := &Resolver{}
	_ = r.Process(ctx, data, code)
}

var ctx = context.Background()
var data = "test"
var code = "US"
`,
		// Test file in same package - this causes Go to load caller.go in multiple packages
		"caller_test.go": `package main

import "testing"

func TestUseResolver(t *testing.T) {
	UseResolver()
}
`,
	})

	// Insert isTest bool at index 2
	r := refactor.New()
	spec := analyzer.RefactorSpec{
		NewParams: []analyzer.Parameter{
			{Name: "ctx", Type: "context.Context"},
			{Name: "data", Type: "string"},
			{Name: "isTest", Type: "bool"}, // NEW at index 2
			{Name: "code", Type: "string"}, // Was index 2, now index 3
		},
		NewReturns: []analyzer.Parameter{
			{Type: "error"},
		},
		DefaultValues: map[string]string{
			"isTest": "false",
		},
	}

	edits, err := r.Refactor(filepath.Join(testDir, "resolver.go"), 60, spec)
	if err != nil {
		t.Fatalf("Refactor failed: %v", err)
	}

	// Verify no duplicate edits for caller.go
	callerEdits := edits.Changes[filepath.Join(testDir, "caller.go")]
	if len(callerEdits) != 1 {
		t.Errorf("expected exactly 1 edit for caller.go, got %d (duplicates not removed!)", len(callerEdits))
		for i, edit := range callerEdits {
			t.Logf("  Edit %d: [%d-%d]", i+1, edit.Range.Start.Offset, edit.Range.End.Offset)
		}
	}

	// Apply edits and verify no corruption
	content := applyEdits(t, testDir, edits)

	// The key check: "code" should appear exactly once, not corrupted like "codecode"
	expectedCall := `r.Process(ctx, data, false, code)`
	if !strings.Contains(content["caller.go"], expectedCall) {
		t.Errorf("call site corrupted or not updated correctly.\nExpected: %s\nGot:\n%s", expectedCall, content["caller.go"])
	}

	// Also verify no duplicate text patterns that would indicate corruption
	if strings.Contains(content["caller.go"], "codecode") {
		t.Error("detected corruption pattern 'codecode' - duplicate edits were applied!")
	}
}

func TestAddParameterInMiddle_BodyUsagesPreserved(t *testing.T) {
	// Given a function that uses its parameters in the body
	testDir := setupTestDir(t, map[string]string{
		"main.go": `package main

import "fmt"

func ResolveData(ctx context.Context, products []string, hfWeek string, marketCode string) error {
	fmt.Println(hfWeek)
	fmt.Println(marketCode)
	return nil
}

func caller() {
	_ = ResolveData(context.Background(), []string{"a"}, "2024-W01", "US")
}
`,
	})

	// When adding a new parameter (isTest) at the hfWeek position
	// This shifts hfWeek and marketCode to new positions
	r := refactor.New()
	spec := analyzer.RefactorSpec{
		NewParams: []analyzer.Parameter{
			{Name: "ctx", Type: "context.Context"},
			{Name: "products", Type: "[]string"},
			{Name: "isTest", Type: "bool"},      // NEW parameter inserted here
			{Name: "hfWeek", Type: "string"},    // Was at index 2, now at index 3
			{Name: "marketCode", Type: "string"}, // Was at index 3, now at index 4
		},
		NewReturns: []analyzer.Parameter{
			{Type: "error"},
		},
		DefaultValues: map[string]string{
			"isTest": "false",
		},
	}

	edits, err := r.Refactor(filepath.Join(testDir, "main.go"), 40, spec)

	// Then the function body should still use hfWeek and marketCode correctly
	// NOT have them replaced with isTest
	if err != nil {
		t.Fatalf("Refactor failed: %v", err)
	}

	content := applyEdits(t, testDir, edits)

	// Verify signature is correct
	if !strings.Contains(content["main.go"], "func ResolveData(ctx context.Context, products []string, isTest bool, hfWeek string, marketCode string)") {
		t.Errorf("signature not updated correctly:\n%s", content["main.go"])
	}

	// CRITICAL: Verify hfWeek is still used in body (not replaced with isTest)
	if !strings.Contains(content["main.go"], "fmt.Println(hfWeek)") {
		t.Errorf("hfWeek usage incorrectly renamed in body:\n%s", content["main.go"])
	}

	// CRITICAL: Verify marketCode is still used in body (not replaced with hfWeek)
	if !strings.Contains(content["main.go"], "fmt.Println(marketCode)") {
		t.Errorf("marketCode usage incorrectly renamed in body:\n%s", content["main.go"])
	}

	// Call site should have default value for new param
	if !strings.Contains(content["main.go"], `ResolveData(context.Background(), []string{"a"}, false, "2024-W01", "US")`) {
		t.Errorf("call site not updated correctly:\n%s", content["main.go"])
	}
}

func TestRemoveParameter(t *testing.T) {
	// Given a function with multiple parameters
	testDir := setupTestDir(t, map[string]string{
		"main.go": `package main

func ProcessOrder(ctx context.Context, orderID string, amount int) error {
	return nil
}

func caller() {
	_ = ProcessOrder(context.Background(), "123", 100)
}
`,
	})

	// When removing amount parameter
	r := refactor.New()
	spec := analyzer.RefactorSpec{
		NewParams: []analyzer.Parameter{
			{Name: "ctx", Type: "context.Context"},
			{Name: "orderID", Type: "string"},
		},
		NewReturns: []analyzer.Parameter{
			{Type: "error"},
		},
	}

	edits, err := r.Refactor(filepath.Join(testDir, "main.go"), 25, spec)

	// Then parameter should be removed
	if err != nil {
		t.Fatalf("Refactor failed: %v", err)
	}

	content := applyEdits(t, testDir, edits)

	if !strings.Contains(content["main.go"], "func ProcessOrder(ctx context.Context, orderID string) error") {
		t.Errorf("parameter not removed correctly:\n%s", content["main.go"])
	}

	// Call site should have argument removed
	if !strings.Contains(content["main.go"], `ProcessOrder(context.Background(), "123")`) {
		t.Errorf("argument not removed from call site:\n%s", content["main.go"])
	}
}

func TestInterfaceRefactoring_AddTwoParameters_AllCallSites(t *testing.T) {
	// Given an interface with implementations and call sites on BOTH interface and concrete types
	testDir := setupTestDir(t, map[string]string{
		"service.go": `package main

type OrderService interface {
	CreateOrder(ctx context.Context, customerID string) (string, error)
}

type InMemoryService struct{}

func (s *InMemoryService) CreateOrder(ctx context.Context, customerID string) (string, error) {
	return "order-1", nil
}

func UseViaInterface(svc OrderService) {
	_, _ = svc.CreateOrder(context.Background(), "cust-1")
}

func UseViaConcrete() {
	svc := &InMemoryService{}
	_, _ = svc.CreateOrder(context.Background(), "cust-2")
}
`,
	})

	// When adding TWO parameters to interface method
	r := refactor.New()
	spec := analyzer.RefactorSpec{
		NewParams: []analyzer.Parameter{
			{Name: "ctx", Type: "context.Context"},
			{Name: "customerID", Type: "string"},
			{Name: "isTest", Type: "bool"},
			{Name: "priority", Type: "int"},
		},
		NewReturns: []analyzer.Parameter{
			{Type: "string"},
			{Type: "error"},
		},
		DefaultValues: map[string]string{
			"isTest":   "false",
			"priority": "0",
		},
	}

	edits, err := r.Refactor(filepath.Join(testDir, "service.go"), 60, spec)

	// Then interface, implementation, and ALL call sites should update
	if err != nil {
		t.Fatalf("Refactor failed: %v", err)
	}

	content := applyEdits(t, testDir, edits)

	// Interface should be updated
	if !strings.Contains(content["service.go"], "CreateOrder(ctx context.Context, customerID string, isTest bool, priority int)") {
		t.Errorf("interface not updated:\n%s", content["service.go"])
	}

	// Implementation should be updated
	if !strings.Contains(content["service.go"], "func (s *InMemoryService) CreateOrder(ctx context.Context, customerID string, isTest bool, priority int)") {
		t.Errorf("implementation not updated:\n%s", content["service.go"])
	}

	// Call site via interface should have default values
	if !strings.Contains(content["service.go"], `svc.CreateOrder(context.Background(), "cust-1", false, 0)`) {
		t.Errorf("call site via interface not updated:\n%s", content["service.go"])
	}

	// Call site via concrete type should ALSO have default values
	if !strings.Contains(content["service.go"], `svc.CreateOrder(context.Background(), "cust-2", false, 0)`) {
		t.Errorf("call site via concrete type not updated:\n%s", content["service.go"])
	}
}

func TestInterfaceRefactoring(t *testing.T) {
	// Given an interface with implementations
	testDir := setupTestDir(t, map[string]string{
		"service.go": `package main

type OrderService interface {
	CreateOrder(ctx context.Context, customerID string) (string, error)
}

type InMemoryService struct{}

func (s *InMemoryService) CreateOrder(ctx context.Context, customerID string) (string, error) {
	return "order-1", nil
}

func UseService(svc OrderService) {
	_, _ = svc.CreateOrder(context.Background(), "cust-1")
}
`,
	})

	// When adding a parameter to interface method
	r := refactor.New()
	spec := analyzer.RefactorSpec{
		NewParams: []analyzer.Parameter{
			{Name: "ctx", Type: "context.Context"},
			{Name: "customerID", Type: "string"},
			{Name: "items", Type: "[]string"},
		},
		NewReturns: []analyzer.Parameter{
			{Type: "string"},
			{Type: "error"},
		},
		DefaultValues: map[string]string{
			"items": "nil",
		},
	}

	edits, err := r.Refactor(filepath.Join(testDir, "service.go"), 60, spec)

	// Then interface, implementation, and call sites should all update
	if err != nil {
		t.Fatalf("Refactor failed: %v", err)
	}

	content := applyEdits(t, testDir, edits)

	// Interface should be updated
	if !strings.Contains(content["service.go"], "CreateOrder(ctx context.Context, customerID string, items []string)") {
		t.Errorf("interface not updated:\n%s", content["service.go"])
	}

	// Implementation should be updated
	if !strings.Contains(content["service.go"], "func (s *InMemoryService) CreateOrder(ctx context.Context, customerID string, items []string)") {
		t.Errorf("implementation not updated:\n%s", content["service.go"])
	}

	// Call site should have default value
	if !strings.Contains(content["service.go"], `svc.CreateOrder(context.Background(), "cust-1", nil)`) {
		t.Errorf("call site not updated:\n%s", content["service.go"])
	}
}

// Helper functions

func setupTestDir(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()

	// Write go.mod
	goMod := "module testpkg\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
	}

	return dir
}

func applyEdits(t *testing.T, baseDir string, edits *analyzer.WorkspaceEdit) map[string]string {
	t.Helper()

	result := make(map[string]string)

	for filename, fileEdits := range edits.Changes {
		content, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("failed to read %s: %v", filename, err)
		}

		// Filter out overlapping edits (keep only non-overlapping ones)
		// Edits are already sorted by offset descending
		var filteredEdits []analyzer.TextEdit
		lastStart := len(content) + 1
		for _, edit := range fileEdits {
			end := edit.Range.End.Offset
			if end <= lastStart {
				filteredEdits = append(filteredEdits, edit)
				lastStart = edit.Range.Start.Offset
			}
		}

		// Apply edits (already in reverse order)
		for _, edit := range filteredEdits {
			start := edit.Range.Start.Offset
			end := edit.Range.End.Offset
			if start < 0 || end > len(content) || start > end {
				t.Logf("skipping invalid edit: start=%d end=%d len=%d", start, end, len(content))
				continue
			}
			content = append(content[:start], append([]byte(edit.NewText), content[end:]...)...)
		}

		relPath, err := filepath.Rel(baseDir, filename)
		if err != nil {
			relPath = filename
		}
		result[relPath] = string(content)
	}

	return result
}
