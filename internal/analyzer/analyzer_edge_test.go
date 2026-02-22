package analyzer_test

import (
	"path/filepath"
	"testing"

	"github.com/hellofresh/nvim-go-sigrefactor/internal/analyzer"
)

func TestAnalyzeSignature_VariadicFunction(t *testing.T) {
	testFile := filepath.Join("..", "..", "testdata", "variadic", "main.go")
	absPath, err := filepath.Abs(testFile)
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	a := analyzer.New()
	result, err := a.Analyze(absPath, 70) // Offset to PrintAll

	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	sig := result.Signature
	if sig.Name != "PrintAll" {
		t.Errorf("expected name PrintAll, got %s", sig.Name)
	}

	if len(sig.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(sig.Params))
	}

	// Check variadic parameter
	if !sig.Params[1].Variadic {
		t.Error("expected second param to be variadic")
	}

	if sig.Params[1].Type != "...interface{}" {
		t.Errorf("expected type ...interface{}, got %s", sig.Params[1].Type)
	}
}

func TestAnalyzeSignature_GenericFunction(t *testing.T) {
	testFile := filepath.Join("..", "..", "testdata", "generics", "main.go")
	absPath, err := filepath.Abs(testFile)
	if err != nil {
		t.Fatalf("failed to get absolute path: %v", err)
	}

	a := analyzer.New()
	result, err := a.Analyze(absPath, 65) // Offset to Map

	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	sig := result.Signature
	if sig.Name != "Map" {
		t.Errorf("expected name Map, got %s", sig.Name)
	}

	if len(sig.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(sig.Params))
	}

	// Check generic params
	if sig.Params[0].Type != "[]T" {
		t.Errorf("expected type []T, got %s", sig.Params[0].Type)
	}

	if sig.Params[1].Type != "func(T) U" {
		t.Errorf("expected type func(T) U, got %s", sig.Params[1].Type)
	}

	// Check return
	if len(sig.Returns) != 1 {
		t.Fatalf("expected 1 return, got %d", len(sig.Returns))
	}

	if sig.Returns[0].Type != "[]U" {
		t.Errorf("expected return type []U, got %s", sig.Returns[0].Type)
	}
}
