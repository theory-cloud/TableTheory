# Blog Notification Service

## Overview

The notification service provides a flexible and extensible system for sending notifications in the blog application. It supports multiple notification providers and delivers every notification synchronously, inside the invocation that requested it.

## Features

- **Multiple Providers**: Support for email, webhook, and custom notification providers
- **Synchronous Delivery**: `Send` and the `SendComment*Notification` helpers deliver on the caller's goroutine and return only once the delivery attempt has finished, which is what a Lambda handler must use: Lambda freezes the execution environment the moment the handler returns
- **Context-Bounded**: The caller's context bounds the whole delivery, including the waits between retries, so a slow provider cannot outlive its invocation
- **Retry Logic**: Bounded retry with an interruptible exponential backoff (3 attempts, 1 s doubling)
- **Provider Interface**: Easy to extend with new notification providers
- **Test Mode**: Built-in test mode for development and testing

## Architecture

```
NotificationService
├── Provider Interface
│   ├── EmailProvider
│   ├── WebhookProvider
│   └── (Custom Providers)
└── Bounded Retry Logic
```

The service owns no queue and starts no goroutine. The blog example deploys only as Lambda functions (`deployment/sam-template.yaml` defines the posts and comments functions), so nothing keeps running after a handler returns to drain a queue: work handed to a background goroutine would be frozen mid-flight when the handler returns.

## Usage

### Initialize the Service

```go
// Create the notification service. It starts nothing.
notificationService := services.NewNotificationService()

// Configure email provider
emailConfig := services.EmailConfig{
    SMTPHost:     "smtp.example.com",
    SMTPPort:     "587",
    SMTPUsername: "user@example.com",
    SMTPPassword: "password",
    FromEmail:    "noreply@example.com",
    FromName:     "Blog Notifications",
    TestMode:     false, // Set to true for testing
}
emailProvider := services.NewEmailProvider(emailConfig)
notificationService.RegisterProvider(emailProvider)

// Configure webhook provider
webhookConfig := services.WebhookConfig{
    DefaultWebhookURL: "https://api.example.com/webhooks",
    SigningSecret:     "webhook-secret",
    TestMode:          false,
}
webhookProvider := services.NewWebhookProvider(webhookConfig)
notificationService.RegisterProvider(webhookProvider)
```

### Send Notifications

```go
// Deliver inside the invocation. The call is bounded by ctx, and nothing is
// left running when it returns.
err := notificationService.SendCommentModerationNotification(ctx, comment, post)

err := notificationService.SendCommentApprovalNotification(ctx, comment, post)

// Send a custom notification
notification := &Notification{
    Type: NotificationTypeCommentReply,
    Recipient: NotificationRecipient{
        Email: "user@example.com",
        Name:  "John Doe",
    },
    Subject: "New reply to your comment",
    Content: "Someone replied to your comment...",
    Data: map[string]interface{}{
        "comment_id": "123",
        "reply_id":   "456",
    },
}
err := notificationService.Send(ctx, notification)
```

### Environment Variables

```bash
# Email Provider Configuration
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=your-email@gmail.com
SMTP_PASSWORD=your-app-password
FROM_EMAIL=noreply@yourblog.com

# Webhook Provider Configuration
WEBHOOK_URL=https://api.yourservice.com/webhooks
WEBHOOK_SECRET=your-webhook-secret

# Test Mode (logs notifications instead of sending)
NOTIFICATION_TEST_MODE=true
```

## Implementing a Custom Provider

To add a new notification provider, implement the `NotificationProvider` interface:

```go
type NotificationProvider interface {
    Send(ctx context.Context, notification *Notification) error
    CanHandle(notification *Notification) bool
    Name() string
}
```

Example SMS provider:

```go
type SMSProvider struct {
    client *sms.Client
}

func (p *SMSProvider) Send(ctx context.Context, notification *Notification) error {
    if notification.Recipient.Phone == "" {
        return fmt.Errorf("phone number required")
    }
    
    return p.client.SendSMS(
        notification.Recipient.Phone,
        notification.Content,
    )
}

func (p *SMSProvider) CanHandle(notification *Notification) bool {
    return notification.Recipient.Phone != ""
}

func (p *SMSProvider) Name() string {
    return "SMSProvider"
}
```

A provider's `Send` runs on the caller's goroutine, so it must return before the invocation ends and should honor the context it is given.

## Testing

The service includes comprehensive tests and a mock provider for testing:

```go
// Create mock provider for testing
mockProvider := NewMockProvider("test")
service := NewNotificationService()
service.RegisterProvider(mockProvider)

// Send notification
err := service.SendCommentModerationNotification(context.Background(), comment, post)

// Delivery already finished when Send returned
assert.Len(t, mockProvider.sentNotifications, 1)
```

## Retry Logic

A failed delivery is retried with an interruptible exponential backoff:
- Maximum attempts: 3
- Initial backoff: 1 second
- Backoff multiplier: 2x
- The wait is cut short by the caller's context, so the whole `Send` stays inside the invoking handler's deadline

## Performance Considerations

- **Timeout**: Pass a context with the deadline your invocation can afford
- **Provider Timeouts**: Configure provider timeouts appropriately
- **Failure Handling**: A delivery failure is returned to the caller; the blog handlers log it without failing the request

## Integration Status

✅ **Completed**:
- Comment moderation notifications
- Comment approval notifications
- Email provider implementation
- Webhook provider implementation
- Synchronous delivery with bounded retry logic
- Comprehensive test coverage

🚧 **TODO**:
- AWS SES provider
- SMS provider (Twilio/SNS)
- Push notification provider
- Notification preferences per user
- Notification templates
