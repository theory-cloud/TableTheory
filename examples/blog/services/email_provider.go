package services

import (
	"context"
	"fmt"
	"log"
	"net/smtp"
)

// EmailConfig holds email provider configuration
type EmailConfig struct {
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	FromEmail    string
	FromName     string
	TestMode     bool // If true, logs emails instead of sending
}

// EmailProvider implements NotificationProvider for email notifications
type EmailProvider struct {
	config EmailConfig
}

// NewEmailProvider creates a new email provider
func NewEmailProvider(config EmailConfig) *EmailProvider {
	return &EmailProvider{
		config: config,
	}
}

// Send sends an email notification
func (p *EmailProvider) Send(ctx context.Context, notification *Notification) error {
	if notification.Recipient.Email == "" {
		return fmt.Errorf("recipient email is required")
	}

	// In test mode, just log the email
	if p.config.TestMode {
		log.Printf("TEST MODE - Email notification:\nTo: %s\nSubject: %s\nContent:\n%s\n",
			notification.Recipient.Email,
			notification.Subject,
			notification.Content)
		return nil
	}

	// Prepare email
	from := fmt.Sprintf("%s <%s>", p.config.FromName, p.config.FromEmail)
	to := notification.Recipient.Email
	if notification.Recipient.Name != "" {
		to = fmt.Sprintf("%s <%s>", notification.Recipient.Name, notification.Recipient.Email)
	}

	// Build email message
	message := []byte(fmt.Sprintf(
		"From: %s\r\n"+
			"To: %s\r\n"+
			"Subject: %s\r\n"+
			"Content-Type: text/plain; charset=UTF-8\r\n"+
			"\r\n"+
			"%s",
		from, to, notification.Subject, notification.Content,
	))

	// Send email via SMTP
	auth := smtp.PlainAuth("", p.config.SMTPUsername, p.config.SMTPPassword, p.config.SMTPHost)
	addr := fmt.Sprintf("%s:%s", p.config.SMTPHost, p.config.SMTPPort)

	err := smtp.SendMail(addr, auth, p.config.FromEmail, []string{notification.Recipient.Email}, message)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	log.Printf("Email sent successfully to %s", notification.Recipient.Email)
	return nil
}

// CanHandle checks if this provider can handle the notification
func (p *EmailProvider) CanHandle(notification *Notification) bool {
	// Can handle any notification with an email recipient
	return notification.Recipient.Email != ""
}

// Name returns the provider name
func (p *EmailProvider) Name() string {
	return "EmailProvider"
}
