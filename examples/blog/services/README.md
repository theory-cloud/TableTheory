# Blog Notification Service

## Overview

The notification service provides a flexible and extensible system for sending notifications in the blog application. It supports multiple notification providers and handles async processing with retry logic.

## Features

- **Multiple Providers**: Support for email, webhook, and custom notification providers
- **Async Processing**: Non-blocking notification sending with background workers
- **Retry Logic**: Automatic retry with exponential backoff for failed notifications
- **Provider Interface**: Easy to extend with new notification providers
- **Test Mode**: Built-in test mode for development and testing

## Architecture

```
NotificationService
├── Provider Interface
│   ├── EmailProvider
│   ├── WebhookProvider
│   └── (Custom Providers)
├── Async Queue
├── Worker Pool
└── Retry Logic
```

## Usage

### Initialize the Service

```go
// Create notification service with 5 workers
notificationService := services.NewNotificationService(5)

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
// Send comment moderation notification
err := notificationService.SendCommentModerationNotification(comment, post)

// Send comment approval notification
err := notificationService.SendCommentApprovalNotification(comment, post)

// Send custom notification
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
err := notificationService.Send(notification)
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

## Testing

The service includes comprehensive tests and a mock provider for testing:

```go
// Create mock provider for testing
mockProvider := NewMockProvider("test")
service.RegisterProvider(mockProvider)

// Send notification
err := service.SendCommentModerationNotification(comment, post)

// Verify notification was sent
assert.Len(t, mockProvider.sentNotifications, 1)
```

## Retry Logic

Failed notifications are automatically retried with exponential backoff:
- Maximum retries: 3
- Initial backoff: 1 second
- Backoff multiplier: 2x

## Performance Considerations

- **Worker Pool Size**: Adjust based on expected notification volume
- **Queue Size**: Default is 1000, increase for high-volume scenarios
- **Timeout**: Configure provider timeouts appropriately
- **Graceful Shutdown**: Call `Shutdown()` to process remaining notifications

## Integration Status

✅ **Completed**:
- Comment moderation notifications
- Comment approval notifications
- Email provider implementation
- Webhook provider implementation
- Async processing with retry logic
- Comprehensive test coverage

🚧 **TODO**:
- AWS SES provider
- SMS provider (Twilio/SNS)
- Push notification provider
- Notification preferences per user
- Notification templates
- Batch notification sending 