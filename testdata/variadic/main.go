package variadic

import "fmt"

// PrintAll prints all given values
func PrintAll(prefix string, values ...interface{}) {
	for _, v := range values {
		fmt.Println(prefix, v)
	}
}

func caller() {
	PrintAll("item:", 1, 2, 3)
	PrintAll("name:", "alice", "bob")
}
