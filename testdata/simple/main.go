package simple

import "context"

// ProcessOrder handles order processing
func ProcessOrder(ctx context.Context, orderID string, amount int) error {
	return nil
}

func callProcessOrder() {
	_ = ProcessOrder(context.Background(), "123", 100)
	_ = ProcessOrder(context.TODO(), "456", 200)
}
