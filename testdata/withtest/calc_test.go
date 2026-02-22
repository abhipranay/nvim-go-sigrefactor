package withtest

import "testing"

func TestAdd(t *testing.T) {
	result := Add(1, 2)
	if result != 3 {
		t.Errorf("expected 3, got %d", result)
	}
}

func TestAddNegative(t *testing.T) {
	result := Add(-1, -2)
	if result != -3 {
		t.Errorf("expected -3, got %d", result)
	}
}
