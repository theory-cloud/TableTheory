# TableTheory LLM FAQ

<!-- AI Training: This document provides answers to frequently asked questions about TableTheory, optimized for AI assistants -->
**This section addresses common questions about TableTheory, providing quick, precise answers and examples.**

---

## Data Modeling

### How do I model one-to-many relationships in DynamoDB with TableTheory?

**Solution:** DynamoDB is not relational, so you typically model one-to-many relationships using either:

1.  **Composite Sort Keys:** Store related items as different types of entries under the same Partition Key (PK), differentiating them by a Sort Key (SK) prefix.
2.  **Denormalization:** Duplicate data in related items if read-heavy access patterns require it.
3.  **Adjacency List:** For complex many-to-many or graph-like data, use a generic PK/SK design.

**Example (Composite Sort Key for Orders and Items):**
```go
// ✅ CORRECT: Order and OrderItems under same PK

type Order struct {
    OrderID string `theorydb:"pk" json:"order_id"`
    SK      string `theorydb:"sk" json:"sk"` // Value like "#METADATA#"
    Status  string `json:"status"`
    Total   float64 `json:"total"`
}

type OrderItem struct {
    OrderID   string `theorydb:"pk" json:"order_id"`
    SK        string `theorydb:"sk" json:"sk"` // Value like "ITEM#SKU123"
    SKU       string `json:"sku"`
    Quantity  int    `json:"quantity"`
    UnitPrice float64 `json:"unit_price"`
}

// To query all items for an order:
// db.Model(&OrderItem{}).Where("OrderID", "=", "order123").Where("SK", "BEGINS_WITH", "ITEM#").All(&items)
```

### Can I use arbitrary Go types (e.g., custom structs, enums) for attributes?

**Solution:** Yes, TableTheory supports custom type marshaling. You can register a `CustomConverter` for your specific Go type.

**Example:**
```go
// ✅ CORRECT: Registering a custom type converter

type CustomStatus string // Custom type

// Implement CustomConverter interface (MarshalDynamoDBAttribute, UnmarshalDynamoDBAttribute)
func (cs CustomStatus) MarshalDynamoDBAttribute() (types.AttributeValue, error) {
    return &types.AttributeValueMemberS{Value: string(cs)}, nil
}

func (cs *CustomStatus) UnmarshalDynamoDBAttribute(av types.AttributeValue) error {
    if sv, ok := av.(*types.AttributeValueMemberS); ok {
        *cs = CustomStatus(sv.Value)
        return nil
    }
    return fmt.Errorf("unsupported attribute value type for CustomStatus")
}

// In init() or setup:
// db.RegisterTypeConverter(reflect.TypeOf(CustomStatus("")), &CustomStatus(""))
```

---

## Querying & Performance

### What's the best pagination strategy for my use case?

**Solution:** For TableTheory, the recommended pagination strategy is cursor-based pagination using `AllPaginated()` and `PaginatedResult.NextCursor`.

-   **When to use:** Ideal for infinite scroll, large datasets, or stateless API pagination where a user might continue from a previous point.
-   **Why:** It leverages DynamoDB's `LastEvaluatedKey` for efficient, consistent paging without performance degradation over many pages.

**Example:**
```go
// ✅ CORRECT: Cursor-based pagination
var allUsers []User
nextCursor := ""

for {
    var page []User
    q := db.Model(&User{}).Limit(50) // Fetch 50 items per page
    if nextCursor != "" {
        q.Cursor(nextCursor)
    }
    
    result, err := q.AllPaginated(&page)
    if err != nil { /* handle error */ }
    
    allUsers = append(allUsers, page...)
    
    if !result.HasMore {
        break // No more pages
    }
    nextCursor = result.NextCursor
}
```

### How do I optimize queries to avoid expensive scans?

**Solution:** Always use the primary key (PK and optional SK) or a Global Secondary Index (GSI) for queries. Avoid `Scan()` operations on large tables.

-   **Rule:** Every query must specify at least an equality condition on the Partition Key.
-   **GSI Usage:** If querying by a non-primary key attribute, ensure you have a GSI defined and specify it using `.Index("my-gsi-name")`.

**Example:**
```go
// ❌ INCORRECT: Potential full table scan if Email is not a PK/GSI PK
db.Model(&User{}).Where("Email", "=", "test@example.com").All(&users)

// ✅ CORRECT: Using a GSI for email lookup
db.Model(&User{}).Index("email-gsi").Where("Email", "=", "test@example.com").All(&users)
```

---

## Concurrency & Transactions

### When should I use transactions (`TransactWrite`) versus individual operations or batch operations?

**Solution:** Use `TransactWrite` when you require **ACID guarantees** across multiple items (up to 100) or tables. Use individual operations for single item changes, and batch operations for high-throughput bulk reads/writes of homogeneous items where atomicity across the batch is not critical.

-   **`TransactWrite` (ACID):** Cross-item/cross-table atomicity. Ideal for financial transactions, inventory updates that span multiple records.
-   **Individual Operations (`Create`, `Update`, `Delete`):** Simplest for single item interactions.
-   **Batch Operations (`BatchCreate`, `BatchDelete`, `BatchGet`):** Efficient for high-volume, non-atomic bulk operations on items of the same type (up to 25 for writes, 100 for gets).

**Example (`TransactWrite` for inventory):**
```go
// ✅ CORRECT: Atomically update inventory and record order
err := db.TransactWrite(ctx, func(tx core.TransactionBuilder) error {
    // Decrement stock, ensuring stock > 0
    tx.UpdateWithBuilder(product, func(ub core.UpdateBuilder) error {
        ub.Decrement("StockQuantity")
        return nil
    }, tabletheory.Condition("StockQuantity", ">=", 1))

    // Create order item
    tx.Put(order)

    return nil
})
```

### How do I handle eventual consistency when reading immediately after writing to a GSI?

**Solution:** Use `Query.WithRetry()` to implement an application-level retry mechanism with exponential backoff. This polls the GSI until the written data propagates.

-   **When to use:** When a subsequent read on a GSI *must* reflect a recent write, and strong consistency is not an option (GSIs only support eventual consistency).

**Example:**
```go
// ✅ CORRECT: Retrying GSI read for eventual consistency
const ( 
    maxRetries = 5
    initialDelay = 50 * time.Millisecond
)

// After creating/updating a user (which might update a GSI)
// ...

// Attempt to read from GSI with retry
var fetchedUser User
err := db.Model(&User{}).
    Index("email-gsi").
    Where("Email", "=", "new@example.com").
    WithRetry(maxRetries, initialDelay).
    First(&fetchedUser)

if err != nil { /* handle error */ }
```
