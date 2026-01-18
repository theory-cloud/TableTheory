package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetTestConfigDefaults(t *testing.T) {
	t.Setenv("DYNAMODB_ENDPOINT", "")
	t.Setenv("AWS_REGION", "")
	t.Setenv("SKIP_INTEGRATION", "")

	cfg := GetTestConfig()
	assert.Equal(t, "http://localhost:8000", cfg.Endpoint)
	assert.Equal(t, "us-east-1", cfg.Region)
	assert.False(t, cfg.SkipIntegration)
}

func TestGetTestConfigOverrides(t *testing.T) {
	t.Setenv("DYNAMODB_ENDPOINT", "http://override.local:9000")
	t.Setenv("AWS_REGION", "eu-west-1")
	t.Setenv("SKIP_INTEGRATION", "true")

	cfg := GetTestConfig()
	assert.Equal(t, "http://override.local:9000", cfg.Endpoint)
	assert.Equal(t, "eu-west-1", cfg.Region)
	assert.True(t, cfg.SkipIntegration)
}

func withStaticAWSEnv(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	t.Setenv("AWS_SESSION_TOKEN", "token")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}

func TestIsDynamoDBLocalRunning(t *testing.T) {
	withStaticAWSEnv(t)

	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		resp := map[string]any{
			"TableNames": []string{},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer successServer.Close()

	assert.True(t, isDynamoDBLocalRunning(successServer.URL))

	failureServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "failure", http.StatusInternalServerError)
	}))
	defer failureServer.Close()

	assert.False(t, isDynamoDBLocalRunning(failureServer.URL))
}

func TestRequireDynamoDBLocalSkipsInShortMode(t *testing.T) {
	if !testing.Short() {
		t.Skip("requires -short mode to assert early skip")
	}

	RequireDynamoDBLocal(t)
	t.Fatal("require should have skipped in short mode")
}

func TestRequireDynamoDBLocalSkipsWhenIntegrationDisabled(t *testing.T) {
	if testing.Short() {
		t.Skip("requires full test run")
	}

	t.Setenv("SKIP_INTEGRATION", "true")
	RequireDynamoDBLocal(t)
	t.Fatal("require should have skipped when SKIP_INTEGRATION=true")
}

func TestIsDynamoDBLocalRunningTimeout(t *testing.T) {
	withStaticAWSEnv(t)

	blockingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		w.WriteHeader(http.StatusGatewayTimeout)
	}))
	defer blockingServer.Close()

	assert.False(t, isDynamoDBLocalRunning(blockingServer.URL))
}
