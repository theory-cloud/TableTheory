package dynamodbprobe_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4/internal/dynamodbprobe"
)

func TestCheckEndpointRejectsNonDynamoDBService(t *testing.T) {
	cases := []struct {
		name   string
		ctype  string
		body   string
		status int
	}{
		{name: "200 empty body", status: http.StatusOK, ctype: "application/x-amz-json-1.0"},
		{name: "200 empty json object", status: http.StatusOK, ctype: "application/x-amz-json-1.0", body: "{}"},
		{name: "200 unrelated json", status: http.StatusOK, ctype: "application/json", body: `{"status":"ok"}`},
		{name: "200 html", status: http.StatusOK, ctype: "text/html; charset=utf-8", body: "<html><body>hello</body></html>"},
		{name: "404 plain text", status: http.StatusNotFound, ctype: "text/plain", body: "not found"},
		{name: "405 json error", status: http.StatusMethodNotAllowed, ctype: "application/x-amz-json-1.0", body: `{"__type":"UnknownOperationException"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.ctype != "" {
					w.Header().Set("Content-Type", tc.ctype)
				}
				w.WriteHeader(tc.status)
				if _, err := w.Write([]byte(tc.body)); err != nil {
					t.Errorf("write probe response: %v", err)
				}
			}))
			defer srv.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := dynamodbprobe.CheckEndpoint(ctx, srv.URL)
			require.Error(t, err)
			require.ErrorIs(t, err, dynamodbprobe.ErrNotDynamoDB)
			require.Contains(t, err.Error(), srv.URL, "error should name the endpoint that was probed")
		})
	}
}

func TestCheckEndpointRejectsUnreachableEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint := srv.URL
	srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := dynamodbprobe.CheckEndpoint(ctx, endpoint)
	require.Error(t, err)
	require.ErrorIs(t, err, dynamodbprobe.ErrNotDynamoDB)
}

func TestCheckEndpointRejectsEmptyEndpoint(t *testing.T) {
	for _, endpoint := range []string{"", "   "} {
		err := dynamodbprobe.CheckEndpoint(context.Background(), endpoint)
		require.Error(t, err)
		require.ErrorIs(t, err, dynamodbprobe.ErrNotDynamoDB)
	}
}

// TestCheckEndpointDynamoDBLocalControl is the positive control: when DynamoDB
// Local is listening on the configured endpoint, CheckEndpoint must accept it.
// It skips (never fails) when nothing is listening so the unit suite stays
// green without Docker.
func TestCheckEndpointDynamoDBLocalControl(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping DynamoDB Local control in -short mode")
	}

	endpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if endpoint == "" {
		endpoint = dynamodbprobe.DefaultEndpoint
	}

	if !endpointListening(t, endpoint) {
		t.Skipf(`DynamoDB Local is not reachable at %s.

To run this control:
1. Install Docker: https://www.docker.com/
2. Start DynamoDB Local: make dynamodb-up`, endpoint)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, dynamodbprobe.CheckEndpoint(ctx, endpoint))
}

// endpointListening reports whether something accepts TCP connections at the
// endpoint, independent of whether it speaks DynamoDB.
func endpointListening(t *testing.T, endpoint string) bool {
	t.Helper()

	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return false
	}

	host := u.Host
	if !strings.Contains(host, ":") {
		if u.Scheme == "https" {
			host += ":443"
		} else {
			host += ":80"
		}
	}

	dialer := &net.Dialer{Timeout: time.Second}
	// #nosec G704 -- the probe dials a locally configured test endpoint, not attacker-controlled input.
	conn, err := dialer.DialContext(context.Background(), "tcp", host)
	if err != nil {
		return false
	}
	if err := conn.Close(); err != nil {
		t.Logf("close probe connection to %s: %v", host, err)
	}
	return true
}
