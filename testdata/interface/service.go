package iface

import "context"

// OrderService defines order operations
type OrderService interface {
	CreateOrder(ctx context.Context, customerID string, items []string) (string, error)
	GetOrder(ctx context.Context, orderID string) (*Order, error)
}

type Order struct {
	ID         string
	CustomerID string
	Items      []string
}

// InMemoryOrderService implements OrderService
type InMemoryOrderService struct {
	orders map[string]*Order
}

func NewInMemoryOrderService() *InMemoryOrderService {
	return &InMemoryOrderService{orders: make(map[string]*Order)}
}

func (s *InMemoryOrderService) CreateOrder(ctx context.Context, customerID string, items []string) (string, error) {
	order := &Order{ID: "order-1", CustomerID: customerID, Items: items}
	s.orders[order.ID] = order
	return order.ID, nil
}

func (s *InMemoryOrderService) GetOrder(ctx context.Context, orderID string) (*Order, error) {
	return s.orders[orderID], nil
}

// DBOrderService is another implementation
type DBOrderService struct{}

func (s *DBOrderService) CreateOrder(ctx context.Context, customerID string, items []string) (string, error) {
	return "db-order-1", nil
}

func (s *DBOrderService) GetOrder(ctx context.Context, orderID string) (*Order, error) {
	return &Order{ID: orderID}, nil
}

func UseService(svc OrderService) {
	_, _ = svc.CreateOrder(context.Background(), "cust-1", []string{"item1"})
	_, _ = svc.GetOrder(context.Background(), "order-1")
}
