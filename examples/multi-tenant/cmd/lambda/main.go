// Command lambda is the multi-tenant example's Lambda entrypoint. A single
// binary serves every function the SAM template declares; the FUNCTION_TYPE
// environment variable selects which one an invocation is.
//
// Nothing here starts work that could outlive an invocation. Every handler runs
// on the invocation's own goroutine and returns its result before it returns —
// there are no timers, no background loops, and no goroutines, because Lambda
// freezes the execution environment the moment the handler returns.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/examples/multi-tenant/handlers"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
)

// db opens the shared TableTheory handle once per execution environment and
// reuses it across warm invocations. Constructing the handle starts no
// background work, so opening it cannot leave anything running after a handler
// returns.
var db = sync.OnceValues(func() (core.ExtendedDB, error) {
	config := session.Config{Region: region()}
	if endpoint := strings.TrimSpace(os.Getenv("DYNAMODB_ENDPOINT")); endpoint != "" {
		config.Endpoint = endpoint
	}
	return tabletheory.New(config)
})

// region reads the deployed region, falling back to the standard AWS variable.
func region() string {
	if value := strings.TrimSpace(os.Getenv("REGION")); value != "" {
		return value
	}
	return os.Getenv("AWS_REGION")
}

func main() {
	lambda.Start(dispatch)
}

// dispatch routes an invocation to the handler family named by FUNCTION_TYPE.
// An unknown type is an error rather than a silent success, so a
// misconfiguration fails loudly instead of returning an empty response.
func dispatch(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	switch functionType := strings.TrimSpace(os.Getenv("FUNCTION_TYPE")); functionType {
	case "organization", "user", "project", "resource", "apikey":
		var event events.APIGatewayProxyRequest
		if err := json.Unmarshal(raw, &event); err != nil {
			return nil, fmt.Errorf("decode API Gateway request: %w", err)
		}
		response, err := serveAPI(ctx, event)
		if err != nil {
			return nil, err
		}
		return json.Marshal(response)

	case "jwt_authorizer":
		var event events.APIGatewayCustomAuthorizerRequestTypeRequest
		if err := json.Unmarshal(raw, &event); err != nil {
			return nil, fmt.Errorf("decode authorizer request: %w", err)
		}
		response, err := authorizeJWT(event)
		if err != nil {
			return nil, err
		}
		return json.Marshal(response)

	case "apikey_authorizer":
		var event events.APIGatewayCustomAuthorizerRequestTypeRequest
		if err := json.Unmarshal(raw, &event); err != nil {
			return nil, fmt.Errorf("decode authorizer request: %w", err)
		}
		response, err := authorizeAPIKey(event)
		if err != nil {
			return nil, err
		}
		return json.Marshal(response)

	default:
		return nil, fmt.Errorf("FUNCTION_TYPE %q is not implemented by this example", functionType)
	}
}

// serveAPI adapts an API Gateway proxy invocation to the example's shared HTTP
// route table and adapts the response back. The whole exchange happens on this
// invocation's goroutine and finishes before the function returns.
func serveAPI(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	database, err := db()
	if err != nil {
		return events.APIGatewayProxyResponse{}, fmt.Errorf("initialize TableTheory: %w", err)
	}

	request, err := httpRequest(ctx, event)
	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Body:       err.Error(),
		}, nil
	}

	recorder := &recorder{}
	handlers.NewRouter(database).ServeHTTP(recorder, request)

	return events.APIGatewayProxyResponse{
		StatusCode: recorder.statusCode(),
		Headers:    recorder.firstHeaders(),
		Body:       recorder.body.String(),
	}, nil
}

// httpRequest turns an API Gateway proxy event into the *http.Request the shared
// router expects. The path is set on the URL directly rather than through
// http.NewRequest: the example's tenant IDs contain a literal "#" (for example
// org#demo), which URL parsing would otherwise treat as a fragment, and the
// router matches on the decoded path.
func httpRequest(ctx context.Context, event events.APIGatewayProxyRequest) (*http.Request, error) {
	path := event.Path
	if path == "" {
		path = "/"
	}
	if decoded, err := url.PathUnescape(path); err == nil {
		path = decoded
	}

	query := url.Values{}
	for key, value := range event.QueryStringParameters {
		query.Set(key, value)
	}

	request := &http.Request{
		Method:        event.HTTPMethod,
		URL:           &url.URL{Path: path, RawQuery: query.Encode()},
		Header:        make(http.Header, len(event.Headers)),
		Body:          io.NopCloser(strings.NewReader(event.Body)),
		ContentLength: int64(len(event.Body)),
		Host:          "lambda",
	}
	for key, value := range event.Headers {
		request.Header.Set(key, value)
	}
	return request.WithContext(ctx), nil
}

// recorder is a minimal http.ResponseWriter that captures what a handler wrote.
// It is an ordinary value on the invocation's stack — not a shared or
// backgrounded buffer — so nothing survives the call.
type recorder struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (r *recorder) Header() http.Header {
	if r.header == nil {
		r.header = http.Header{}
	}
	return r.header
}

func (r *recorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(data)
}

func (r *recorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *recorder) statusCode() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

// firstHeaders flattens the recorded headers to the single value per key an API
// Gateway proxy response carries.
func (r *recorder) firstHeaders() map[string]string {
	if len(r.header) == 0 {
		return nil
	}
	headers := make(map[string]string, len(r.header))
	for key, values := range r.header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	return headers
}

// allowPolicy builds the API Gateway authorization response for an allowed
// principal: an IAM policy permitting execute-api:Invoke on exactly the method
// being authorized, plus the tenant context the handlers read.
func allowPolicy(principalID, methodARN string, context map[string]any) events.APIGatewayCustomAuthorizerResponse {
	return events.APIGatewayCustomAuthorizerResponse{
		PrincipalID: principalID,
		PolicyDocument: events.APIGatewayCustomAuthorizerPolicy{
			Version: "2012-10-17",
			Statement: []events.IAMPolicyStatement{
				{
					Action:   []string{"execute-api:Invoke"},
					Effect:   "Allow",
					Resource: []string{methodARN},
				},
			},
		},
		Context: context,
	}
}

// authorizeJWT validates the example's simplified "user_id:org_id" bearer token
// and returns the tenant context the handlers read. The example's token
// handling is deliberately illustrative and matches the local server's
// middleware: a real deployment would verify a signed JWT here.
func authorizeJWT(event events.APIGatewayCustomAuthorizerRequestTypeRequest) (events.APIGatewayCustomAuthorizerResponse, error) {
	token := strings.TrimSpace(strings.TrimPrefix(header(event.Headers, "Authorization"), "Bearer "))
	if token == "" {
		return events.APIGatewayCustomAuthorizerResponse{}, errors.New("Unauthorized")
	}

	userID, orgID, found := strings.Cut(token, ":")
	if !found {
		userID, orgID = token, "demo"
	}
	userID = strings.TrimSpace(userID)
	orgID = strings.TrimSpace(orgID)
	if userID == "" || orgID == "" {
		return events.APIGatewayCustomAuthorizerResponse{}, errors.New("Unauthorized")
	}

	return allowPolicy("user#"+userID, event.MethodArn, map[string]any{
		"user_id": "user#" + userID,
		"org_id":  "org#" + orgID,
	}), nil
}

// authorizeAPIKey validates an X-API-Key with the same handler the local server
// uses and enforces its per-key rate limit before allowing the invocation.
func authorizeAPIKey(event events.APIGatewayCustomAuthorizerRequestTypeRequest) (events.APIGatewayCustomAuthorizerResponse, error) {
	apiKey := header(event.Headers, "X-API-Key")
	if apiKey == "" {
		return events.APIGatewayCustomAuthorizerResponse{}, errors.New("Unauthorized")
	}

	database, err := db()
	if err != nil {
		return events.APIGatewayCustomAuthorizerResponse{}, fmt.Errorf("initialize TableTheory: %w", err)
	}

	keyHandler := handlers.NewAPIKeyHandler(database)
	key, err := keyHandler.ValidateAPIKey(apiKey)
	if err != nil {
		return events.APIGatewayCustomAuthorizerResponse{}, errors.New("Unauthorized")
	}
	if err := keyHandler.CheckRateLimit(key.KeyID); err != nil {
		log.Printf("API key %s rejected by rate limit: %v", key.KeyID, err)
		return events.APIGatewayCustomAuthorizerResponse{}, errors.New("Unauthorized")
	}

	return allowPolicy(key.KeyID, event.MethodArn, map[string]any{
		"org_id":     key.OrgID,
		"api_key_id": key.KeyID,
	}), nil
}

// header reads a header case-insensitively, since API Gateway preserves the
// sender's casing.
func header(headers map[string]string, name string) string {
	if value := headers[name]; value != "" {
		return value
	}
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}
