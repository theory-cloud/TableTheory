package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/theory-cloud/tabletheory"
	demomodel "github.com/theory-cloud/tabletheory/examples/cdk-multilang/lambdas/go/demo"
	"github.com/theory-cloud/tabletheory/pkg/dms"
	customerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/model"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

type request struct {
	PK       string `json:"pk"`
	SK       string `json:"sk"`
	Value    string `json:"value"`
	Secret   string `json:"secret"`
	Count    int    `json:"count"`
	SKPrefix string `json:"skPrefix"`
}

func jsonResponse(status int, body any) (events.LambdaFunctionURLResponse, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return events.LambdaFunctionURLResponse{StatusCode: http.StatusInternalServerError}, nil
	}
	return events.LambdaFunctionURLResponse{
		StatusCode: status,
		Headers:    map[string]string{"content-type": "application/json"},
		Body:       string(data),
	}, nil
}

func readBody(event events.LambdaFunctionURLRequest) []byte {
	if event.Body == "" {
		return nil
	}
	if event.IsBase64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(event.Body)
		if err == nil {
			return decoded
		}
	}
	return []byte(event.Body)
}

func parseRequest(event events.LambdaFunctionURLRequest) request {
	var req request
	body := readBody(event)
	if len(body) > 0 {
		_ = json.Unmarshal(body, &req)
	}
	qs := event.QueryStringParameters
	if req.PK == "" {
		req.PK = qs["pk"]
	}
	if req.SK == "" {
		req.SK = qs["sk"]
	}
	if req.Value == "" {
		req.Value = qs["value"]
	}
	if req.Secret == "" {
		req.Secret = qs["secret"]
	}
	if req.SKPrefix == "" {
		req.SKPrefix = qs["skPrefix"]
	}
	return req
}

var dmsValidateOnce sync.Once
var dmsValidateErr error

func validateDMS() error {
	dmsValidateOnce.Do(func() {
		b64 := os.Getenv("DMS_MODEL_B64")
		if b64 == "" {
			dmsValidateErr = fmt.Errorf("DMS_MODEL_B64 is required")
			return
		}

		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			dmsValidateErr = fmt.Errorf("decode DMS_MODEL_B64: %w", err)
			return
		}

		doc, err := dms.ParseDocument(raw)
		if err != nil {
			dmsValidateErr = fmt.Errorf("parse DMS document: %w", err)
			return
		}

		want, ok := dms.FindModel(doc, "DemoItem")
		if !ok {
			dmsValidateErr = fmt.Errorf("DMS document missing model DemoItem")
			return
		}

		reg := model.NewRegistry()
		if err := reg.Register(demomodel.DemoItem{}); err != nil {
			dmsValidateErr = fmt.Errorf("register DemoItem: %w", err)
			return
		}
		meta, err := reg.GetMetadata(demomodel.DemoItem{})
		if err != nil {
			dmsValidateErr = fmt.Errorf("get DemoItem metadata: %w", err)
			return
		}

		got, err := dms.FromMetadata(meta)
		if err != nil {
			dmsValidateErr = fmt.Errorf("convert DemoItem metadata to DMS: %w", err)
			return
		}

		if err := dms.AssertModelsEquivalent(got, *want, dms.CompareOptions{IgnoreTableName: true}); err != nil {
			dmsValidateErr = fmt.Errorf("DMS mismatch: %w", err)
			return
		}
	})

	return dmsValidateErr
}

func handler(ctx context.Context, event events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	if err := validateDMS(); err != nil {
		return jsonResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	db, err := theorydb.New(session.Config{
		Region:    os.Getenv("AWS_REGION"),
		KMSKeyARN: os.Getenv("KMS_KEY_ARN"),
	})
	if err != nil {
		return jsonResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	method := event.RequestContext.HTTP.Method
	path := event.RawPath
	if path == "" {
		path = "/"
	}

	switch path {
	case "/batch":
		if method == http.MethodGet {
			return jsonResponse(http.StatusMethodNotAllowed, map[string]string{"error": "use POST/PUT"})
		}
		req := parseRequest(event)
		if req.PK == "" {
			return jsonResponse(http.StatusBadRequest, map[string]string{"error": "pk is required"})
		}

		count := req.Count
		if count <= 0 {
			count = 3
		}
		if count > 25 {
			return jsonResponse(http.StatusBadRequest, map[string]string{"error": "count must be <= 25"})
		}

		prefix := req.SKPrefix
		if prefix == "" {
			prefix = "BATCH#"
		}

		putItems := make([]any, 0, count)
		keys := make([]any, 0, count)
		for i := 1; i <= count; i++ {
			sk := fmt.Sprintf("%s%d", prefix, i)
			putItems = append(putItems, &demomodel.DemoItem{
				PK:     req.PK,
				SK:     sk,
				Value:  req.Value,
				Lang:   "go",
				Secret: req.Secret,
			})
			keys = append(keys, map[string]any{"PK": req.PK, "SK": sk})
		}

		q := db.Model(&demomodel.DemoItem{})
		if err := q.BatchWrite(putItems, nil); err != nil {
			return jsonResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		var out []demomodel.DemoItem
		if err := q.BatchGet(keys, &out); err != nil {
			return jsonResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		return jsonResponse(http.StatusOK, map[string]any{"ok": true, "count": len(out), "items": out})
	case "/tx":
		if method == http.MethodGet {
			return jsonResponse(http.StatusMethodNotAllowed, map[string]string{"error": "use POST/PUT"})
		}

		req := parseRequest(event)
		if req.PK == "" {
			return jsonResponse(http.StatusBadRequest, map[string]string{"error": "pk is required"})
		}

		prefix := req.SKPrefix
		if prefix == "" {
			prefix = "TX#"
		}

		item1 := &demomodel.DemoItem{PK: req.PK, SK: prefix + "1", Value: req.Value, Lang: "go", Secret: req.Secret}
		item2 := &demomodel.DemoItem{PK: req.PK, SK: prefix + "2", Value: req.Value, Lang: "go", Secret: req.Secret}

		if err := db.Transact().WithContext(ctx).Put(item1).Put(item2).Execute(); err != nil {
			return jsonResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		var out []demomodel.DemoItem
		if err := db.Model(&demomodel.DemoItem{}).BatchGet(
			[]any{
				map[string]any{"PK": req.PK, "SK": item1.SK},
				map[string]any{"PK": req.PK, "SK": item2.SK},
			},
			&out,
		); err != nil {
			return jsonResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		return jsonResponse(http.StatusOK, map[string]any{"ok": true, "count": len(out), "items": out})
	default:
		// fallthrough to basic CRUD
	}

	req := parseRequest(event)

	if method == http.MethodGet {
		if req.PK == "" || req.SK == "" {
			return jsonResponse(http.StatusBadRequest, map[string]string{"error": "pk and sk are required"})
		}

		var out demomodel.DemoItem
		err := db.Model(&demomodel.DemoItem{PK: req.PK, SK: req.SK}).First(&out)
		if err != nil {
			if errors.Is(err, customerrors.ErrItemNotFound) {
				return jsonResponse(http.StatusNotFound, map[string]string{"error": "not found"})
			}
			return jsonResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		return jsonResponse(http.StatusOK, map[string]any{"ok": true, "item": out})
	}

	if req.PK == "" || req.SK == "" {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": "pk and sk are required"})
	}

	item := &demomodel.DemoItem{
		PK:     req.PK,
		SK:     req.SK,
		Value:  req.Value,
		Lang:   "go",
		Secret: req.Secret,
	}

	if err := db.Model(item).CreateOrUpdate(); err != nil {
		return jsonResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return jsonResponse(http.StatusOK, map[string]any{"ok": true, "item": item})
}

func main() {
	lambda.Start(handler)
}
