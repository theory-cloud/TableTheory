package models

import "time"

// TestUser is a test model for user data
type TestUser struct {
	CreatedAt time.Time `theorydb:"index:gsi-email,sk"`
	ID        string    `theorydb:"pk"`
	Email     string    `theorydb:"sk,index:gsi-email,pk"`
	Status    string    `theorydb:""`
	Name      string    `theorydb:""`
	Tags      []string  `theorydb:""`
	Age       int       `theorydb:""`
}

// TestProduct is a test model for product data
type TestProduct struct {
	CreatedAt   time.Time `theorydb:""`
	SKU         string    `theorydb:"pk"`
	Category    string    `theorydb:"sk,index:gsi-category,pk"`
	Name        string    `theorydb:""`
	Description string    `theorydb:""`
	Price       float64   `theorydb:"index:gsi-category,sk"`
	InStock     bool      `theorydb:""`
}

// TestOrder is a test model for complex queries
type TestOrder struct {
	CreatedAt  time.Time   `theorydb:"index:gsi-customer,sk"`
	UpdatedAt  time.Time   `theorydb:""`
	OrderID    string      `theorydb:"pk"`
	CustomerID string      `theorydb:"sk,index:gsi-customer,pk"`
	Status     string      `theorydb:"index:gsi-status,pk"`
	Items      []OrderItem `theorydb:""`
	Total      float64     `theorydb:"index:gsi-status,sk"`
}

// OrderItem represents an item in an order
type OrderItem struct {
	ProductSKU string  `theorydb:""`
	Quantity   int     `theorydb:""`
	Price      float64 `theorydb:""`
}
