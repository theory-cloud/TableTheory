// Package examples demonstrates TableTheory's embedded struct support
package examples

import (
	"fmt"
	"log"
	"time"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

// BaseModel represents common fields for a single-table design pattern
// This struct can be embedded in other models to share common fields
type BaseModel struct {
	CreatedAt time.Time `theorydb:"created_at"`
	UpdatedAt time.Time `theorydb:"updated_at"`
	PK        string    `theorydb:"pk"`
	SK        string    `theorydb:"sk"`
	GSI1PK    string    `theorydb:"index:gsi1,pk"`
	GSI1SK    string    `theorydb:"index:gsi1,sk"`
	GSI2PK    string    `theorydb:"index:gsi2,pk"`
	GSI2SK    string    `theorydb:"index:gsi2,sk"`
	Type      string    `theorydb:"attr:type"`
	TenantID  string    `theorydb:"attr:tenantId"`
	Version   int       `theorydb:"version"`
}

// EmbeddedCustomer demonstrates embedding BaseModel for a customer entity
type EmbeddedCustomer struct {
	ID    string `theorydb:"attr:id"`
	Email string `theorydb:"attr:email"`
	Name  string `theorydb:"attr:name"`
	Phone string `theorydb:"attr:phone"`
	BaseModel
}

// TableName returns the DynamoDB table name
func (c *EmbeddedCustomer) TableName() string {
	return "my-application"
}

// Product demonstrates the same pattern for a different entity type
type EmbeddedProduct struct {
	ID          string `theorydb:"attr:id"`
	Name        string `theorydb:"attr:name"`
	Description string `theorydb:"attr:description"`
	CategoryID  string `theorydb:"attr:categoryId"`
	BaseModel
	Price float64 `theorydb:"attr:price"`
	Stock int     `theorydb:"attr:stock"`
}

// TableName returns the DynamoDB table name
func (p *EmbeddedProduct) TableName() string {
	return "my-application"
}

// Order shows how to use embedded structs with relationships
type EmbeddedOrder struct {
	OrderedAt  time.Time `theorydb:"attr:orderedAt"`
	ID         string    `theorydb:"attr:id"`
	CustomerID string    `theorydb:"attr:customerId"`
	Status     string    `theorydb:"attr:status"`
	BaseModel
	Total float64 `theorydb:"attr:total"`
}

// TableName returns the DynamoDB table name
func (o *EmbeddedOrder) TableName() string {
	return "my-application"
}

// ExampleEmbeddedStructs demonstrates how to use embedded structs with TableTheory
func ExampleEmbeddedStructs() {
	// Initialize TableTheory
	config := session.Config{
		Region: "us-east-1",
	}

	db, err := theorydb.New(config)
	if err != nil {
		log.Fatal(err)
	}

	// Create a customer with the embedded BaseModel fields
	customer := &EmbeddedCustomer{
		BaseModel: BaseModel{
			PK:       "CUSTOMER#cus_123",
			SK:       "METADATA",
			GSI1PK:   "TENANT#tenant_456",
			GSI1SK:   "CUSTOMER#cus_123",
			GSI2PK:   "EMAIL#john@example.com",
			GSI2SK:   "CUSTOMER#cus_123",
			Type:     "customer",
			TenantID: "tenant_456",
			// CreatedAt and UpdatedAt are set automatically
		},
		ID:    "cus_123",
		Email: "john@example.com",
		Name:  "John Doe",
		Phone: "+1234567890",
	}

	// Create the customer
	if err := db.Model(customer).Create(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Customer created at: %v\n", customer.CreatedAt)

	// Query customers by tenant using GSI1
	var customers []EmbeddedCustomer
	err = db.Model(&EmbeddedCustomer{}).
		Index("gsi1").
		Where("GSI1PK", "=", "TENANT#tenant_456").
		Where("GSI1SK", "BEGINS_WITH", "CUSTOMER#").
		All(&customers)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Found %d customers for tenant\n", len(customers))

	// Create a product using the same base model pattern
	product := &EmbeddedProduct{
		BaseModel: BaseModel{
			PK:       "PRODUCT#prod_789",
			SK:       "METADATA",
			GSI1PK:   "TENANT#tenant_456",
			GSI1SK:   "PRODUCT#prod_789",
			GSI2PK:   "CATEGORY#electronics",
			GSI2SK:   "PRODUCT#prod_789",
			Type:     "product",
			TenantID: "tenant_456",
		},
		ID:          "prod_789",
		Name:        "Laptop",
		Description: "High-performance laptop",
		Price:       999.99,
		Stock:       50,
		CategoryID:  "electronics",
	}

	if err := db.Model(product).Create(); err != nil {
		log.Fatal(err)
	}

	// Create an order linking customer and product
	order := &EmbeddedOrder{
		BaseModel: BaseModel{
			PK:       "ORDER#ord_555",
			SK:       "METADATA",
			GSI1PK:   "CUSTOMER#cus_123",
			GSI1SK:   fmt.Sprintf("ORDER#%d", time.Now().Unix()),
			GSI2PK:   "TENANT#tenant_456",
			GSI2SK:   fmt.Sprintf("ORDER#%d#ord_555", time.Now().Unix()),
			Type:     "order",
			TenantID: "tenant_456",
		},
		ID:         "ord_555",
		CustomerID: "cus_123",
		Total:      999.99,
		Status:     "pending",
		OrderedAt:  time.Now(),
	}

	if err := db.Model(order).Create(); err != nil {
		log.Fatal(err)
	}

	// Query all orders for a customer using GSI1
	var orders []EmbeddedOrder
	err = db.Model(&EmbeddedOrder{}).
		Index("gsi1").
		Where("GSI1PK", "=", "CUSTOMER#cus_123").
		Where("GSI1SK", "BEGINS_WITH", "ORDER#").
		OrderBy("GSI1SK", "DESC").
		All(&orders)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Customer has %d orders\n", len(orders))

	// Update customer email (which requires updating GSI2PK)
	customer.Email = "johndoe@example.com"
	customer.GSI2PK = "EMAIL#johndoe@example.com"

	if err := db.Model(customer).Update("Email", "GSI2PK"); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Customer email updated, UpdatedAt: %v\n", customer.UpdatedAt)
}

// Benefits of embedded structs:
// 1. DRY (Don't Repeat Yourself) - Common fields defined once
// 2. Consistent key structure across all entities
// 3. Easy to maintain and refactor
// 4. Type safety for common operations
// 5. Perfect for single-table design patterns
