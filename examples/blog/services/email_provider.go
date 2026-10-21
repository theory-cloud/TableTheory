package services

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// defaultSMTPTimeout bounds a delivery attempt when the caller supplied no
// deadline, so a stalled SMTP peer can never hold the calling goroutine open
// indefinitely.
const defaultSMTPTimeout = 10 * time.Second

// flattenToLine returns value as a single line by replacing each CR and LF with
// a space. The provider frames a message as CRLF-terminated header fields, a
// blank line, then the body, so a value that carries CR or LF can close its field
// and start one of the sender's choosing - an extra Bcc recipient, a replacement
// Content-Type, or a body that replaces the real one. Subjects and display names
// are built from attacker-controlled text (a post title, a commenter's name), so
// they must be flattened before being framed. A space keeps the surrounding words
// apart; nothing else is rewritten, because the rest of the value is display
// text and stays as the user wrote it.
func flattenToLine(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	return strings.ReplaceAll(value, "\n", " ")
}

// validateAddress refuses an address that carries a line break instead of
// repairing it. An address is not display text: it is a machine identifier that
// the provider hands to the SMTP envelope commands, where a newline would start a
// second RCPT TO. Repairing a malformed address would silently deliver to an
// address nobody wrote, so the caller gets an error instead.
func validateAddress(label, address string) (string, error) {
	if strings.ContainsAny(address, "\r\n") {
		return "", fmt.Errorf("%s must not contain a line break", label)
	}
	return address, nil
}

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

	recipientEmail, err := validateAddress("recipient email", notification.Recipient.Email)
	if err != nil {
		return err
	}
	fromEmail, err := validateAddress("sender email", p.config.FromEmail)
	if err != nil {
		return err
	}

	// Subject and display names are free text, so they are cleaned rather than
	// refused: flattening the line breaks is what makes them safe to frame.
	subject := flattenToLine(notification.Subject)
	fromName := flattenToLine(p.config.FromName)
	recipientName := flattenToLine(notification.Recipient.Name)

	// In test mode, just log the email. The body is flattened too: a newline in
	// it would otherwise start a log entry that the sender never emitted.
	if p.config.TestMode {
		log.Printf("TEST MODE - Email notification:\nTo: %s\nSubject: %s\nContent:\n%s\n",
			recipientEmail,
			subject,
			flattenToLine(notification.Content))
		return nil
	}

	// Prepare email
	from := fmt.Sprintf("%s <%s>", fromName, fromEmail)
	to := recipientEmail
	if recipientName != "" {
		to = fmt.Sprintf("%s <%s>", recipientName, recipientEmail)
	}

	// Build email message. The content sits after the blank line that separates
	// headers from body, so its own line breaks are ordinary body text and cannot
	// start a header; net/smtp additionally dot-stuffs a leading period so the
	// body cannot end the MAIL transaction early.
	message := []byte(fmt.Sprintf(
		"From: %s\r\n"+
			"To: %s\r\n"+
			"Subject: %s\r\n"+
			"Content-Type: text/plain; charset=UTF-8\r\n"+
			"\r\n"+
			"%s",
		from, to, subject, notification.Content,
	))

	// Give an unbounded caller a finite ceiling of its own.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultSMTPTimeout)
		defer cancel()
	}

	addr := net.JoinHostPort(p.config.SMTPHost, p.config.SMTPPort)
	if err := p.deliver(ctx, addr, fromEmail, recipientEmail, message); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	log.Printf("Email sent successfully to %s", recipientEmail)
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
