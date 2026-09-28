package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/examples/blog/models"
)

// MockProvider is a mock notification provider for testing. Delivery is
// synchronous on the caller's goroutine, so it needs no locking.
type MockProvider struct {
	name              string
	sentNotifications []*Notification
	shouldFail        bool
	failFirst         int
	calls             int
}

func NewMockProvider(name string) *MockProvider {
	return &MockProvider{
		sentNotifications: make([]*Notification, 0),
		name:              name,
	}
}

func (m *MockProvider) Send(ctx context.Context, notification *Notification) error {
	m.calls++
	if m.shouldFail || m.calls <= m.failFirst {
		return fmt.Errorf("mock provider error")
	}
	m.sentNotifications = append(m.sentNotifications, notification)
	return nil
}

func (m *MockProvider) CanHandle(notification *Notification) bool {
	return true
}

func (m *MockProvider) Name() string {
	return m.name
}

func TestNotificationService_SendCommentModerationNotification(t *testing.T) {
	service := NewNotificationService()

	mockProvider := NewMockProvider("mock")
	service.RegisterProvider(mockProvider)

	comment := &models.Comment{
		ID:          "test-comment-1",
		PostID:      "test-post-1",
		AuthorName:  "John Doe",
		AuthorEmail: "john@example.com",
		Content:     "This is a test comment",
		Status:      models.CommentStatusPending,
		IPAddress:   "192.168.1.1",
		CreatedAt:   time.Now(),
	}

	post := &models.Post{
		ID:    "test-post-1",
		Title: "Test Blog Post",
		Slug:  "test-blog-post",
	}

	// Delivery is synchronous: when this returns, the notification has been sent.
	err := service.SendCommentModerationNotification(context.Background(), comment, post)
	require.NoError(t, err)

	assert.Len(t, mockProvider.sentNotifications, 1)
	sent := mockProvider.sentNotifications[0]
	assert.Equal(t, NotificationTypeCommentModeration, sent.Type)
	assert.Equal(t, NotificationStatusSent, sent.Status)
	assert.Contains(t, sent.Subject, "Test Blog Post")
	assert.Contains(t, sent.Content, "John Doe")
	assert.Contains(t, sent.Content, "test comment")
}

func TestNotificationService_SendCommentApprovalNotification(t *testing.T) {
	service := NewNotificationService()

	mockProvider := NewMockProvider("mock")
	service.RegisterProvider(mockProvider)

	comment := &models.Comment{
		ID:          "test-comment-2",
		PostID:      "test-post-2",
		AuthorName:  "Jane Smith",
		AuthorEmail: "jane@example.com",
		Content:     "Great article!",
		Status:      models.CommentStatusApproved,
		CreatedAt:   time.Now(),
	}

	post := &models.Post{
		ID:    "test-post-2",
		Title: "Another Test Post",
		Slug:  "another-test-post",
	}

	err := service.SendCommentApprovalNotification(context.Background(), comment, post)
	require.NoError(t, err)

	assert.Len(t, mockProvider.sentNotifications, 1)
	sent := mockProvider.sentNotifications[0]
	assert.Equal(t, NotificationTypeCommentApproval, sent.Type)
	assert.Equal(t, "jane@example.com", sent.Recipient.Email)
	assert.Contains(t, sent.Subject, "Another Test Post")
	assert.Contains(t, sent.Subject, "approved")
}

func TestNotificationService_RetriesUntilSuccess(t *testing.T) {
	service := NewNotificationService()

	// The provider fails its first attempt and succeeds on the retry.
	mockProvider := NewMockProvider("mock")
	mockProvider.failFirst = 1
	service.RegisterProvider(mockProvider)

	notification := &Notification{
		ID:        "test-notification",
		Type:      NotificationTypeCommentModeration,
		Recipient: NotificationRecipient{Email: "test@example.com"},
		Subject:   "Test Subject",
		Content:   "Test Content",
		Status:    NotificationStatusPending,
		CreatedAt: time.Now(),
	}

	err := service.Send(context.Background(), notification)
	require.NoError(t, err)
	assert.Equal(t, 2, mockProvider.calls)
	assert.Equal(t, 2, notification.Attempts)
	assert.Equal(t, NotificationStatusSent, notification.Status)
	assert.Len(t, mockProvider.sentNotifications, 1)
}

func TestNotificationService_RetryIsBoundedByContext(t *testing.T) {
	service := NewNotificationService()

	mockProvider := NewMockProvider("mock")
	mockProvider.shouldFail = true
	service.RegisterProvider(mockProvider)

	notification := &Notification{
		ID:        "test-notification",
		Type:      NotificationTypeCommentModeration,
		Recipient: NotificationRecipient{Email: "test@example.com"},
		CreatedAt: time.Now(),
	}

	// The backoff wait must be interruptible: a short deadline returns quickly
	// instead of sleeping through every attempt.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := service.Send(ctx, notification)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, NotificationStatusFailed, notification.Status)
	assert.Less(t, mockProvider.calls, sendAttempts)
}

func TestNotificationService_NoProviderIsNotRetried(t *testing.T) {
	service := NewNotificationService()

	notification := &Notification{
		ID:        "test-notification",
		Type:      NotificationTypeCommentModeration,
		CreatedAt: time.Now(),
	}

	err := service.Send(context.Background(), notification)
	require.Error(t, err)
	assert.ErrorIs(t, err, errNoProvider)
	assert.Equal(t, 1, notification.Attempts)
	assert.Equal(t, NotificationStatusFailed, notification.Status)
}

func TestEmailProvider_TestMode(t *testing.T) {
	config := EmailConfig{
		TestMode:  true,
		FromEmail: "test@example.com",
		FromName:  "Test Sender",
	}
	provider := NewEmailProvider(config)

	notification := &Notification{
		Recipient: NotificationRecipient{
			Email: "recipient@example.com",
			Name:  "Test Recipient",
		},
		Subject: "Test Email",
		Content: "This is a test email",
	}

	err := provider.Send(context.Background(), notification)
	assert.NoError(t, err)
}

func TestWebhookProvider_TestMode(t *testing.T) {
	config := WebhookConfig{
		TestMode:          true,
		DefaultWebhookURL: "https://example.com/webhook",
		SigningSecret:     "test-secret",
	}
	provider := NewWebhookProvider(config)

	notification := &Notification{
		ID:   "test-webhook",
		Type: NotificationTypeCommentModeration,
		Data: map[string]any{
			"comment_id": "123",
			"post_id":    "456",
		},
	}

	err := provider.Send(context.Background(), notification)
	assert.NoError(t, err)
}
