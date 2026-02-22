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
