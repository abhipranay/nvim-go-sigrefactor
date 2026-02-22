package generics

// Map applies a function to each element
func Map[T, U any](items []T, fn func(T) U) []U {
	result := make([]U, len(items))
	for i, item := range items {
		result[i] = fn(item)
	}
	return result
}

func caller() {
	nums := []int{1, 2, 3}
	doubled := Map(nums, func(n int) int { return n * 2 })
	_ = doubled
}
