---
title: TableTheory Troubleshooting
---

# TableTheory Troubleshooting

This guide provides solutions to common issues with verified fixes, categorized by the type of error you might encounter.

TypeScript and Python ship their own runtime-specific troubleshooting guides as sibling package surfaces in the shared
TheoryCloud TableTheory subtree. This page is the Go troubleshooting guide.

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

### "TransactionCanceledException" and cancellation reasons

A DynamoDB transaction is atomic: when any operation's condition fails, nothing
is committed and DynamoDB returns a `TransactionCanceledException` with one
cancellation reason per operation, in order (`"None"` for the operations that
did not fail). TableTheory classifies that exception for you instead of leaving
you to parse message text.

**Classification (identical in Go, TypeScript and Python).** The FIRST genuine
failure — the first reason whose code is neither missing nor `"None"` — decides
the error:

| DynamoDB reason code | Go sentinel |
| --- | --- |
| `ConditionalCheckFailed` | `errors.ErrConditionFailed` |
| `TransactionConflict` | `errors.ErrTransactionConflict` |
| `ThrottlingException`, `ThrottlingError`, `ProvisionedThroughputExceeded`, `RequestLimitExceeded`, `InternalServerError`, `ServiceUnavailable` | `errors.ErrThrottled` |
| anything else, no genuine failure, or an empty reason list | `errors.ErrTransactionFailed` |

An unknown or missing code is never reclassified as a condition failure, and an
ambiguous transport error (a timeout, a `dial tcp` failure, a connection reset)
is never turned into a transaction or condition error — it passes through
unchanged.

A cancellation with **no genuine per-operation failure** — an empty reason list,
or every reason missing its code or carrying the `"None"` placeholder — reports
`errors.ErrTransactionFailed` and carries `OperationIndex: -1`. That deliberate
marker keeps it distinct from a real failure at operation 0 (where
`OperationIndex` is `0`); `Operation` and `Reason` are empty on that path.

**Inspect the cause and every reason.** Use `errors.Is` to match a sentinel and
`errors.As` to recover structure and the original AWS exception.

```go
err := db.TransactWrite(ctx, func(tx core.TransactionBuilder) error {
    tx.Put(user, tabletheory.IfNotExists())
    return nil
})

switch {
case errors.Is(err, theorydbErrors.ErrConditionFailed):
    // A conditional write lost; nothing was committed.
case errors.Is(err, theorydbErrors.ErrTransactionConflict):
    // A racing transaction conflict.
case errors.Is(err, theorydbErrors.ErrThrottled):
    // Throttled; back off.
case errors.Is(err, theorydbErrors.ErrTransactionFailed):
    // Canceled with no classifiable reason.
}

var txErr *theorydbErrors.TransactionError
if errors.As(err, &txErr) {
    // First genuine failure (unchanged meaning).
    log.Printf("op %d (%s) failed: %s", txErr.OperationIndex, txErr.Operation, txErr.Reason)
    // Every reason, in original order (positions match the operations).
    for _, reason := range txErr.Reasons {
        log.Printf("  [%d] %s: %s", reason.Index, reason.Code, reason.Message)
    }
    // Only the genuine failures, in order.
    for _, failure := range txErr.RealFailures() {
        log.Printf("  failure [%d] %s", failure.Index, failure.Code)
    }
}

// The original SDK exception (and its ordered CancellationReasons) stays
// recoverable, even when the error is wrapped by an intermediate.
var awsCanceled *types.TransactionCanceledException
if errors.As(err, &awsCanceled) {
    // awsCanceled.CancellationReasons is the raw AWS list.
}
```

`errors.Is` is the supported comparison — do not compare transaction sentinels
with `==`: the transaction entry points return a structured error that unwraps
to the sentinel.

**Item images are never exposed on the typed surface.** A cancellation reason
carries only its code and message. TableTheory never copies the DynamoDB `Item`
attribute map into `TransactionError`, so `Error()`, `fmt` formatting and logs of
the structured error never include item data or attribute values. The original
exception is retained on the exported `Cause` field (and recoverable with
`errors.As`); when the request returned item images (for example with
`ReturnValuesOnConditionCheckFailure`) a reflective or serializing dump of that
raw exception — `%#v`, a JSON/reflection dump of the SDK error — can include
them. Log the typed fields as shown above.

**Safe logging helpers and opt-in item-image redaction.** Go's `Error()`,
`%v`/`%+v`/`%s`/`%q` and the typed fields above carry no item data; only a
reflective dump of the raw `Cause` can. In TypeScript and Python the mapped error
retains the raw SDK exception on `cause` / `__cause__`, and that object *is*
walked by a whole-error serialization — `JSON.stringify(err)`, `console.log(err)`,
`util.inspect`, or a structured logger or error tracker that dumps the cause — so
it can include `CancellationReasons[].Item` or a condition failure's `Item`.

Both runtimes expose a typed-only projection, and an opt-in option that strips
item images from the retained cause:

| Runtime | Safe projection | Opt-in redaction |
| --- | --- | --- |
| TypeScript | `safeErrorFields(err)` → `{ code, codes, message, cancellationReasons }`, or `undefined` when the value is not a `TheorydbError` | `new TheorydbClient(ddb, { redactItemImages: true })` |
| Python | `err.safe_dict()` → `{ "type", "message", ...typed fields }` | `Table(model, client=..., redact_item_images=True)` |

`safeErrorFields` never includes `cause`, so it cannot reach item images.
`safe_dict()` returns only the typed surface and never `__cause__` or any
request/response payload.

**Python coverage boundary.** `redact_item_images` covers the client a `Table`
uses. A component you hand a raw client to does not inherit the option — a
`LeaseManager` keeps the client you pass it — so wrap that client with
`redacting_client` from `tabletheory_py.aws_errors` (it is not a root export):

```python
import boto3

from tabletheory_py import LeaseManager
from tabletheory_py.aws_errors import redacting_client

leases = LeaseManager(
    client=redacting_client(boto3.client("dynamodb")),
    table_name="Leases",
)
```

The generated Python API reference does not render the `Table` constructor, so it
does not list `redact_item_images`; the
[Python troubleshooting guide](runtimes/python/troubleshooting.md) documents the
option and `redacting_client`.

**TypeScript coverage boundary.** `redactItemImages` is client configuration and
is inherited by every client the SDK error is mapped through: the configured
`TheorydbClient`, clients derived with `withSendOptions`, `withDynamoDBClient`,
and `fork`, and `withLambdaTimeout`, which derives through `withSendOptions`.
`lease.ts`'s three `mapDynamoError` call sites and `update-builder`'s `execute()`
map errors directly and cannot inherit it, and there is no standalone TypeScript
wrapper. Stock TypeScript commands do not set
`ReturnValuesOnConditionCheckFailure`, so those direct paths cannot carry item
images today; a custom client or middleware, or future `ALL_OLD` support, could
change that. Revisit this boundary if failure item images are ever requested. See
the [TypeScript troubleshooting guide](runtimes/typescript/troubleshooting.md).

The redaction option is **off by default**, so default behavior is unchanged: the
retained `cause`/`__cause__` is the same object it always was. When it is enabled,
the cause becomes a copy whose `CancellationReasons[].Item` (and a condition
failure's top-level `Item`) are removed, and the original error object is never
mutated. Until it is enabled, dumping a whole error with the default settings can
still reach the item images. Closing that default gap would change the retained
cause's identity for every consumer, so it belongs to a future major release — it
is not done here.

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
