# TableTheory Troubleshooting

This guide provides solutions to common issues with verified fixes, categorized by the type of error you might encounter.

Multi-language troubleshooting:

- TypeScript: [ts/docs/troubleshooting.md](../ts/docs/troubleshooting.md)
- Python: [py/docs/troubleshooting.md](../py/docs/troubleshooting.md)

## Quick Diagnosis

| Symptom                         | Likely Cause                         | Section                                       |
| ------------------------------- | ------------------------------------ | --------------------------------------------- |
| `ValidationException`           | Struct tags mismatch / Type mismatch | [Validation Errors](#validation-errors)       |
| `ResourceNotFoundException`     | Table missing or wrong Region        | [Configuration Errors](#configuration-errors) |
| `TransactionCanceledException`  | Condition check failed / Conflict    | [Transaction Errors](#transaction-errors)     |
| `ProvisionedThroughputExceeded` | Insufficient RCU/WCU                 | [Capacity Errors](#capacity-errors)           |
| Slow Cold Starts (>100ms)       | Handler initialization               | [Performance Issues](#performance-issues)     |
| Fields missing in DynamoDB      | Missing `json` tags                  | [Data Modeling Issues](#data-modeling-issues) |

---

## Validation Errors

### "One or more parameter values were invalid" / "An attribute value must not be empty"

**Cause:**
You are trying to write an item where the Primary Key (PK) or Sort Key (SK) is empty string `""`. DynamoDB does not allow empty strings for key attributes.

**Solution:**
Ensure your struct fields are populated before calling `Create()`.

```go
// ❌ INCORRECT: ID is empty
user := &User{Name: "John"}
db.Model(user).Create() // Fails

// ✅ CORRECT
user := &User{ID: "user_123", Name: "John"}
db.Model(user).Create()
```

### "Member must satisfy enum value set: [BEGINS_WITH, BETWEEN, ...]"

**Cause:**
You used an invalid operator in `Where()`.

**Solution:**
Check your operator string.

- **Valid for Key:** `=`, `<`, `>`, `<=`, `>=`, `BEGINS_WITH`, `BETWEEN`.
- **Valid for Filter:** All of the above plus `IN`, `CONTAINS`.

---

## Configuration Errors

### "ResourceNotFoundException: Requested resource not found"

**Cause:**

1. The table does not exist in the configured AWS Region.
2. TableTheory is inferring the wrong table name.

**Solution:**

1. Check your region in `session.Config`.
2. Check the table name. By default, `User` struct maps to `users` table.
3. If you use custom table names, implement the `TableName()` interface.

```go
// Optional: Custom table name
func (u User) TableName() string {
    return "my_custom_user_table"
}
```

---

## Transaction Errors

### "TransactionCanceledException: Transaction canceled, please refer cancellation reasons"

**Cause:**
A `ConditionCheck` within the transaction failed. This often happens when using `.IfNotExists()` and the item already exists, or when an optimistic lock version mismatch occurs.

**Solution:**
Use `errors.As` to inspect the `TransactionError`.

```go
err := db.TransactWrite(ctx, func(tx core.TransactionBuilder) error {
    tx.Put(user, tabletheory.IfNotExists())
    return nil
})

if err != nil {
    var txErr *customerrors.TransactionError
    if errors.As(err, &txErr) {
        log.Printf("Operation at index %d failed: %s", txErr.OperationIndex, txErr.Reason)
        if txErr.Reason == "ConditionalCheckFailed" {
            // Handle duplicate item
        }
    }
}
```

---

## Capacity Errors

### "ProvisionedThroughputExceededException"

**Cause:**
Your application is exceeding the RCU/WCU limits of the table.

**Solution:**

1. **Short Term:** Enable auto-scaling on your DynamoDB table.
2. **Retry Config:** Increase `MaxRetries` in `session.Config`.
3. **Code Fix:** Reduce batch sizes or use `BatchGetWithOptions` with a rate limiter.

```go
// Increase retries for bursty workloads
db, _ := tabletheory.New(session.Config{
    MaxRetries: 10, // Default is 3
})
```

---

## Data Modeling Issues

### Fields are saving as "AttributeName" (Title Case) but I want "attribute_name" (snake_case)

**Cause:**
TableTheory uses the struct field name by default if no tag is present.

**Solution:**
Add `json` tags to your struct. TableTheory respects `json` tags for attribute naming.

```go
type User struct {
    // Saves as "first_name" in DynamoDB
    FirstName string `json:"first_name"`

    // Saves as "LastName" (Go field name)
    LastName  string
}
```

### "Item not found" when querying by GSI

**Cause:**
You are using `Where()` on a GSI key but forgot to call `Index()`. Without `Index()`, TableTheory assumes you are querying the main table's Primary Key.

**Solution:**
Explicitly specify the index name.

```go
// ❌ INCORRECT: Assumes Email is the table's PK
db.Model(&User{}).Where("Email", "=", "me@example.com").First(&u)

// ✅ CORRECT: Tells DynamoDB to look at the GSI
db.Model(&User{}).Index("email-index").Where("Email", "=", "me@example.com").First(&u)
```

---

## Performance Issues

### Slow Lambda Cold Starts

**Diagnosis:**
Check your logs for initialization time. If > 500ms, you are likely creating a new AWS session per invocation.

**Solution:**
Use the `LambdaInit` helper in your `init()` function.

```go
var db *tabletheory.LambdaDB

func init() {
    // Pre-registers models and warms up connection
    db, _ = tabletheory.LambdaInit(&User{}, &Order{})
}
```

---

## Production Scenarios & Incident Management

This section covers advanced troubleshooting and incident response in production environments.

### Emergency Procedures

#### Table Accidentally Deleted

**Immediate Action:**

1.  **STOP Deployments:** Halt any ongoing CI/CD pipelines to prevent further damage.
2.  **Restore from Backup:** If point-in-time recovery (PITR) is enabled, restore the table to the last known good state. If not, recover from the most recent daily/weekly backup.
3.  **Notify Team:** Alert relevant stakeholders (DevOps, SRE, affected teams).

**Prevention:**

- Implement strong IAM policies to restrict `DeleteTable` permissions.
- Enable PITR on all production tables.
- Use infrastructure-as-code (IaC) for all table management.

#### Wrong Region Deployment

**Immediate Action:**

1.  **Verify Region:** Confirm the `Region` in your `session.Config` and AWS environment variables.
2.  **Rollback:** Deploy to the correct region, or roll back to a known good version.
3.  **Clean Up:** Manually delete any resources accidentally created in the wrong region.

**Prevention:**

- Enforce region specificity in CI/CD pipelines.
- Use environment variables (e.g., `AWS_REGION`) consistently.

### Performance Debugging

#### Slow Queries / High Costs

**Diagnosis:**

- **CloudWatch Metrics:** Monitor `ConsumedReadCapacityUnits`, `ConsumedWriteCapacityUnits`, `ThrottledRequests` for your DynamoDB table and indexes.
- **AWS X-Ray:** Trace requests to identify bottlenecks in your Lambda function or DynamoDB calls.
- **TableTheory Logs:** Enable detailed logging within TableTheory (if available) to see query plans.

**Solution:**

1.  **Eliminate Scans:** Refactor code to use `Query()` with proper PK/SK conditions or GSIs.
2.  **Optimize Indexes:** Ensure GSIs are correctly used and projected attributes are minimal.
3.  **Batching:** Group multiple `GetItem` or `PutItem` requests into `BatchGet` or `BatchWrite` calls.
4.  **Lambda Profiling:** Use `LambdaDB.GetMemoryStats()` and Go pprof to identify CPU/memory intensive code within your Lambda.

#### High Cold Start Latency

**Diagnosis:**

- Check CloudWatch Logs for `REPORT` lines, specifically `Init Duration`.
- Monitor the time taken by your `init()` function.

**Solution:**

- Ensure `tabletheory.LambdaInit()` is called once in `init()`.
- Pre-register all models with `LambdaInit()` to avoid reflection at runtime.
- Increase Lambda memory. This can indirectly speed up initialization.

### Monitoring & Alerting Setup

**Recommendations:**

- **CloudWatch Alarms:** Set up alarms for:
  - `ConsumedReadCapacityUnits` (above provisioned/on-demand thresholds)
  - `ConsumedWriteCapacityUnits` (above provisioned/on-demand thresholds)
  - `ThrottledRequests` (any non-zero count)
  - `IteratorAge` (for DynamoDB Streams, indicates backlog)
- **X-Ray Tracing:** Enable X-Ray for all Lambda functions interacting with TableTheory to get detailed trace maps and performance breakdowns.
- **Custom Metrics:** Emit custom metrics for critical business operations, latency, and error rates.

### Production Incident Playbooks

**General Steps:**

1.  **Detect:** Alerts from CloudWatch, PagerDuty, etc.
2.  **Assess:** Verify the issue, identify impacted services/customers.
3.  **Diagnose:** Use logs (CloudWatch Logs, application logs), traces (X-Ray), and metrics (CloudWatch) to pinpoint the root cause.
4.  **Mitigate:** Implement temporary fixes (e.g., disable a feature, scale up capacity).
5.  **Resolve:** Apply permanent fix (code deployment, infrastructure change).
6.  **Post-Mortem:** Document the incident, root cause, and preventative measures.

**Key Metrics to Monitor:**

- **API Latency:** End-to-end response times.
- **Error Rate:** Percentage of failed requests.
- **Throughput:** Requests per second.
- **DynamoDB Consumed Capacity:** Read/Write units consumed.
- **Lambda Invocation Duration & Errors:** Monitor Lambda function health.

**Communication:** Always have a clear communication plan for incidents, informing stakeholders about impact, status, and resolution.
