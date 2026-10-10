package services

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"time"
)

// defaultSMTPTimeout bounds a delivery attempt when the caller supplied no
// deadline, so a stalled SMTP peer can never hold the calling goroutine open
// indefinitely.
const defaultSMTPTimeout = 10 * time.Second

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

// Send sends an email notification. The whole delivery - dial, greeting
// handshake, AUTH, and message body - is bounded by ctx, so a slow or stalled
// SMTP peer cannot outlive the request that triggered it.
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

	// Give an unbounded caller a finite ceiling of its own.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultSMTPTimeout)
		defer cancel()
	}

	addr := net.JoinHostPort(p.config.SMTPHost, p.config.SMTPPort)
	if err := p.deliver(ctx, addr, p.config.FromEmail, notification.Recipient.Email, message); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	log.Printf("Email sent successfully to %s", notification.Recipient.Email)
	return nil
}

// deliver performs the SMTP dialogue on a connection bounded by ctx. It replaces
// net/smtp.SendMail, which ignores context: the dial, the greeting/EHLO
// handshake, and every subsequent read or write inherit the caller's deadline,
// and a canceled context closes the connection instead of waiting for the peer.
// The delivery runs entirely on the caller's goroutine and leaves nothing
// running when it returns.
func (p *EmailProvider) deliver(ctx context.Context, addr, from, to string, message []byte) error {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}

	// Closing the connection is what unblocks a stalled handshake or write when
	// ctx is canceled. context.AfterFunc registers the callback without a
	// dedicated watcher goroutine, and stop() removes it before we return, so no
	// delivery work outlives this call.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer func() {
		stop()
		_ = conn.Close()
	}()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	client, err := smtp.NewClient(conn, p.config.SMTPHost)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{
			ServerName: p.config.SMTPHost,
			MinVersion: tls.VersionTLS12,
		}); err != nil {
			return err
		}
	}

	if ok, _ := client.Extension("AUTH"); ok {
		auth := smtp.PlainAuth("", p.config.SMTPUsername, p.config.SMTPPassword, p.config.SMTPHost)
		if err := client.Auth(auth); err != nil {
			return err
		}
	}

	if err := client.Mail(from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}

	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(message); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	return client.Quit()
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
