package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4/examples/blog/models"
)

func TestNotificationIntegration(t *testing.T) {
	// This test verifies that the notification service integrates correctly
	// with the comment handler's use cases. Delivery is synchronous, so each
	// call returns only after the notification has been handled.

	service := NewNotificationService()

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
		err := service.SendCommentModerationNotification(context.Background(), comment, post)
		require.NoError(t, err)
	})

	t.Run("Approval Notification", func(t *testing.T) {
		// Change status to approved
		comment.Status = models.CommentStatusApproved

		err := service.SendCommentApprovalNotification(context.Background(), comment, post)
		require.NoError(t, err)
	})

	t.Run("Multiple Notifications", func(t *testing.T) {
		// Send multiple notifications in sequence
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

			err := service.SendCommentModerationNotification(context.Background(), testComment, post)
			assert.NoError(t, err)
		}
	})
}

func TestNotificationProviderSelection(t *testing.T) {
	// Test that the correct provider is selected based on notification type

	service := NewNotificationService()

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

		err := service.Send(context.Background(), notification)
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

		err := service.Send(context.Background(), notification)
		assert.NoError(t, err)
	})
}
