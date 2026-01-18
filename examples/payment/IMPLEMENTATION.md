# Payment Example Implementation Guide

## Overview
This document describes the implementation of the Payment Example's three main features:
1. **Webhook Notification System** - Async webhook delivery with retry logic
2. **JWT Authentication** - Token validation and merchant ID extraction  
3. **Export Lambda Integration** - Async export job processing

## Feature 1: Webhook Notification System

### Implementation Details

#### Files Created:
- `utils/webhook.go` - Core webhook sender implementation

#### Key Components:

1. **WebhookSender** - Main webhook delivery service
   - Manages worker pool for async processing
   - Queues webhooks for delivery
   - Handles graceful shutdown

2. **Webhook Delivery Features**:
   - Exponential backoff retry (up to 5 attempts)
   - HMAC-SHA256 signature generation
   - Webhook status tracking in DynamoDB
   - TTL-based expiration (24 hours)
   - Support for multiple webhook endpoints

3. **RetryWorker** - Background worker for failed webhooks
   - Polls for failed webhooks periodically
   - Retries delivery with saved state
   - Updates webhook status

### Usage Example:

```go
// Initialize webhook sender
webhookSender := utils.NewWebhookSender(db, 5) // 5 workers
defer webhookSender.Stop()

// Send webhook notification
job := &utils.WebhookJob{
    MerchantID: "merchant-123",
    EventType:  "payment.succeeded",
    PaymentID:  "pay-456",
    Data:       paymentData,
}

// Non-blocking send
if err := webhookSender.Send(job); err != nil {
    log.Printf("Failed to queue webhook: %v", err)
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

## Feature 3: Export Job Queue

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
   - API creates job record in DynamoDB
   - Returns job ID immediately (async)
   - Separate worker processes pending jobs
   - Updates job with result URL when complete

### Usage Example:

```bash
# Request export
POST /payments/export?start_date=2024-01-01&end_date=2024-01-31&format=csv

# Response
{
  "export_id": "export-merchant123-1234567890",
  "status": "pending",
  "message": "Export job created. You will receive a notification when complete.",
  "check_url": "/exports/export-merchant123-1234567890"
}
```

### Worker Implementation (Separate Process):
```go
// Poll for pending jobs
var jobs []*ExportJob
db.Model(&ExportJob{}).
    Index("gsi-status").
    Where("Status", "=", "pending").
    Limit(10).
    All(&jobs)

// Process each job
for _, job := range jobs {
    // 1. Execute query
    // 2. Generate CSV/JSON
    // 3. Upload to S3
    // 4. Update job with result URL
    // 5. Send webhook notification
}
```

## Integration Points

### Process Handler Updates:
```go
// Added webhook sender initialization
webhookSender := utils.NewWebhookSender(db, 5)

// Added JWT validator
jwtValidator := utils.NewSimpleJWTValidator(...)

// Integrated webhook sending after payment success
go func() {
    if err := h.webhookSender.Send(webhookJob); err != nil {
        fmt.Printf("Failed to queue webhook: %v\n", err)
    }
}()
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

3. **Export Security**:
   - Pre-signed S3 URLs with expiration
   - Merchant-scoped exports only
   - Audit trail for all exports

## Performance Considerations

1. **Webhook Delivery**:
   - Configurable worker pool size
   - Non-blocking sends
   - Queue size limits to prevent OOM

2. **Export Processing**:
   - Async job queue pattern
   - Pagination for large datasets
   - S3 multipart uploads for large files

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