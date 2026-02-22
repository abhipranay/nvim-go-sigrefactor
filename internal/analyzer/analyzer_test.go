package analyzer_test

import (
	"path/filepath"
	"testing"

	"github.com/hellofresh/nvim-go-sigrefactor/internal/analyzer"
)

func TestAnalyzeSignature_SimpleFunction(t *testing.T) {
	// Given a simple function
	testFile := filepath.Join("..", "..", "testdata", "simple", "main.go")
	absPath, err := filepath.Abs(testFile)
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	// When analyzing at the function declaration (offset points to "ProcessOrder")
	// Line 6: "func ProcessOrder(ctx context.Context, orderID string, amount int) error {"
	// Offset to "ProcessOrder" starts at 0x50 = 80 decimal
	a := analyzer.New()
	result, err := a.Analyze(absPath, 85)

	// Then we should get the correct signature
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	sig := result.Signature
	if sig.Name != "ProcessOrder" {
		t.Errorf("expected name ProcessOrder, got %s", sig.Name)
	}

	if sig.Receiver != nil {
		t.Errorf("expected no receiver for function, got %+v", sig.Receiver)
	}

	if len(sig.Params) != 3 {
		t.Fatalf("expected 3 params, got %d", len(sig.Params))
	}

	// Check parameters
	expectedParams := []struct {
		name string
		typ  string
	}{
		{"ctx", "context.Context"},
		{"orderID", "string"},
		{"amount", "int"},
	}

	for i, exp := range expectedParams {
		if sig.Params[i].Name != exp.name {
			t.Errorf("param %d: expected name %s, got %s", i, exp.name, sig.Params[i].Name)
		}
		if sig.Params[i].Type != exp.typ {
			t.Errorf("param %d: expected type %s, got %s", i, exp.typ, sig.Params[i].Type)
		}
	}

	// Check returns
	if len(sig.Returns) != 1 {
		t.Fatalf("expected 1 return, got %d", len(sig.Returns))
	}
	if sig.Returns[0].Type != "error" {
		t.Errorf("expected return type error, got %s", sig.Returns[0].Type)
	}
}

func TestAnalyzeSignature_Method(t *testing.T) {
	// Given a method on a struct
	testFile := filepath.Join("..", "..", "testdata", "interface", "service.go")
	absPath, err := filepath.Abs(testFile)
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	// When analyzing at the InMemoryOrderService.CreateOrder method
	// Line 25: "func (s *InMemoryOrderService) CreateOrder(..."
	a := analyzer.New()
	result, err := a.Analyze(absPath, 580)

	// Then we should get the correct signature with receiver
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	sig := result.Signature
	if sig.Name != "CreateOrder" {
		t.Errorf("expected name CreateOrder, got %s", sig.Name)
	}

	if sig.Receiver == nil {
		t.Fatal("expected receiver for method")
	}

	if sig.Receiver.Name != "s" {
		t.Errorf("expected receiver name 's', got %s", sig.Receiver.Name)
	}

	if sig.Receiver.Type != "InMemoryOrderService" {
		t.Errorf("expected receiver type InMemoryOrderService, got %s", sig.Receiver.Type)
	}

	if !sig.Receiver.Pointer {
		t.Error("expected pointer receiver")
	}
}

func TestAnalyzeSignature_InterfaceMethod(t *testing.T) {
	// Given an interface method
	testFile := filepath.Join("..", "..", "testdata", "interface", "service.go")
	absPath, err := filepath.Abs(testFile)
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	// When analyzing at the interface method CreateOrder
	// Line 7: "CreateOrder(ctx context.Context, customerID string, items []string) (string, error)"
	a := analyzer.New()
	result, err := a.Analyze(absPath, 130)

	// Then we should recognize it as an interface method
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	sig := result.Signature
	if !sig.IsInterface {
		t.Error("expected IsInterface to be true")
	}

	if sig.InterfaceName != "OrderService" {
		t.Errorf("expected interface name OrderService, got %s", sig.InterfaceName)
	}
}

func TestFindUsages_SimpleFunction(t *testing.T) {
	// Given a function with call sites
	testFile := filepath.Join("..", "..", "testdata", "simple", "main.go")
	absPath, err := filepath.Abs(testFile)
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	// When finding usages
	a := analyzer.New()
	result, err := a.Analyze(absPath, 85)

	// Then we should find all call sites
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// We expect 2 usages in callProcessOrder function
	if len(result.Usages) != 2 {
		t.Errorf("expected 2 usages, got %d", len(result.Usages))
	}

	for _, usage := range result.Usages {
		if usage.Kind != "call" {
			t.Errorf("expected kind 'call', got %s", usage.Kind)
		}
		if usage.InFunc != "callProcessOrder" {
			t.Errorf("expected inFunc 'callProcessOrder', got %s", usage.InFunc)
		}
	}
}

func TestFindImplementations_InterfaceMethod(t *testing.T) {
	// Given an interface method
	testFile := filepath.Join("..", "..", "testdata", "interface", "service.go")
	absPath, err := filepath.Abs(testFile)
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	// When analyzing an interface method
	a := analyzer.New()
	result, err := a.Analyze(absPath, 130) // CreateOrder in interface

	// Then we should find all implementations
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// We expect 2 implementations: InMemoryOrderService and DBOrderService
	if len(result.Implementations) != 2 {
		t.Fatalf("expected 2 implementations, got %d", len(result.Implementations))
	}

	implTypes := make(map[string]bool)
	for _, impl := range result.Implementations {
		implTypes[impl.TypeName] = true
	}

	if !implTypes["InMemoryOrderService"] {
		t.Error("expected InMemoryOrderService implementation")
	}
	if !implTypes["DBOrderService"] {
		t.Error("expected DBOrderService implementation")
	}
}
