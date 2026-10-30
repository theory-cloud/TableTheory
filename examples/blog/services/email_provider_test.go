package services

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"log"
	"math/big"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeSMTPOptions struct {
	startTLS bool
	auth     bool
	stall    bool
}

// fakeSMTPServer is a minimal, real TCP SMTP peer used to exercise the provider's
// delivery path without a network or a deployed service.
type fakeSMTPServer struct {
	listener  net.Listener
	addr      string
	opts      fakeSMTPOptions
	tlsConfig *tls.Config
	roots     *x509.CertPool
	received  chan []byte
	connDone  chan struct{}
	mu        sync.Mutex
	commands  []string
	sawMail   bool
}

func startFakeSMTPServer(t *testing.T, stall bool) *fakeSMTPServer {
	t.Helper()
	return startFakeSMTPServerWithOptions(t, fakeSMTPOptions{startTLS: true, auth: true, stall: stall})
}

func startFakeSMTPServerWithOptions(t *testing.T, opts fakeSMTPOptions) *fakeSMTPServer {
	t.Helper()

	cert, roots := newSMTPTestCert(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := &fakeSMTPServer{
		listener: listener,
		addr:     listener.Addr().String(),
		opts:     opts,
		tlsConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		},
		roots:    roots,
		received: make(chan []byte, 1),
		connDone: make(chan struct{}),
	}
	go server.serve()
	t.Cleanup(func() { _ = listener.Close() })

	return server
}

func (s *fakeSMTPServer) hostPort() (string, string) {
	host, port, err := net.SplitHostPort(s.addr)
	if err != nil {
		return s.addr, ""
	}
	return host, port
}

func (s *fakeSMTPServer) recordCommand(command string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commands = append(s.commands, command)
}

func (s *fakeSMTPServer) commandIndex(prefix string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, command := range s.commands {
		if strings.HasPrefix(command, prefix) {
			return index
		}
	}
	return -1
}

func (s *fakeSMTPServer) mailSeen() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sawMail
}

func (s *fakeSMTPServer) serve() {
	defer close(s.connDone)

	conn, err := s.listener.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	reader := bufio.NewReader(conn)
	writeLine := func(line string) {
		_, _ = conn.Write([]byte(line + "\r\n"))
	}

	writeLine("220 fake ESMTP")

	tlsActive := false
	stalled := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		if stalled {
			// A stalled peer keeps the connection open and consumes input
			// without ever answering, until the caller gives up and closes.
			continue
		}

		command := strings.ToUpper(strings.TrimSpace(line))
		s.recordCommand(command)

		switch {
		case strings.HasPrefix(command, "EHLO"), strings.HasPrefix(command, "HELO"):
			if s.opts.stall {
				stalled = true
				continue
			}
			if s.opts.startTLS && !tlsActive {
				writeLine("250-fake")
				writeLine("250-STARTTLS")
				writeLine("250 OK")
				continue
			}
			if s.opts.auth {
				writeLine("250-fake")
				writeLine("250 AUTH PLAIN")
				continue
			}
			writeLine("250 OK")
		case strings.HasPrefix(command, "STARTTLS"):
			if !s.opts.startTLS || tlsActive {
				writeLine("454 TLS not available")
				continue
			}
			writeLine("220 ready to start TLS")
			tlsConn := tls.Server(conn, s.tlsConfig)
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			conn = tlsConn
			reader = bufio.NewReader(conn)
			tlsActive = true
		case strings.HasPrefix(command, "AUTH"):
			if !tlsActive || !s.opts.auth {
				writeLine("503 5.7.0 AUTH not available")
				continue
			}
			writeLine("235 authenticated")
		case strings.HasPrefix(command, "MAIL"):
			s.mu.Lock()
			s.sawMail = true
			s.mu.Unlock()
			writeLine("250 ok")
		case strings.HasPrefix(command, "RCPT"):
			writeLine("250 ok")
		case strings.HasPrefix(command, "DATA"):
			writeLine("354 end with <CRLF>.<CRLF>")
			data, err := readSMTPData(reader)
			if err != nil {
				return
			}
			select {
			case s.received <- data:
			default:
			}
			writeLine("250 queued")
		case strings.HasPrefix(command, "QUIT"):
			writeLine("221 bye")
			return
		default:
			writeLine("250 ok")
		}
	}
}

func readSMTPData(reader *bufio.Reader) ([]byte, error) {
	var buf bytes.Buffer
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if strings.TrimRight(line, "\r\n") == "." {
			return buf.Bytes(), nil
		}
		buf.WriteString(line)
	}
}

func newSMTPTestCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	parsed, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	roots.AddCert(parsed)

	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, roots
}

func newTestEmailProvider(server *fakeSMTPServer) *EmailProvider {
	host, port := server.hostPort()
	provider := NewEmailProvider(EmailConfig{
		SMTPHost:     host,
		SMTPPort:     port,
		SMTPUsername: "user",
		SMTPPassword: "pass",
		FromEmail:    "notifications@example.com",
		FromName:     "Blog Notifications",
	})
	provider.testTLSConfig = &tls.Config{RootCAs: server.roots}
	return provider
}

func testNotification() *Notification {
	return &Notification{
		Recipient: NotificationRecipient{Email: "reader@example.com"},
		Subject:   "Hello",
		Content:   "Comment body",
	}
}

// TestEmailProviderStalledPeerHonorsContext proves a peer that completes the
// greeting, receives EHLO, and then stalls cannot hold the delivery past the
// caller's deadline. Before the context-aware transport this blocked until the
// Lambda/function timeout; now the call returns within the context window and
// closes the connection instead of leaking the handshake goroutine.
func TestEmailProviderStalledPeerHonorsContext(t *testing.T) {
	server := startFakeSMTPServer(t, true)
	provider := newTestEmailProvider(server)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	start := time.Now()
	err := provider.Send(ctx, testNotification())
	elapsed := time.Since(start)

	require.Error(t, err, "a stalled SMTP peer must surface a delivery error")
	require.Less(t, elapsed, 3*time.Second, "delivery must be bounded by the context, not the 10s fallback")

	select {
	case <-server.connDone:
	case <-time.After(2 * time.Second):
		t.Fatal("provider left the stalled SMTP connection open after the context ended")
	}
}

// TestEmailProviderCanceledContextAbortsHandshake proves explicit cancellation
// interrupts a stalled handshake promptly, with no goroutine left behind.
func TestEmailProviderCanceledContextAbortsHandshake(t *testing.T) {
	server := startFakeSMTPServer(t, true)
	provider := newTestEmailProvider(server)

	ctx, cancel := context.WithCancel(context.Background())

	baseline := runtime.NumGoroutine()
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := provider.Send(ctx, testNotification())
	elapsed := time.Since(start)

	require.Error(t, err)
	require.Less(t, elapsed, 2*time.Second, "cancellation must abort the handshake promptly")

	select {
	case <-server.connDone:
	case <-time.After(2 * time.Second):
		t.Fatal("provider left the canceled SMTP connection open")
	}

	require.Eventually(t, func() bool {
		return runtime.NumGoroutine() <= baseline+1
	}, 2*time.Second, 25*time.Millisecond, "delivery must not leak goroutines")
}

// TestEmailProviderPreventsHeaderInjection proves line breaks in untrusted
// fields cannot add SMTP headers. A post title becomes a Subject and a commenter
// name becomes a display name, so both carry attacker-chosen text: one newline
// ends the field and starts one of the attacker's choosing, turning a
// notification into a spoofed message with an extra Bcc recipient.
func TestEmailProviderPreventsHeaderInjection(t *testing.T) {
	server := startFakeSMTPServer(t, false)
	provider := newTestEmailProvider(server)

	notification := &Notification{
		Recipient: NotificationRecipient{
			Email: "reader@example.com",
			Name:  "Reader\r\nBcc: attacker@evil.example",
		},
		Subject: "Hello\r\nBcc: attacker@evil.example\r\n\r\nForged body",
		Content: "Comment body",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, provider.Send(ctx, notification))

	select {
	case data := <-server.received:
		headers, body, ok := strings.Cut(string(data), "\r\n\r\n")
		require.True(t, ok, "message must keep a header/body separator")

		// The attacker text survives as inert words inside a header value, which is
		// the point: it must stay inside the field it was written for. What it must
		// not do is become a field of its own, so assert the exact header set.
		lines := strings.Split(headers, "\r\n")
		require.Len(t, lines, 4, "a newline must not add a header")
		require.True(t, strings.HasPrefix(lines[0], "From: "))
		require.True(t, strings.HasPrefix(lines[1], "To: "))
		require.True(t, strings.HasPrefix(lines[2], "Subject: Hello"))
		require.True(t, strings.HasPrefix(lines[3], "Content-Type: "))

		require.NotContains(t, body, "Forged body", "a newline must not replace the body")
		require.Contains(t, body, "Comment body")
	case <-time.After(2 * time.Second):
		t.Fatal("fast SMTP peer did not receive the message")
	}
}

// TestEmailProviderRejectsLineBreaksInAddress proves an address that carries a
// line break is refused instead of reaching the SMTP envelope commands, where a
// newline would start a second RCPT TO.
func TestEmailProviderRejectsLineBreaksInAddress(t *testing.T) {
	server := startFakeSMTPServer(t, false)
	provider := newTestEmailProvider(server)

	notification := &Notification{
		Recipient: NotificationRecipient{Email: "reader@example.com\r\nRCPT TO:<attacker@evil.example>"},
		Subject:   "Hello",
		Content:   "Comment body",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := provider.Send(ctx, notification)
	require.Error(t, err, "an address carrying a line break must be refused")
	require.Contains(t, err.Error(), "line break")
}

// TestEmailProviderTestModeLogsOneEntry proves untrusted content cannot forge log
// records: newlines in the subject or content stay inside the entry that the
// provider emitted instead of starting a record of the attacker's making.
func TestEmailProviderTestModeLogsOneEntry(t *testing.T) {
	var logged bytes.Buffer
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	provider := NewEmailProvider(EmailConfig{TestMode: true})
	notification := &Notification{
		Recipient: NotificationRecipient{Email: "reader@example.com"},
		Subject:   "Hello\nlevel=ERROR forged entry",
		Content:   "Comment body\r\nlevel=ERROR forged body",
	}

	require.NoError(t, provider.Send(context.Background(), notification))
	require.NotContains(t, logged.String(), "\nlevel=ERROR forged entry")
	require.NotContains(t, logged.String(), "\nlevel=ERROR forged body")
}

// TestEmailProviderFastPeerDeliversMessage is the ordinary authorized-success
// control: a healthy peer receives the exact message.
func TestEmailProviderFastPeerDeliversMessage(t *testing.T) {
	server := startFakeSMTPServer(t, false)
	provider := newTestEmailProvider(server)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	notification := testNotification()
	require.NoError(t, provider.Send(ctx, notification))

	select {
	case data := <-server.received:
		body := string(data)
		require.Contains(t, body, "Subject: Hello")
		require.Contains(t, body, "Comment body")
		require.Contains(t, body, "To: reader@example.com")
	case <-time.After(2 * time.Second):
		t.Fatal("fast SMTP peer did not receive the message")
	}
}

func TestEmailProviderRequiresSTARTTLSForNonTestDelivery(t *testing.T) {
	server := startFakeSMTPServerWithOptions(t, fakeSMTPOptions{startTLS: false, auth: true})
	provider := newTestEmailProvider(server)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := provider.Send(ctx, testNotification())
	require.Error(t, err)
	require.Contains(t, err.Error(), "STARTTLS")
	require.False(t, server.mailSeen(), "MAIL must not be issued without STARTTLS")
	select {
	case <-server.received:
		t.Fatal("message must not be sent without STARTTLS")
	default:
	}
}

func TestEmailProviderRequiresAUTHAfterTLSWhenCredentialsExist(t *testing.T) {
	server := startFakeSMTPServerWithOptions(t, fakeSMTPOptions{startTLS: true, auth: false})
	provider := newTestEmailProvider(server)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := provider.Send(ctx, testNotification())
	require.Error(t, err)
	require.Contains(t, err.Error(), "AUTH")
	require.False(t, server.mailSeen(), "MAIL must not be issued without AUTH")
	select {
	case <-server.received:
		t.Fatal("message must not be sent without AUTH")
	default:
	}
}

func TestEmailProviderRejectsIncompleteCredentials(t *testing.T) {
	provider := NewEmailProvider(EmailConfig{
		SMTPHost:     "127.0.0.1",
		SMTPPort:     "2525",
		SMTPUsername: "user",
		FromEmail:    "notifications@example.com",
	})

	err := provider.Send(context.Background(), testNotification())
	require.Error(t, err)
	require.Contains(t, err.Error(), "both")
}

func TestEmailProviderSecureDeliveryOrdersSTARTTLSAndAUTHBeforeMAIL(t *testing.T) {
	server := startFakeSMTPServer(t, false)
	provider := newTestEmailProvider(server)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, provider.Send(ctx, testNotification()))

	startTLS := server.commandIndex("STARTTLS")
	auth := server.commandIndex("AUTH")
	mail := server.commandIndex("MAIL")
	require.True(t, startTLS >= 0, "STARTTLS must be issued")
	require.True(t, auth > startTLS, "AUTH must follow STARTTLS")
	require.True(t, mail > auth, "MAIL must follow AUTH")

	select {
	case data := <-server.received:
		require.Contains(t, string(data), "Comment body")
	case <-time.After(2 * time.Second):
		t.Fatal("secure SMTP peer did not receive the message")
	}
}

// TestEmailProviderSecureUnauthenticatedRelayWithoutCredentials proves the
// no-credentials path still works for a STARTTLS relay that does not require
// AUTH. Credentials are what trigger the AUTH requirement.
func TestEmailProviderSecureUnauthenticatedRelayWithoutCredentials(t *testing.T) {
	server := startFakeSMTPServerWithOptions(t, fakeSMTPOptions{startTLS: true, auth: false})
	host, port := server.hostPort()
	provider := NewEmailProvider(EmailConfig{
		SMTPHost:  host,
		SMTPPort:  port,
		FromEmail: "notifications@example.com",
	})
	provider.testTLSConfig = &tls.Config{RootCAs: server.roots}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, provider.Send(ctx, testNotification()))

	select {
	case data := <-server.received:
		require.Contains(t, string(data), "Comment body")
	case <-time.After(2 * time.Second):
		t.Fatal("STARTTLS relay without credentials did not receive the message")
	}
}
