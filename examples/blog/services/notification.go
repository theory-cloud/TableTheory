package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/theory-cloud/tabletheory/v3/examples/blog/models"
)

// NotificationType represents the type of notification
type NotificationType string

const (
	NotificationTypeCommentModeration NotificationType = "comment_moderation"
	NotificationTypeCommentApproval   NotificationType = "comment_approval"
	NotificationTypeCommentReply      NotificationType = "comment_reply"
	NotificationTypeNewPost           NotificationType = "new_post"
)

// NotificationStatus represents the delivery status
type NotificationStatus string

const (
	NotificationStatusPending  NotificationStatus = "pending"
	NotificationStatusSent     NotificationStatus = "sent"
	NotificationStatusFailed   NotificationStatus = "failed"
	NotificationStatusRetrying NotificationStatus = "retrying"
)

// Notification represents a notification to be sent
type Notification struct {
	LastAttempt time.Time             `json:"last_attempt,omitempty"`
	CreatedAt   time.Time             `json:"created_at"`
	SentAt      time.Time             `json:"sent_at,omitempty"`
	Data        map[string]any        `json:"data"`
	Recipient   NotificationRecipient `json:"recipient"`
	ID          string                `json:"id"`
	Type        NotificationType      `json:"type"`
	Subject     string                `json:"subject"`
	Content     string                `json:"content"`
	Status      NotificationStatus    `json:"status"`
	Error       string                `json:"error,omitempty"`
	Attempts    int                   `json:"attempts"`
}

// NotificationRecipient represents the recipient of a notification
type NotificationRecipient struct {
	Email   string `json:"email,omitempty"`
	Webhook string `json:"webhook,omitempty"`
	UserID  string `json:"user_id,omitempty"`
	Name    string `json:"name,omitempty"`
}

// NotificationProvider is the interface for notification providers
type NotificationProvider interface {
	Send(ctx context.Context, notification *Notification) error
	CanHandle(notification *Notification) bool
	Name() string
}

// NotificationService delivers notifications synchronously on the caller's
// goroutine. It owns no queue and starts no worker, because the blog deploys
// only as Lambda functions (see deployment/sam-template.yaml): Lambda freezes
// the execution environment the moment the handler returns, so a background
// worker would be frozen mid-flight. Every delivery therefore happens inside the
// invocation that requested it, bounded by the context that invocation passes.
type NotificationService struct {
	providers []NotificationProvider
}

// NewNotificationService creates a new notification service. It starts nothing
// and owns no queue; providers are registered on the caller's goroutine.
func NewNotificationService() *NotificationService {
	return &NotificationService{
		providers: make([]NotificationProvider, 0),
	}
}

// RegisterProvider registers a notification provider
func (s *NotificationService) RegisterProvider(provider NotificationProvider) {
	s.providers = append(s.providers, provider)
	log.Printf("Registered notification provider: %s", provider.Name())
}

const (
	// sendAttempts is the total number of delivery attempts Send makes before
	// giving up.
	sendAttempts = 3
	// sendBackoff is the wait before the second attempt; it doubles for each
	// further attempt and is always interruptible by ctx.
	sendBackoff = time.Second
)

// errNoProvider reports that no registered provider can handle the
// notification. It is a configuration error, so Send does not retry it.
var errNoProvider = errors.New("no notification provider can handle this notification")

// Send delivers a notification synchronously and returns only once the delivery
// attempt has finished. ctx bounds the whole call, including the waits between
// retries; no work is left running when Send returns. Retries are bounded to
// sendAttempts attempts with an interruptible exponential backoff.
func (s *NotificationService) Send(ctx context.Context, notification *Notification) error {
	backoff := sendBackoff

	for attempt := 1; ; attempt++ {
		notification.Attempts = attempt
		notification.LastAttempt = time.Now()

		err := s.deliver(ctx, notification)
		if err == nil {
			return nil
		}

		// The caller's deadline governs the whole call: stop retrying as soon as
		// it is gone, and a missing provider cannot be fixed by retrying.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return s.fail(notification, ctxErr)
		}
		if errors.Is(err, errNoProvider) || attempt >= sendAttempts {
			return s.fail(notification, err)
		}

		notification.Status = NotificationStatusRetrying
		select {
		case <-time.After(backoff):
			backoff *= 2
		case <-ctx.Done():
			return s.fail(notification, ctx.Err())
		}
	}
}

// deliver hands the notification to the first registered provider that can
// handle it, on the caller's goroutine.
func (s *NotificationService) deliver(ctx context.Context, notification *Notification) error {
	for _, provider := range s.providers {
		if !provider.CanHandle(notification) {
			continue
		}
		if err := provider.Send(ctx, notification); err != nil {
			notification.Error = err.Error()
			return err
		}
		notification.Status = NotificationStatusSent
		notification.SentAt = time.Now()
		return nil
	}
	return fmt.Errorf("%w: %s", errNoProvider, notification.Type)
}

// fail records a terminal delivery failure on the notification and returns err.
func (s *NotificationService) fail(notification *Notification, err error) error {
	notification.Status = NotificationStatusFailed
	notification.Error = err.Error()
	return err
}

// SendCommentModerationNotification delivers a notification to moderators about
// a new comment, synchronously and bounded by ctx.
func (s *NotificationService) SendCommentModerationNotification(ctx context.Context, comment *models.Comment, post *models.Post) error {
	return s.Send(ctx, s.buildModerationNotification(comment, post))
}

func (s *NotificationService) buildModerationNotification(comment *models.Comment, post *models.Post) *Notification {
	return &Notification{
		ID:   fmt.Sprintf("mod-%s-%d", comment.ID, time.Now().Unix()),
		Type: NotificationTypeCommentModeration,
		Recipient: NotificationRecipient{
			Email: getModerationEmail(), // This would come from config
		},
		Subject: fmt.Sprintf("New comment requires moderation on: %s", post.Title),
		Content: s.buildModerationEmailContent(comment, post),
		Data: map[string]any{
			"comment_id":   comment.ID,
			"post_id":      post.ID,
			"post_title":   post.Title,
			"author_name":  comment.AuthorName,
			"author_email": comment.AuthorEmail,
			"content":      comment.Content,
			"ip_address":   comment.IPAddress,
		},
		Status:    NotificationStatusPending,
		CreatedAt: time.Now(),
	}
}

// SendCommentApprovalNotification delivers a notification to the comment author
// when the comment is approved, synchronously and bounded by ctx.
func (s *NotificationService) SendCommentApprovalNotification(ctx context.Context, comment *models.Comment, post *models.Post) error {
	return s.Send(ctx, s.buildApprovalNotification(comment, post))
}

func (s *NotificationService) buildApprovalNotification(comment *models.Comment, post *models.Post) *Notification {
	return &Notification{
		ID:   fmt.Sprintf("apr-%s-%d", comment.ID, time.Now().Unix()),
		Type: NotificationTypeCommentApproval,
		Recipient: NotificationRecipient{
			Email: comment.AuthorEmail,
			Name:  comment.AuthorName,
		},
		Subject: fmt.Sprintf("Your comment on '%s' has been approved", post.Title),
		Content: s.buildApprovalEmailContent(comment, post),
		Data: map[string]any{
			"comment_id": comment.ID,
			"post_id":    post.ID,
			"post_title": post.Title,
			"post_slug":  post.Slug,
		},
		Status:    NotificationStatusPending,
		CreatedAt: time.Now(),
	}
}

// Helper functions for building email content

func (s *NotificationService) buildModerationEmailContent(comment *models.Comment, post *models.Post) string {
	return fmt.Sprintf(`
A new comment requires moderation.

Post: %s
Author: %s (%s)
IP Address: %s

Comment:
%s

---
Moderate this comment: %s
`, post.Title, comment.AuthorName, comment.AuthorEmail, comment.IPAddress, comment.Content, getModerationURL(comment.ID))
}

func (s *NotificationService) buildApprovalEmailContent(comment *models.Comment, post *models.Post) string {
	return fmt.Sprintf(`
Hello %s,

Your comment on the post "%s" has been approved and is now visible to other readers.

View your comment: %s

Thank you for contributing to the discussion!

Best regards,
The Blog Team
`, comment.AuthorName, post.Title, getCommentURL(post.Slug, comment.ID))
}

// Configuration helpers (these would typically come from environment/config)

func getModerationEmail() string {
	// In production, this would come from configuration
	return "moderators@example.com"
}

func getModerationURL(commentID string) string {
	// In production, this would be the actual admin URL
	return fmt.Sprintf("https://admin.example.com/comments/%s/moderate", commentID)
}

func getCommentURL(postSlug, commentID string) string {
	// In production, this would be the actual blog URL
	return fmt.Sprintf("https://blog.example.com/posts/%s#comment-%s", postSlug, commentID)
}
