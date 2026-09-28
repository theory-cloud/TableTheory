# Payment Example Implementation Guide

## Overview
This document describes the implementation of the Payment Example's three main features:
1. **Webhook Notification System** - Synchronous webhook delivery with retry logic
2. **JWT Authentication** - Token validation and merchant ID extraction  
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
   - Exponential backoff retry (up to 5 attempts, all inside the invocation)
   - HMAC-SHA256 signature generation
   - Webhook status tracking in DynamoDB
   - TTL-based expiration (24 hours)
   - Support for multiple webhook endpoints

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

### Webhook Headers:
- `X-Webhook-ID` - Unique webhook identifier
- `X-Webhook-Timestamp` - Unix timestamp
- `X-Webhook-Signature` - HMAC-SHA256 signature

### Signature Verification:
```
signature = HMAC-SHA256(secret, timestamp + "." + payload)
```

## Feature 2: JWT Authentication

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

### Usage Example:

```go
// Initialize JWT validator
jwtValidator := utils.NewSimpleJWTValidator(
    "your-secret-key",
    "your-issuer",
    "payment-api",
)

// Extract merchant ID from request
merchantID, err := utils.ValidateAndExtractMerchantID(
    request.Headers["Authorization"],
    jwtValidator,
)
```

### JWT Claims Structure:
```json
{
  "merchant_id": "merchant-123",
  "email": "merchant@example.com",
  "permissions": ["payments", "refunds"],
  "iss": "your-issuer",
  "aud": ["payment-api"],
  "exp": 1234567890,
  "iat": 1234567890
}
```

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
cd theorydb/examples/payment/tests
go test -v webhook_test.go
```

### Test Coverage:
- ✅ Webhook delivery with retry
- ✅ JWT token validation
- ✅ Token extraction from headers
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

2. **Webhook Security**:
   - Verify webhook signatures
   - Use HTTPS endpoints only
   - Implement request timeouts

3. **Export Security** (guidance for the consumer that is not yet shipped):
   - Serve results only through pre-signed, expiring URLs
   - Scope every export to the requesting merchant
   - Keep an audit trail for export requests

## Performance Considerations

1. **Webhook Delivery**:
   - Synchronous, bounded by the caller's context and the sender's timeout
   - Retries happen inside the invocation that triggered them

2. **Export Job Recording**:
   - One DynamoDB write per export request, on the invocation path
   - Merchant-scoped records with a TTL for cleanup
   - The record stores the query parameters and format the consumer will need

## Next Steps

1. **Production Readiness**:
   - Add proper logging (structured logs)
   - Implement metrics/monitoring
   - Add circuit breakers for webhooks
   - Rate limiting per merchant

2. **Enhanced Features**:
   - Multiple webhook URLs per merchant
   - Webhook event filtering
   - Export scheduling
   - Real-time export progress

3. **Testing**:
   - Load testing for webhooks
   - Integration tests with real JWT tokens
   - Export performance benchmarks 