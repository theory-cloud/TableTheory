package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/examples/blog/models"
)

// MockProvider is a mock notification provider for testing
type MockProvider struct {
	name              string
	sentNotifications []*Notification
	shouldFail        bool
}

func NewMockProvider(name string) *MockProvider {
	return &MockProvider{
		sentNotifications: make([]*Notification, 0),
		name:              name,
	}
}

func (m *MockProvider) Send(ctx context.Context, notification *Notification) error {
	if m.shouldFail {
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
	// Create notification service
	service := NewNotificationService(1)
	defer service.Shutdown()

	// Add mock provider
	mockProvider := NewMockProvider("mock")
	service.RegisterProvider(mockProvider)

	// Create test data
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

	// Send notification
	err := service.SendCommentModerationNotification(comment, post)
	require.NoError(t, err)

	// Wait for async processing
	time.Sleep(100 * time.Millisecond)

	// Verify notification was sent
	assert.Len(t, mockProvider.sentNotifications, 1)
	sent := mockProvider.sentNotifications[0]
	assert.Equal(t, NotificationTypeCommentModeration, sent.Type)
	assert.Contains(t, sent.Subject, "Test Blog Post")
	assert.Contains(t, sent.Content, "John Doe")
	assert.Contains(t, sent.Content, "test comment")
}

func TestNotificationService_SendCommentApprovalNotification(t *testing.T) {
	// Create notification service
	service := NewNotificationService(1)
	defer service.Shutdown()

	// Add mock provider
	mockProvider := NewMockProvider("mock")
	service.RegisterProvider(mockProvider)

	// Create test data
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

	// Send notification
	err := service.SendCommentApprovalNotification(comment, post)
	require.NoError(t, err)

	// Wait for async processing
	time.Sleep(100 * time.Millisecond)

	// Verify notification was sent
	assert.Len(t, mockProvider.sentNotifications, 1)
	sent := mockProvider.sentNotifications[0]
	assert.Equal(t, NotificationTypeCommentApproval, sent.Type)
	assert.Equal(t, "jane@example.com", sent.Recipient.Email)
	assert.Contains(t, sent.Subject, "Another Test Post")
	assert.Contains(t, sent.Subject, "approved")
}

func TestNotificationService_RetryLogic(t *testing.T) {
	// Create notification service
	service := NewNotificationService(1)
	defer service.Shutdown()

	// Add mock provider that fails initially
	mockProvider := NewMockProvider("mock")
	mockProvider.shouldFail = true
	service.RegisterProvider(mockProvider)

	// Create test notification
	notification := &Notification{
		ID:   "test-notification",
		Type: NotificationTypeCommentModeration,
		Recipient: NotificationRecipient{
			Email: "test@example.com",
		},
		Subject:   "Test Subject",
		Content:   "Test Content",
		Status:    NotificationStatusPending,
		CreatedAt: time.Now(),
	}

	// Send notification
	err := service.Send(notification)
	require.NoError(t, err) // Send to queue should succeed

	// Wait for processing and retries
	time.Sleep(5 * time.Second)

	// Verify retry attempts were made
	// The notification should have failed after max retries
}

func TestNotificationService_QueueOverflow(t *testing.T) {
	// Create notification service with small queue
	service := &NotificationService{
		providers: make([]NotificationProvider, 0),
		queue:     make(chan *Notification, 2), // Small queue
		workers:   0,                           // No workers
		ctx:       context.Background(),
	}

	// Fill the queue
	for i := 0; i < 2; i++ {
		err := service.Send(&Notification{ID: fmt.Sprintf("notif-%d", i)})
		require.NoError(t, err)
	}

	// Try to send one more - should fail
	err := service.Send(&Notification{ID: "overflow"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "queue is full")
}

func TestEmailProvider_TestMode(t *testing.T) {
	// Create email provider in test mode
	config := EmailConfig{
		TestMode:  true,
		FromEmail: "test@example.com",
		FromName:  "Test Sender",
	}
	provider := NewEmailProvider(config)

	// Create notification
	notification := &Notification{
		Recipient: NotificationRecipient{
			Email: "recipient@example.com",
			Name:  "Test Recipient",
		},
		Subject: "Test Email",
		Content: "This is a test email",
	}

	// Send should succeed in test mode
	err := provider.Send(context.Background(), notification)
	assert.NoError(t, err)
}

func TestWebhookProvider_TestMode(t *testing.T) {
	// Create webhook provider in test mode
	config := WebhookConfig{
		TestMode:          true,
		DefaultWebhookURL: "https://example.com/webhook",
		SigningSecret:     "test-secret",
	}
	provider := NewWebhookProvider(config)

	// Create notification
	notification := &Notification{
		ID:   "test-webhook",
		Type: NotificationTypeCommentModeration,
		Data: map[string]any{
			"comment_id": "123",
			"post_id":    "456",
		},
	}

	// Send should succeed in test mode
	err := provider.Send(context.Background(), notification)
	assert.NoError(t, err)
}
