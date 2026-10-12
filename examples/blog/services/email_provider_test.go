package services

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeSMTPServer is a minimal, real TCP SMTP peer used to exercise the provider's
// delivery path without a network or a deployed service.
type fakeSMTPServer struct {
	listener net.Listener
	addr     string
	stall    bool
	received chan []byte
	connDone chan struct{}
}

func startFakeSMTPServer(t *testing.T, stall bool) *fakeSMTPServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := &fakeSMTPServer{
		listener: listener,
		addr:     listener.Addr().String(),
		stall:    stall,
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

		switch command := strings.ToUpper(strings.TrimSpace(line)); {
		case strings.HasPrefix(command, "EHLO"), strings.HasPrefix(command, "HELO"):
			if s.stall {
				stalled = true
				continue
			}
			writeLine("250-fake")
			writeLine("250 AUTH PLAIN")
		case strings.HasPrefix(command, "AUTH"):
			writeLine("235 authenticated")
		case strings.HasPrefix(command, "MAIL"), strings.HasPrefix(command, "RCPT"):
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

func newTestEmailProvider(server *fakeSMTPServer) *EmailProvider {
	host, port := server.hostPort()
	return NewEmailProvider(EmailConfig{
		SMTPHost:     host,
		SMTPPort:     port,
		SMTPUsername: "user",
		SMTPPassword: "pass",
		FromEmail:    "notifications@example.com",
		FromName:     "Blog Notifications",
	})
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
