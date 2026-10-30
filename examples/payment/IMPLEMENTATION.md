# Payment Example Implementation Guide

## Overview
This document describes the implementation of the Payment Example's three main features:
1. **Webhook Notification System** - Synchronous public-HTTPS delivery with retry logic and destination validation
2. **JWT Authentication and Merchant Policy** - Token validation plus active-merchant, permission, amount, and rate-limit enforcement
3. **Export Job Records** - Export requests are recorded as jobs; processing is out of scope for this example

## Feature 1: Webhook Notification System

### Implementation Details

#### Files Created:
- `utils/webhook.go` - Core webhook sender implementation

#### Key Components:

1. **WebhookSender** - Main webhook delivery service
   - Delivers on the caller's goroutine: delivery is the caller's own work,
     with no queue behind it, because Lambda freezes the execution environment
     the moment the handler returns
   - Bounded by the caller's context and the sender's internal timeout

2. **Webhook Delivery Features**:
   - Public HTTPS destinations only; `http://` and other schemes are refused
   - DNS and dialed-IP validation before record creation, before every attempt,
     and on every redirect
   - Loopback, link-local, private, unspecified, CGNAT, documentation,
     benchmark, multicast, and reserved address ranges are refused
   - The validated address is dialed directly, with a second check at the
     socket boundary, so DNS rebinding cannot swap in a private target
   - Exponential backoff retry (up to 5 attempts, all inside the invocation) for
     ordinary network and server failures; unsafe destinations fail permanently
   - HMAC-SHA256 signature generation
   - Webhook status tracking in DynamoDB
   - TTL-based expiration (24 hours)
   - Support for multiple webhook endpoints

There is no environment variable or private-network override. A merchant with a
private, loopback, or plaintext webhook URL receives no delivery attempt until
the merchant's configuration is changed to a public HTTPS endpoint.

Failed webhooks are re-driven from a queue or a scheduled invocation, never from
anything left running inside the invocation: this example deploys only as Lambda
functions (`lambda/process`, `lambda/query`, `lambda/reconcile`), so nothing
survives a handler's return to carry the retry.

### Usage Example:

```go
// Initialize webhook sender. It starts no goroutine.
webhookSender := utils.NewWebhookSender(db)

// Deliver a webhook synchronously inside the calling invocation. Lambda freezes
// the execution environment when the handler returns, so a delivery launched in
// a goroutine would be frozen mid-flight. The call is bounded by ctx and by the
// sender's internal timeout.
job := &utils.WebhookJob{
    MerchantID: "merchant-123",
    EventType:  "payment.succeeded",
    PaymentID:  "pay-456",
    Data:       paymentData,
}

if err := webhookSender.SendSync(ctx, job); err != nil {
    log.Printf("Failed to deliver webhook: %v", err)
}
```

`SendSync` refuses a destination that is not public HTTPS before creating the
webhook record. Keep every merchant `WebhookURL` on public HTTPS.

### Webhook Headers:
- `X-Webhook-ID` - Unique webhook identifier
- `X-Webhook-Timestamp` - Unix timestamp
- `X-Webhook-Signature` - HMAC-SHA256 signature

### Signature Verification:
```
signature = HMAC-SHA256(secret, timestamp + "." + payload)
```

## Feature 2: JWT Authentication and Merchant Policy

### Implementation Details

#### Files Created:
- `utils/jwt.go` - JWT validation implementation

#### Key Components:

1. **SimpleJWTValidator** - HMAC-based JWT validator
   - HS256 algorithm support
   - Standard claims validation
   - Custom merchant ID claim requirement

2. **Token Validation Features**:
   - Expiration checking
   - Issuer validation
   - Audience validation
   - Merchant ID extraction
   - Exact permission extraction (`ValidateAndExtractClaims`)

The process handler loads the stored merchant before the idempotency reservation
or any payment write. It requires:

- merchant `Status == "active"`
- the exact JWT permission `payments:write` (no prefix, casing, or alias match)
- a positive `MaxPaymentAmount`, and `amount <= MaxPaymentAmount`
- positive `PaymentsPerMinute` and `PaymentsPerDay` values

Zero or negative configured limits are fail-closed: they deny positive payments.
They do **not** mean "unlimited". A refusal at this point writes no payment,
transaction, usage counter, webhook record, or idempotency record.

### Usage Example:

```go
// Initialize JWT validator
jwtValidator := utils.NewSimpleJWTValidator(
    "your-secret-key",
    "your-issuer",
    "payment-api",
)

// Extract and validate the claims used by the merchant policy.
claims, err := utils.ValidateAndExtractClaims(
    request.Headers["Authorization"],
    jwtValidator,
)
```

### JWT Claims Structure:
```json
{
  "merchant_id": "merchant-123",
  "email": "merchant@example.com",
  "permissions": ["payments:write"],
  "iss": "your-issuer",
  "aud": ["payment-api"],
  "exp": 1234567890,
  "iat": 1234567890
}
```

### Minute and Day Usage Accounting

`UsageCounter` stores one row per merchant per fixed UTC minute and per fixed UTC
day. The counter increment is an `ADD`, guarded by
`attribute_not_exists(usageCount) OR usageCount < :limit`, and it shares the
same DynamoDB transaction as the payment and transaction records. A conditional
failure cancels the whole transaction, so concurrent requests cannot overshoot
the limit and a refused request leaves no counter increment behind.

Idempotent replay never executes the payment again: a successful replay returns
the stored payment with HTTP 200 and no additional counter increment, while a
rate-limit refusal replays as HTTP 429.

## Feature 3: Export Job Records

### Implementation Details

#### Files Modified:
- `lambda/query/handler.go` - Added export endpoint

#### Key Components:

1. **ExportJob Model** - DynamoDB-backed job queue
   ```go
   type ExportJob struct {
       ID         string       // Unique job ID
       MerchantID string       // Merchant requesting export
       Status     string       // pending, processing, completed, failed
       Query      QueryRequest // Export parameters
       Format     string       // csv, json
       ResultURL  string       // S3 URL when complete
       ExpiresAt  time.Time    // TTL for cleanup
   }
   ```

2. **Export Flow**:
   - The API creates an `ExportJob` record in DynamoDB with status `pending`
   - It returns the job ID and a status URL immediately
   - That is the whole flow this example implements: the job is recorded and
     nothing consumes it

   Processing is out of scope here. A `pending` job stays `pending` until a
   separately deployed consumer reads it; this repository ships no such
   consumer, so no code turns a job into a result. Everything the future
   consumer needs is written into the record — merchant, query parameters,
   requested format, and an `ExpiresAt` TTL — but finishing the job is left
   to the reader.

### Usage Example:

```bash
# Request export
POST /payments/export?start_date=2024-01-01&end_date=2024-01-31&format=csv

# Response
{
  "export_id": "export-merchant123-1234567890",
  "status": "pending",
  "message": "Export job recorded. Nothing in this example processes export jobs, so no notification will be sent and the job stays pending.",
  "check_url": "/exports/export-merchant123-1234567890"
}
```

### Export Processing Is Out of Scope

This example records the job and stops there. Nothing in this repository reads
pending `ExportJob` items, generates the CSV/JSON artifact, uploads it, or
writes `ResultURL` back to the record. Adding that consumer is a separate,
explicitly deployed piece of work that the example does not include; until it
exists, a job simply stays `pending`.

The `message` in the response above states exactly what this example does: the
job is recorded, nothing processes it, and no notification is sent. It is not a
promise the example keeps somewhere else.

## Integration Points

### Process Handler Updates:
```go
// Added webhook sender initialization: starts no goroutine
webhookSender := utils.NewWebhookSender(db)

// Added JWT validator
jwtValidator := utils.NewSimpleJWTValidator(...)

// Integrated webhook delivery after payment success, synchronously inside the
// invocation and bounded by ctx. It must not be detached: work launched in a
// goroutine here would be frozen when the handler returns.
if err := h.webhookSender.SendSync(ctx, webhookJob); err != nil {
    fmt.Printf("Failed to deliver webhook: %v\n", err)
}
```

### Query Handler Updates:
```go
// Added JWT validation for all endpoints
merchantID, err := h.extractMerchantID(request.Headers)

// Added export job creation
exportJob := &ExportJob{...}
if err := h.db.Model(exportJob).Create(); err != nil {
    return errorResponse(...)
}
```

## Testing

### Run Tests:
```bash
go test ./examples/payment/...
go test -race ./examples/payment/...
```

### Test Coverage:
- ✅ Public HTTPS webhook delivery, signature, retry, and timeout behavior
- ✅ Direct, redirect, mixed-DNS, and DNS-rebinding SSRF refusals before bytes
- ✅ JWT token validation and exact `payments:write` enforcement
- ✅ Active/missing/suspended merchant and zero/excess amount refusals
- ✅ Atomic minute/day boundaries, concurrent overshoot attempts, and replay
- ✅ Export job creation

## Environment Variables

```bash
# JWT Configuration
JWT_SECRET=your-secret-key
JWT_ISSUER=your-issuer
JWT_AUDIENCE=payment-api

# AWS Configuration
AWS_REGION=us-east-1
```

## Security Considerations

1. **JWT Security**:
   - Use strong secret keys (min 256 bits)
   - Rotate keys regularly
   - Short token expiration (1 hour recommended)

2. **Merchant Policy**:
   - Require active merchant status and exact `payments:write`
   - Treat zero/negative limits as deny, never as unlimited
   - Keep the counter increment in the payment transaction

3. **Webhook Security**:
   - Public HTTPS endpoints only; no private-network or environment override
   - Resolve and validate the destination before record creation, every attempt,
     and every redirect; dial the validated address
   - Verify webhook signatures
   - Implement request timeouts

4. **Export Security** (guidance for the consumer that is not yet shipped):
   - Serve results only through pre-signed, expiring URLs
   - Scope every export to the requesting merchant
   - Keep an audit trail for export requests

## Policy Migration and Rollback

These checks are security corrections, not feature flags.

- Merchants with zero or negative `MaxPaymentAmount`, `PaymentsPerMinute`, or
  `PaymentsPerDay` now have positive payments denied. Set all three to positive
  values before enabling a merchant; there is no legacy unlimited
  interpretation.
- Merchants with non-HTTPS or private-network webhook URLs stop receiving
  deliveries. Update the stored URL to a public HTTPS endpoint. There is no
  environment opt-out.
- Create the `UsageCounters` table with partition key `id` and enable TTL on
  `expiresAt` alongside the existing payment tables. The example registers the
  model but deliberately does not provision tables.
- The new `UsageCounters` rows are additive and harmless if unused. Reverting the
  enforcement restores the original exposure; roll forward with corrected policy
  instead of restoring the unsafe behavior.
- Payment, transaction, audit, idempotency, and webhook records are not migrated
  or deleted. Retain them through any rollback.

## Performance Considerations

1. **Webhook Delivery**:
   - Synchronous, bounded by the caller's context and the sender's timeout
   - Retries happen inside the invocation that triggered them

2. **Export Job Recording**:
   - One DynamoDB write per export request, on the invocation path
   - Merchant-scoped records with a TTL for cleanup
   - The record stores the query parameters and format the consumer will need

## Next Steps

1. **Operational Readiness**:
   - Add proper logging (structured logs)
   - Implement metrics/monitoring
   - Add circuit breakers for webhooks
   - Add merchant provisioning and limit administration outside this example;
     the example reads stored limits but does not create or edit merchants

2. **Enhanced Features**:
   - Multiple webhook URLs per merchant
   - Webhook event filtering
   - Export scheduling
   - Real-time export progress

3. **Testing**:
   - Load testing for webhooks
   - Hosted endpoint integration tests for webhook and SMTP delivery
   - Export performance benchmarks