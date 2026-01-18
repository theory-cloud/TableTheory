# Core Patterns

This guide documents canonical usage patterns for the **Go** TableTheory SDK, designed to be copy-pasted into your application.

Multi-language core patterns:

- TypeScript: [ts/docs/core-patterns.md](../ts/docs/core-patterns.md)
- Python: [py/docs/core-patterns.md](../py/docs/core-patterns.md)

## Lambda Optimization

**Problem:** AWS Lambda functions suffer from "cold starts" if connections are re-established on every invocation.
**Solution:** Use `NewLambdaOptimized` in the global scope or `init()` function.

```go
// ✅ CORRECT: Global initialization
var db *theorydb.LambdaDB

func init() {
    var err error
    // Initialize once during cold start
    db, err = theorydb.NewLambdaOptimized()
    if err != nil {
        panic(err)
    }
}

func handler(ctx context.Context) error {
    // db is ready to use immediately
    return db.Model(&User{}).Create()
}
```

```go
// ❌ INCORRECT: Handler initialization
func handler(ctx context.Context) error {
    db, _ := theorydb.New(...) // Re-connects every time! 10x slower.
    return db.Model(&User{}).Create()
}
```

## Pagination

**Problem:** Retrieving large datasets in a single call can exceed DynamoDB limits (1MB) or timeout.
**Solution:** Use `Limit()` and loop until results are exhausted.

```go
// ✅ CORRECT: Paginated Query
var allUsers []User
lastEvaluatedKey := ""

for {
    var page []User
    // Configure query
    q := db.Model(&User{}).Limit(50)

    // Apply cursor if continuing
    if lastEvaluatedKey != "" {
        q.Cursor(lastEvaluatedKey)
    }

    // Fetch page
    result, err := q.AllPaginated(&page)
    if err != nil {
        log.Fatal(err)
    }

    allUsers = append(allUsers, page...)

    // Check if more pages exist
    if !result.HasMore {
        break
    }
    lastEvaluatedKey = result.NextCursor
}
```

## Optimistic Locking (Versioning)

**Problem:** Two users update the same item simultaneously, overwriting each other's changes.
**Solution:** Use a version field and `AtVersion` condition.

1. **Model Setup:** Add a version field.

```go
type Document struct {
    ID      string `theorydb:"pk"`
    Content string
    Version int64  `theorydb:"version"` // Automatically increments on update
}
```

2. **Update Logic:**

```go
// ✅ CORRECT: Guarded Update
doc := &Document{ID: "doc_1", Content: "New Content", Version: 5}

// Fails if current version in DB is not 5
err := db.Model(doc).
    Where("ID", "=", doc.ID).
    WithCondition("Version", "=", 5). // Or use .AtVersion(5) in Transaction
    Update()

if errors.Is(err, customerrors.ErrConditionFailed) {
    // Handle conflict: Fetch latest and retry
}
```

## Batch Operations

**Problem:** Reading items one by one is slow and inefficient.
**Solution:** Use `BatchGet` to fetch up to 100 items in parallel.

```go
// ✅ CORRECT: Batch retrieval
var users []User
// Define keys to fetch
keys := []User{
    {ID: "1"},
    {ID: "2"},
    {ID: "3"},
}
// Pass slice of structs with keys set
err := db.Model(&User{}).BatchGet(keys, &users)
```

## DynamoDB Streams

**Problem:** Processing stream events requires parsing complex DynamoDB JSON.
**Solution:** Use `theorydb.UnmarshalStreamImage` to convert stream images to your models.

```go
// ✅ CORRECT: Stream processing
func handleStream(e events.DynamoDBEvent) {
    for _, record := range e.Records {
        if record.EventName == "INSERT" || record.EventName == "MODIFY" {
            var user User
            // Convert Lambda Event Map -> Go Struct
            err := theorydb.UnmarshalStreamImage(record.Change.NewImage, &user)
            if err != nil {
                log.Println("Parse error:", err)
                continue
            }
            processUser(user)
        }
    }
}
```

## Atomic Transactions

**Problem:** Need to update multiple items atomically (e.g., bank transfer).
**Solution:** Use `db.TransactWrite` for ACID guarantees.

```go
// ✅ CORRECT: Transaction
err := db.TransactWrite(ctx, func(tx core.TransactionBuilder) error {
    // 1. Deduct from Sender
    tx.Update(sender, []string{"Balance"})

    // 2. Add to Receiver
    tx.Update(receiver, []string{"Balance"})

    // 3. Record Audit Log
    tx.Put(auditLog)

    return nil
})
// If ANY operation fails, EVERYTHING is rolled back.
```

## Conditional Writes

**Problem:** Prevent overwriting data if it already exists (idempotency).
**Solution:** Use `.IfNotExists()` or `.Where()` conditions on writes.

```go
// ✅ CORRECT: Insert only if ID doesn't exist
err := db.Model(&User{ID: "123"}).
    IfNotExists().
    Create()

if errors.Is(err, customerrors.ErrConditionFailed) {
    log.Println("User already exists!")
}
```

## Conditional Writes

**Problem:** Prevent overwriting data if it already exists (idempotency).
**Solution:** Use `.IfNotExists()` or `.Where()` conditions on writes.

```go
// ✅ CORRECT: Insert only if ID doesn't exist
err := db.Model(&User{ID: "123"}).
    IfNotExists().
    Create()

if errors.Is(err, customerrors.ErrConditionFailed) {
    log.Println("User already exists!")
}
```

## Business Value & Use Cases

TableTheory is designed to provide significant business value by improving developer efficiency, reducing operational costs, and enhancing application performance and reliability.

### Developer Efficiency & Team Velocity

- **Reduced Boilerplate:** TableTheory eliminates approximately **80% of the boilerplate code** typically required for DynamoDB interactions with the raw AWS SDK. This frees developers to focus on business logic.
- **Type Safety:** Compile-time type safety with Go generics prevents common runtime errors, leading to fewer bugs and faster development cycles.
- **Intuitive API:** The fluent, chainable API makes code more readable and easier to maintain, reducing the learning curve for new team members.

### Performance & Reliability

- **Sub-15ms Cold Starts:** With Lambda-optimized initialization (`NewLambdaOptimized`, `LambdaInit`), TableTheory achieves **91% faster cold starts** compared to raw AWS SDK usage, crucial for responsive serverless applications.
- **Optimized Memory Usage:** TableTheory uses **57% less memory** in Lambda environments, contributing to lower execution costs and fewer memory-related issues.
- **Production-Ready Patterns:** Built-in support for transactions, conditional writes, and retry logic helps build robust and fault-tolerant applications.

### Cost Optimization

- **Reduced RCUs/WCUs:** By promoting efficient querying (avoiding scans, using indexes correctly) and batch operations, TableTheory helps minimize consumed Read Capacity Units (RCUs) and Write Capacity Units (WCUs), directly lowering DynamoDB costs.
- **Lower Lambda Costs:** Faster cold starts and reduced memory footprint mean Lambda functions run for shorter durations and require less memory, leading to lower compute costs.
- **Faster Development = Lower Project Costs:** Increased team velocity translates to projects delivered faster and with fewer resources.

### Primary Use Cases

TableTheory is ideal for:

- **Serverless Backends:** Building highly scalable and performant APIs with AWS Lambda and API Gateway.
- **Event-Driven Architectures:** Processing DynamoDB Streams with type-safe model transformations.
- **High-Throughput Microservices:** Services requiring fast, efficient interactions with DynamoDB.
- **Financial & Critical Systems:** Leveraging atomic transactions for data consistency.
- **Real-time Data Processing:** Applications needing low-latency access to DynamoDB data.

---

## Performance & Cost Optimization

**Problem:** Unoptimized DynamoDB interactions or Lambda configurations can lead to high costs and slow performance.
**Solution:** Implement strategies for Lambda memory sizing, efficient connection pooling, and careful RCU/WCU management.

### Lambda Memory Sizing

**Principle:** Higher Lambda memory often means more CPU, faster execution, and lower overall cost for compute-bound tasks, despite a higher _per-GB-second_ price.

- **Recommendation:** Start with 512MB-1GB for most TableTheory-based Lambda functions. Monitor CPU time and memory usage (using `LambdaDB.GetMemoryStats()`) to fine-tune.
- **Impact:**
  - **Memory <= 256MB:** May incur higher cold starts due to limited CPU.
  - **Memory >= 1024MB:** Generally provides optimal performance for typical workloads.

**Example (Monitoring Memory Usage):**

```go
// ✅ CORRECT: Logging memory stats for optimization
func handler(ctx context.Context) error {
    // ... your business logic ...

    stats := db.GetMemoryStats()
    log.Printf("Lambda Memory Stats: Alloc: %.2fMB, Sys: %.2fMB, Used: %.2f%%",
        stats.AllocatedMB, stats.SystemMB, stats.MemoryPercent)

    // Use these stats to adjust Lambda memory configuration in your CDK/CloudFormation
    return nil
}
```

### Connection Pooling & Reuse

**Principle:** Reusing HTTP connections and DynamoDB clients across Lambda invocations drastically reduces cold start latency.

- **Recommendation:** Always use `theorydb.NewLambdaOptimized()` or `theorydb.LambdaInit()` in your `init()` function or global scope.
- **Details:** TableTheory's `LambdaDB` manages an optimized `http.Client` with appropriate `MaxIdleConns` and `IdleConnTimeout` settings for Lambda's execution model.

**Example:**

```go
// ✅ CORRECT: Global init ensures connection pooling
var db *theorydb.LambdaDB

func init() {
    db, _ = theorydb.NewLambdaOptimized()
    db.OptimizeForMemory() // Auto-adjusts internal buffers
}
```

### Read/Write Capacity Unit (RCU/WCU) Management

**Principle:** Minimize consumed capacity by avoiding full table scans and using efficient access patterns.

- **Recommendation:**
  - **Avoid Scans:** Unless absolutely necessary for infrequent analytics on small tables, never use `Scan()` for primary access patterns. Always prefer `Query()` with appropriate Partition and Sort Key conditions.
  - **Batch Operations:** Use `BatchGet`, `BatchCreate`, `BatchDelete` for multiple items to reduce network overhead and potentially consumed capacity compared to individual operations.
  - **Consistent Reads:** Only enable `ConsistentRead()` when strong consistency is strictly required, as it consumes 2x RCUs.
  - **GSI Projection:** Use `KEYS_ONLY` or `INCLUDE` projections on GSIs to reduce the size of items read from the index, minimizing RCU consumption.

**Example (Efficient Query vs. Scan):**

```go
// ❌ INCORRECT: Expensive full table scan
// Will consume RCU proportional to table size, not result size
db.Model(&Product{}).Where("Category", "=", "electronics").All(&products)

// ✅ CORRECT: Efficient GSI query
// Assumes a GSI 'category-index' with Category as its PK
db.Model(&Product{}).Index("category-index").Where("Category", "=", "electronics").All(&products)
```

**Problem:** Prevent overwriting data if it already exists (idempotency).
**Solution:** Use `.IfNotExists()` or `.Where()` conditions on writes.

```go
// ✅ CORRECT: Insert only if ID doesn't exist
err := db.Model(&User{ID: "123"}).
    IfNotExists().
    Create()

if errors.Is(err, customerrors.ErrConditionFailed) {
    log.Println("User already exists!")
}
```
