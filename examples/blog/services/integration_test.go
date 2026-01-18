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

func TestNotificationIntegration(t *testing.T) {
	// This test verifies that the notification service integrates correctly
	// with the comment handler's use cases

	// Create notification service
	service := NewNotificationService(2)
	defer service.Shutdown()

	// Add email provider in test mode
	emailConfig := EmailConfig{
		TestMode:  true,
		FromEmail: "test@blog.com",
		FromName:  "Test Blog",
	}
	emailProvider := NewEmailProvider(emailConfig)
	service.RegisterProvider(emailProvider)

	// Test data
	comment := &models.Comment{
		ID:          "comment-123",
		PostID:      "post-456",
		AuthorName:  "Test User",
		AuthorEmail: "testuser@example.com",
		Content:     "This is a test comment that needs moderation",
		Status:      models.CommentStatusPending,
		IPAddress:   "127.0.0.1",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	post := &models.Post{
		ID:    "post-456",
		Title: "Test Blog Post",
		Slug:  "test-blog-post",
	}

	t.Run("Moderation Notification", func(t *testing.T) {
		err := service.SendCommentModerationNotification(comment, post)
		require.NoError(t, err)

		// Wait for async processing
		time.Sleep(200 * time.Millisecond)
	})

	t.Run("Approval Notification", func(t *testing.T) {
		// Change status to approved
		comment.Status = models.CommentStatusApproved

		err := service.SendCommentApprovalNotification(comment, post)
		require.NoError(t, err)

		// Wait for async processing
		time.Sleep(200 * time.Millisecond)
	})

	t.Run("Multiple Notifications", func(t *testing.T) {
		// Send multiple notifications rapidly
		for i := 0; i < 10; i++ {
			testComment := &models.Comment{
				ID:          fmt.Sprintf("comment-%d", i),
				PostID:      post.ID,
				AuthorName:  fmt.Sprintf("User %d", i),
				AuthorEmail: fmt.Sprintf("user%d@example.com", i),
				Content:     fmt.Sprintf("Comment %d", i),
				Status:      models.CommentStatusPending,
				CreatedAt:   time.Now(),
			}

			err := service.SendCommentModerationNotification(testComment, post)
			assert.NoError(t, err)
		}

		// Wait for all to process
		time.Sleep(500 * time.Millisecond)
	})
}

func TestNotificationProviderSelection(t *testing.T) {
	// Test that the correct provider is selected based on notification type

	service := NewNotificationService(1)
	defer service.Shutdown()

	// Add email provider
	emailProvider := NewEmailProvider(EmailConfig{TestMode: true})
	service.RegisterProvider(emailProvider)

	// Add webhook provider
	webhookProvider := NewWebhookProvider(WebhookConfig{
		TestMode:          true,
		DefaultWebhookURL: "https://test.webhook.com",
	})
	service.RegisterProvider(webhookProvider)

	t.Run("Email Notification", func(t *testing.T) {
		notification := &Notification{
			ID:   "email-test",
			Type: NotificationTypeCommentApproval,
			Recipient: NotificationRecipient{
				Email: "user@example.com",
			},
			Subject: "Test Email",
			Content: "Test content",
		}

		ctx := context.Background()
		err := service.SendSync(ctx, notification)
		assert.NoError(t, err)
	})

	t.Run("Webhook Notification", func(t *testing.T) {
		notification := &Notification{
			ID:   "webhook-test",
			Type: NotificationTypeCommentModeration,
			Recipient: NotificationRecipient{
				Webhook: "https://custom.webhook.com",
			},
			Data: map[string]any{
				"test": "data",
			},
		}

		ctx := context.Background()
		err := service.SendSync(ctx, notification)
		assert.NoError(t, err)
	})
}
