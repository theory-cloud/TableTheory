package utils

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v6"
	payment "github.com/theory-cloud/tabletheory/v6/examples/payment"
	"github.com/theory-cloud/tabletheory/v6/pkg/core"
	"github.com/theory-cloud/tabletheory/v6/pkg/session"
	"github.com/theory-cloud/tabletheory/v6/pkg/testing/fakedb"
)

type webhookTestKMS struct{}

func (webhookTestKMS) GenerateDataKey(context.Context, *kms.GenerateDataKeyInput, ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	return &kms.GenerateDataKeyOutput{
		Plaintext:      make([]byte, 32),
		CiphertextBlob: []byte("wrapped"),
	}, nil
}

func (webhookTestKMS) Decrypt(context.Context, *kms.DecryptInput, ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	return &kms.DecryptOutput{Plaintext: make([]byte, 32)}, nil
}

func newWebhookTestDB(t *testing.T) core.ExtendedDB {
	t.Helper()

	db, err := tabletheory.NewWithClient(session.Config{
		Region:    "us-east-1",
		KMSKeyARN: "arn:aws:kms:us-east-1:111111111111:key/test",
		KMSClient: webhookTestKMS{},
	}, fakedb.New())
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&payment.Merchant{}))
	require.NoError(t, db.CreateTable(&payment.Webhook{}))
	return db
}

func seedWebhookMerchant(t *testing.T, db core.ExtendedDB, webhookURL string) *payment.Merchant {
	t.Helper()

	merchant := &payment.Merchant{
		ID:            "merchant-webhook",
		Name:          "Webhook Merchant",
		Email:         "webhook@example.com",
		Status:        payment.MerchantStatusActive,
		WebhookURL:    webhookURL,
		WebhookSecret: "secret",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Version:       1,
	}
	require.NoError(t, db.Model(merchant).Create())
	return merchant
}

func publicLookup(ip string) func(context.Context, string) ([]net.IPAddr, error) {
	return func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP(ip)}}, nil
	}
}

func dialTo(serverAddr string, counter *atomic.Int32) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, _ string) (net.Conn, error) {
		if counter != nil {
			counter.Add(1)
		}
		return (&net.Dialer{}).DialContext(ctx, network, serverAddr)
	}
}

func newDestinationTestSender(
	t *testing.T,
	db core.ExtendedDB,
	lookup func(context.Context, string) ([]net.IPAddr, error),
	dial func(context.Context, string, string) (net.Conn, error),
	roots *x509.CertPool,
) *WebhookSender {
	t.Helper()

	sender := NewWebhookSender(db)
	sender.sendWait = 3 * time.Second
	if lookup != nil {
		sender.lookupIP = lookup
	}
	if dial != nil {
		sender.dial = dial
	}
	if roots != nil {
		transport := sender.client.Transport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    roots,
		}
		sender.client.Transport = transport
	}
	return sender
}

func webhookJob(merchantID string) *WebhookJob {
	return &WebhookJob{
		MerchantID: merchantID,
		EventType:  "payment.succeeded",
		PaymentID:  "payment-1",
		Data:       map[string]any{"amount": 100},
	}
}

func newWebhookTestCert(t *testing.T, dnsNames ...string) (tls.Certificate, *x509.CertPool) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: dnsNames[0]},
		DNSNames:     dnsNames,
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

func countWebhooks(t *testing.T, db core.ExtendedDB) []payment.Webhook {
	t.Helper()

	var records []payment.Webhook
	require.NoError(t, db.Model(&payment.Webhook{}).Scan(&records))
	return records
}

func TestWebhookRejectsNonPublicDestinations(t *testing.T) {
	destinations := []string{
		"http://93.184.216.34/hook",
		"https://127.0.0.1/hook",
		"https://10.0.0.5/hook",
		"https://172.16.0.1/hook",
		"https://192.168.1.1/hook",
		"https://169.254.169.254/latest/meta-data",
		"https://[::1]/hook",
		"https://0.0.0.0/hook",
		"https://100.64.0.1/hook",
		"https://198.18.0.1/hook",
		"https://240.0.0.1/hook",
		"https://user:pass@93.184.216.34/hook",
	}

	for _, destination := range destinations {
		t.Run(destination, func(t *testing.T) {
			db := newWebhookTestDB(t)
			merchant := seedWebhookMerchant(t, db, destination)

			var lookups atomic.Int32
			var dials atomic.Int32
			sender := newDestinationTestSender(t, db,
				func(context.Context, string) ([]net.IPAddr, error) {
					lookups.Add(1)
					return nil, fmt.Errorf("lookup must not run")
				},
				func(context.Context, string, string) (net.Conn, error) {
					dials.Add(1)
					return nil, fmt.Errorf("dial must not run")
				},
				nil,
			)

			err := sender.SendSync(context.Background(), webhookJob(merchant.ID))
			require.Error(t, err)
			require.Zero(t, lookups.Load(), "a refused URL must be rejected before DNS")
			require.Zero(t, dials.Load(), "a refused URL must be rejected before dialing")
			require.Empty(t, countWebhooks(t, db), "a refused destination must not create a delivery record")
		})
	}
}

func TestWebhookRejectsHostnameWithAnyNonPublicAnswer(t *testing.T) {
	db := newWebhookTestDB(t)
	merchant := seedWebhookMerchant(t, db, "https://webhook.example/hook")

	var dials atomic.Int32
	sender := newDestinationTestSender(t, db,
		func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{
				{IP: net.ParseIP("93.184.216.34")},
				{IP: net.ParseIP("10.0.0.5")},
			}, nil
		},
		func(context.Context, string, string) (net.Conn, error) {
			dials.Add(1)
			return nil, fmt.Errorf("dial must not run")
		},
		nil,
	)

	err := sender.SendSync(context.Background(), webhookJob(merchant.ID))
	require.Error(t, err)
	require.Zero(t, dials.Load())
	require.Empty(t, countWebhooks(t, db))
}

func TestWebhookRejectsDNSRebindingBetweenAttempts(t *testing.T) {
	db := newWebhookTestDB(t)
	merchant := seedWebhookMerchant(t, db, "https://webhook.example/hook")

	var lookups atomic.Int32
	var dials atomic.Int32
	sender := newDestinationTestSender(t, db,
		func(context.Context, string) ([]net.IPAddr, error) {
			if lookups.Add(1) == 1 {
				return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
			}
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		},
		func(context.Context, string, string) (net.Conn, error) {
			dials.Add(1)
			return nil, fmt.Errorf("dial must not run")
		},
		nil,
	)

	err := sender.SendSync(context.Background(), webhookJob(merchant.ID))
	require.Error(t, err)
	require.EqualValues(t, 2, lookups.Load(), "preflight and the delivery attempt must each resolve")
	require.Zero(t, dials.Load(), "a rebound private answer must be rejected before dialing")

	records := countWebhooks(t, db)
	require.Len(t, records, 1)
	require.Equal(t, payment.WebhookStatusFailed, records[0].Status)
}

func TestWebhookPublicHTTPSDeliveryPreservesSignature(t *testing.T) {
	cert, roots := newWebhookTestCert(t, "webhook.example")

	type capturedRequest struct {
		body      []byte
		timestamp string
		signature string
	}
	received := make(chan capturedRequest, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- capturedRequest{
			body:      body,
			timestamp: r.Header.Get("X-Webhook-Timestamp"),
			signature: r.Header.Get("X-Webhook-Signature"),
		}
		w.WriteHeader(http.StatusOK)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	server.StartTLS()
	defer server.Close()

	db := newWebhookTestDB(t)
	merchant := seedWebhookMerchant(t, db, "https://webhook.example/hook")

	var dials atomic.Int32
	sender := newDestinationTestSender(t, db,
		publicLookup("93.184.216.34"),
		dialTo(server.Listener.Addr().String(), &dials),
		roots,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, sender.SendSync(ctx, webhookJob(merchant.ID)))
	require.EqualValues(t, 1, dials.Load())

	select {
	case request := <-received:
		mac := hmac.New(sha256.New, []byte(merchant.WebhookSecret))
		_, _ = mac.Write([]byte(request.timestamp + "." + string(request.body)))
		require.Equal(t, fmt.Sprintf("sha256=%x", mac.Sum(nil)), request.signature)
		require.Contains(t, string(request.body), `"event_type":"payment.succeeded"`)
	case <-time.After(2 * time.Second):
		t.Fatal("public HTTPS endpoint did not receive the webhook")
	}

	records := countWebhooks(t, db)
	require.Len(t, records, 1)
	require.Equal(t, payment.WebhookStatusDelivered, records[0].Status)
}

func TestWebhookFollowsPublicHTTPSRedirectAfterRevalidation(t *testing.T) {
	cert, roots := newWebhookTestCert(t, "webhook.example")

	var finalHits atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "https://webhook.example/final", http.StatusFound)
			return
		}
		finalHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	server.StartTLS()
	defer server.Close()

	db := newWebhookTestDB(t)
	merchant := seedWebhookMerchant(t, db, "https://webhook.example/start")

	var dials atomic.Int32
	sender := newDestinationTestSender(t, db,
		publicLookup("93.184.216.34"),
		dialTo(server.Listener.Addr().String(), &dials),
		roots,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, sender.SendSync(ctx, webhookJob(merchant.ID)))
	require.EqualValues(t, 1, finalHits.Load())
}

func TestWebhookRejectsRedirectToNonPublicDestination(t *testing.T) {
	cert, roots := newWebhookTestCert(t, "webhook.example")

	var requests atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Redirect(w, r, "https://127.0.0.1/internal", http.StatusFound)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	server.StartTLS()
	defer server.Close()

	db := newWebhookTestDB(t)
	merchant := seedWebhookMerchant(t, db, "https://webhook.example/start")

	var dials atomic.Int32
	sender := newDestinationTestSender(t, db,
		publicLookup("93.184.216.34"),
		dialTo(server.Listener.Addr().String(), &dials),
		roots,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sender.SendSync(ctx, webhookJob(merchant.ID))
	require.Error(t, err)
	require.EqualValues(t, 1, requests.Load(), "the redirected request must not be sent")
	require.EqualValues(t, 1, dials.Load(), "the redirected target must not be dialed")

	records := countWebhooks(t, db)
	require.Len(t, records, 1)
	require.Equal(t, payment.WebhookStatusFailed, records[0].Status)
}

func TestIsPublicIP(t *testing.T) {
	public := []string{
		"93.184.216.34",
		"8.8.8.8",
		"2606:4700:4700::1111",
	}
	nonPublic := []string{
		"127.0.0.1",
		"10.0.0.1",
		"172.16.0.1",
		"192.168.0.1",
		"169.254.169.254",
		"0.0.0.0",
		"100.64.0.1",
		"192.0.2.1",
		"198.18.0.1",
		"198.51.100.1",
		"203.0.113.1",
		"240.0.0.1",
		"::1",
		"fc00::1",
		"fe80::1",
		"2001:db8::1",
		"ff02::1",
		"::ffff:127.0.0.1",
	}

	for _, value := range public {
		require.True(t, isPublicIP(net.ParseIP(value)), "%s must be public", value)
	}
	for _, value := range nonPublic {
		require.False(t, isPublicIP(net.ParseIP(value)), "%s must not be public", value)
	}
}
