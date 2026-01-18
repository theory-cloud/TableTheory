# TableTheory Lambda Example

This example demonstrates how to use TableTheory in AWS Lambda with multi-account support and optimized cold starts.

## Features

- ✅ Optimized for Lambda cold starts (< 100ms)
- ✅ Multi-account support with AssumeRole
- ✅ Connection reuse across warm invocations
- ✅ Lambda timeout handling
- ✅ Pre-registered models for faster initialization

## Setup

### 1. Environment Variables

Configure these environment variables in your Lambda function:

```bash
# Base configuration
AWS_REGION=us-east-1

# Partner 1 configuration
PARTNER1_ROLE_ARN=arn:aws:iam::123456789012:role/TableTheoryPartner1Role
PARTNER1_EXTERNAL_ID=unique-external-id-partner1
PARTNER1_REGION=us-east-1

# Partner 2 configuration
PARTNER2_ROLE_ARN=arn:aws:iam::987654321098:role/TableTheoryPartner2Role
PARTNER2_EXTERNAL_ID=unique-external-id-partner2
PARTNER2_REGION=us-west-2
```

### 2. IAM Roles

Create cross-account roles for each partner with this trust policy:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::YOUR_ACCOUNT_ID:root"
      },
      "Action": "sts:AssumeRole",
      "Condition": {
        "StringEquals": {
          "sts:ExternalId": "unique-external-id-partner1"
        }
      }
    }
  ]
}
```

### 3. Lambda Function Permissions

Your Lambda execution role needs:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "dynamodb:*"
      ],
      "Resource": "arn:aws:dynamodb:*:*:table/payments*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "sts:AssumeRole"
      ],
      "Resource": [
        "arn:aws:iam::*:role/TableTheoryPartner*"
      ]
    }
  ]
}
```

## Building and Deployment

### Build for Lambda

```bash
# Build the Lambda function
GOOS=linux GOARCH=amd64 go build -tags lambda -ldflags="-s -w" -o bootstrap main.go

# Create deployment package
zip function.zip bootstrap
```

### Deploy with AWS CLI

```bash
# Create function
aws lambda create-function \
  --function-name theorydb-example \
  --runtime provided.al2 \
  --role arn:aws:iam::YOUR_ACCOUNT:role/lambda-execution-role \
  --handler bootstrap \
  --zip-file fileb://function.zip \
  --memory-size 512 \
  --timeout 30 \
  --environment Variables="{
    PARTNER1_ROLE_ARN=arn:aws:iam::123456789012:role/TableTheoryPartner1Role,
    PARTNER1_EXTERNAL_ID=unique-external-id-partner1,
    PARTNER1_REGION=us-east-1
  }"

# Update function code
aws lambda update-function-code \
  --function-name theorydb-example \
  --zip-file fileb://function.zip
```

## Usage

### Request Format

```json
{
  "partnerId": "partner1",
  "action": "createPayment",
  "data": {
    "amount": 100.50,
    "currency": "USD"
  }
}
```

### Supported Actions

1. **Get Payment**
```json
{
  "partnerId": "partner1",
  "action": "getPayment",
  "data": {
    "paymentId": "pay_1234567890"
  }
}
```

2. **Create Payment**
```json
{
  "partnerId": "partner1",
  "action": "createPayment",
  "data": {
    "amount": 100.50,
    "currency": "USD"
  }
}
```

### Response Format

Success response:
```json
{
  "success": true,
  "data": {
    "id": "pay_1234567890",
    "partnerId": "partner1",
    "amount": 10050,
    "currency": "USD",
    "status": "pending",
    "createdAt": "2024-01-01T00:00:00Z",
    "updatedAt": "2024-01-01T00:00:00Z"
  }
}
```

Error response:
```json
{
  "success": false,
  "error": "payment not found"
}
```

## Performance Optimization

### Cold Start Optimization

The example uses several techniques to minimize cold starts:

1. **Global DB Instance**: Reuses connections across warm invocations
2. **Pre-registered Models**: Models are registered during init
3. **Optimized HTTP Client**: Configured for Lambda environment
4. **Connection Pooling**: Tuned for Lambda memory constraints

### Memory Configuration

Adjust Lambda memory based on your needs:

- **256 MB**: Basic operations, minimal concurrent requests
- **512 MB**: Standard operations, moderate load
- **1024 MB**: Complex queries, high throughput
- **3008 MB**: Maximum performance, heavy workloads

### Monitoring

Enable X-Ray tracing for performance insights:

```bash
aws lambda update-function-configuration \
  --function-name theorydb-example \
  --tracing-config Mode=Active
```

## Testing

### Local Testing

```bash
# Install SAM CLI
brew install aws-sam-cli

# Run locally
sam local start-lambda

# Invoke function
aws lambda invoke \
  --function-name theorydb-example \
  --endpoint-url http://localhost:3001 \
  --payload '{"partnerId":"partner1","action":"getPayment","data":{"paymentId":"test"}}' \
  response.json
```

### Integration Testing

```bash
# Run integration tests
go test -v ./examples/lambda/...
```

## Troubleshooting

### Common Issues

1. **Timeout Errors**
   - Increase Lambda timeout
   - Check DynamoDB throttling
   - Verify network configuration

2. **Permission Errors**
   - Check IAM roles and policies
   - Verify external ID matches
   - Ensure trust relationships are correct

3. **Cold Start Performance**
   - Pre-warm Lambda with scheduled events
   - Increase memory allocation
   - Enable provisioned concurrency

### Debug Logging

Enable debug logs:
```go
os.Setenv("DYNAMORM_DEBUG", "true")
```

## Cost Optimization

1. **Use On-Demand Tables**: Better for unpredictable workloads
2. **Enable Auto-Scaling**: For provisioned capacity
3. **Batch Operations**: Reduce API calls
4. **Efficient Queries**: Use indexes properly

## Next Steps

- Implement caching with ElastiCache
- Add API Gateway integration
- Set up CI/CD pipeline
- Configure CloudWatch alarms 