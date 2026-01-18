package services

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/theory-cloud/tabletheory/examples/blog/models"
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

// NotificationService handles sending notifications
type NotificationService struct {
	ctx         context.Context
	queue       chan *Notification
	cancel      context.CancelFunc
	providers   []NotificationProvider
	wg          sync.WaitGroup
	workers     int
	providersMu sync.RWMutex
}

// NewNotificationService creates a new notification service
func NewNotificationService(workers int) *NotificationService {
	if workers <= 0 {
		workers = 5
	}

	ctx, cancel := context.WithCancel(context.Background())

	service := &NotificationService{
		providers: make([]NotificationProvider, 0),
		queue:     make(chan *Notification, 1000),
		workers:   workers,
		ctx:       ctx,
		cancel:    cancel,
	}

	// Start workers
	service.startWorkers()

	return service
}

// RegisterProvider registers a notification provider
func (s *NotificationService) RegisterProvider(provider NotificationProvider) {
	s.providersMu.Lock()
	defer s.providersMu.Unlock()
	s.providers = append(s.providers, provider)
	log.Printf("Registered notification provider: %s", provider.Name())
}

// SendCommentModerationNotification sends a notification to moderators about a new comment
func (s *NotificationService) SendCommentModerationNotification(comment *models.Comment, post *models.Post) error {
	notification := &Notification{
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

	return s.Send(notification)
}

// SendCommentApprovalNotification sends a notification to the comment author when approved
func (s *NotificationService) SendCommentApprovalNotification(comment *models.Comment, post *models.Post) error {
	notification := &Notification{
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

	return s.Send(notification)
}

// Send adds a notification to the queue for async processing
func (s *NotificationService) Send(notification *Notification) error {
	select {
	case s.queue <- notification:
		return nil
	case <-s.ctx.Done():
		return fmt.Errorf("notification service is shutting down")
	default:
		return fmt.Errorf("notification queue is full")
	}
}

// SendSync sends a notification synchronously
func (s *NotificationService) SendSync(ctx context.Context, notification *Notification) error {
	s.providersMu.RLock()
	defer s.providersMu.RUnlock()
	for _, provider := range s.providers {
		if provider.CanHandle(notification) {
			if err := provider.Send(ctx, notification); err != nil {
				notification.Status = NotificationStatusFailed
				notification.Error = err.Error()
				notification.LastAttempt = time.Now()
				notification.Attempts++
				return err
			}
			notification.Status = NotificationStatusSent
			notification.SentAt = time.Now()
			return nil
		}
	}
	return fmt.Errorf("no provider available for notification type: %s", notification.Type)
}

// startWorkers starts the background workers
func (s *NotificationService) startWorkers() {
	for i := 0; i < s.workers; i++ {
		s.wg.Add(1)
		go s.worker(i)
	}
}

// worker processes notifications from the queue
func (s *NotificationService) worker(id int) {
	defer s.wg.Done()

	for {
		select {
		case notification, ok := <-s.queue:
			if !ok {
				// Channel closed
				return
			}
			if notification == nil {
				// Skip nil notifications
				continue
			}
			s.processNotification(notification)
		case <-s.ctx.Done():
			return
		}
	}
}

// processNotification processes a single notification with retry logic
func (s *NotificationService) processNotification(notification *Notification) {
	maxRetries := 3
	backoff := time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		notification.Attempts = attempt + 1
		notification.LastAttempt = time.Now()

		err := s.SendSync(context.Background(), notification)
		if err == nil {
			log.Printf("Successfully sent notification %s (attempt %d)", notification.ID, attempt+1)
			return
		}

		log.Printf("Failed to send notification %s (attempt %d): %v", notification.ID, attempt+1, err)

		if attempt < maxRetries-1 {
			notification.Status = NotificationStatusRetrying
			time.Sleep(backoff)
			backoff *= 2 // Exponential backoff
		}
	}

	notification.Status = NotificationStatusFailed
	log.Printf("Failed to send notification %s after %d attempts", notification.ID, maxRetries)
}

// Shutdown gracefully shuts down the notification service
func (s *NotificationService) Shutdown() {
	s.cancel()
	close(s.queue)
	s.wg.Wait()
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
