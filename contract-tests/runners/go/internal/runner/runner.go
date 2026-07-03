package runner

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory-contract-tests/runners/go/internal/driver"
	"github.com/theory-cloud/tabletheory-contract-tests/runners/go/internal/scenario"
	"github.com/theory-cloud/tabletheory-contract-tests/runners/go/internal/spec"
	theorydbErrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

type Runner struct {
	ddb    *dynamodb.Client
	driver driver.Driver
	vars   map[string]any
}

func New(driver driver.Driver) (*Runner, error) {
	endpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if region == "" {
		region = "us-east-1"
	}

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
	)
	if err != nil {
		return nil, err
	}

	ddb := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	return &Runner{
		ddb:    ddb,
		driver: driver,
		vars:   make(map[string]any),
	}, nil
}

func (r *Runner) Ping(ctx context.Context) error {
	_, err := r.ddb.ListTables(ctx, &dynamodb.ListTablesInput{Limit: aws.Int32(1)})
	return err
}

func (r *Runner) RunScenario(t require.TestingT, ctx context.Context, s *scenario.Scenario, models map[string]spec.Model) {
	model, ok := models[s.Model]
	require.True(t, ok, "unknown model %s", s.Model)

	tableName := s.Table.Name
	if tableName == "" {
		tableName = model.Table.Name
	}

	require.NotEmpty(t, tableName, "table name required")

	steps, recreate := stepsForRuntime(s, "go")
	if recreate {
		require.NoError(t, r.recreateTable(ctx, tableName, model))
	}

	for i := range steps {
		step := steps[i]
		r.runStep(t, ctx, s, models, tableName, step)
	}
}

func stepsForRuntime(s *scenario.Scenario, runtimeName string) ([]scenario.Step, bool) {
	if s.SeedRuntime == "" {
		return s.Steps, true
	}
	if strings.EqualFold(s.SeedRuntime, runtimeName) {
		steps := make([]scenario.Step, 0, len(s.SeedSteps)+len(s.ReadSteps))
		steps = append(steps, s.SeedSteps...)
		steps = append(steps, s.ReadSteps...)
		return steps, true
	}
	return s.ReadSteps, false
}

func (r *Runner) runStep(t require.TestingT, ctx context.Context, s *scenario.Scenario, models map[string]spec.Model, tableName string, step scenario.Step) {
	modelName := stepModelName(s, step)
	model, ok := models[modelName]
	require.True(t, ok, "unknown model %s", modelName)

	switch step.Op {
	case "sleep":
		if step.Ms > 0 {
			time.Sleep(time.Duration(step.Ms) * time.Millisecond)
		}
		return

	case "create":
		err := r.driver.Create(ctx, modelName, step.Item, step.IfNotExists)
		r.assertStepResult(t, step.Expect, nil, err, nil, model)
		return

	case "update":
		err := r.driver.Update(ctx, modelName, step.Item, step.Fields, step.ProtectedAttributes)
		r.assertStepResult(t, step.Expect, nil, err, nil, model)
		return

	case "save":
		err := r.driver.Save(ctx, modelName, step.Item)
		r.assertStepResult(t, step.Expect, nil, err, nil, model)
		return

	case "delete":
		err := r.driver.Delete(ctx, modelName, step.Key)
		r.assertStepResult(t, step.Expect, nil, err, nil, model)
		return

	case "get":
		item, err := r.driver.Get(ctx, modelName, step.Key)
		raw, rawErr := r.getRawItem(ctx, tableName, model, step.Key)
		if err == nil && rawErr != nil {
			err = rawErr
		}

		if err == nil && len(step.Save) > 0 {
			for varName, attrName := range step.Save {
				r.vars[varName] = item[attrName]
			}
		}

		r.assertStepResult(t, step.Expect, item, err, raw, model)
		return

	case "query":
		require.NotNil(t, step.Query, "query request is required")
		result, err := r.driver.Query(ctx, modelName, readRequestFromScenario(step.Query))
		r.assertReadResult(t, step.Expect, result, err, model)
		return

	case "scan":
		require.NotNil(t, step.Scan, "scan request is required")
		result, err := r.driver.Scan(ctx, modelName, readRequestFromScenario(step.Scan))
		r.assertReadResult(t, step.Expect, result, err, model)
		return

	case "transition_append_event":
		require.NotNil(t, step.Actual, "transition_append_event actual is required")
		require.NotNil(t, step.Event, "transition_append_event event is required")
		err := r.driver.TransitionAppendEvent(ctx, driver.TransitionActual{
			Model:           step.Actual.Model,
			Key:             step.Actual.Key,
			Set:             step.Actual.Set,
			ExpectedVersion: step.Actual.ExpectedVersion,
		}, driver.TransitionEvent{
			Model: step.Event.Model,
			Item:  step.Event.Item,
		})
		r.assertStepResult(t, step.Expect, nil, err, nil, model)
		return

	case "validate_provenance":
		err := r.driver.ValidateProvenance(ctx, modelName, step.Item)
		r.assertStepResult(t, step.Expect, nil, err, nil, model)
		return

	default:
		require.FailNow(t, fmt.Sprintf("unsupported op: %s", step.Op))
	}
}

func readRequestFromScenario(req *scenario.ReadRequest) driver.ReadRequest {
	if req == nil {
		return driver.ReadRequest{}
	}
	out := driver.ReadRequest{
		Index:          req.Index,
		SortDirection:  req.SortDirection,
		Limit:          req.Limit,
		Projection:     append([]string(nil), req.Projection...),
		Cursor:         req.Cursor,
		ConsistentRead: req.ConsistentRead,
	}
	if req.Partition != nil {
		partition := readConditionFromScenario(*req.Partition)
		out.Partition = &partition
	}
	if req.Sort != nil {
		sortCond := readConditionFromScenario(*req.Sort)
		out.Sort = &sortCond
	}
	for _, filter := range req.Filter {
		out.Filter = append(out.Filter, readConditionFromScenario(filter))
	}
	return out
}

func readConditionFromScenario(cond scenario.ReadCondition) driver.ReadCondition {
	return driver.ReadCondition{
		Attribute: cond.Attribute,
		Operator:  cond.Operator,
		Value:     cond.Value,
		Values:    append([]any(nil), cond.Values...),
	}
}

func stepModelName(s *scenario.Scenario, step scenario.Step) string {
	if step.Model != "" {
		return step.Model
	}
	return s.Model
}

func (r *Runner) assertStepResult(t require.TestingT, expect scenario.Expectation, item map[string]any, err error, raw map[string]types.AttributeValue, model spec.Model) {
	hasItemAssertion := expectationHasItemAssertion(expect)
	hasRawAssertion := expectationHasRawItemAssertion(expect)

	if expect.Error != "" {
		require.Error(t, err)
		require.Equal(t, driver.ErrorCode(expect.Error), driver.MapError(err))
		require.False(t, hasItemAssertion, "item assertions cannot be combined with error expectations")
		return
	}
	if hasItemAssertion {
		require.NoError(t, err, "expected successful operation for item assertions")
		require.NotNil(t, item, "expected item for item assertions")
	}
	if hasRawAssertion {
		require.NotNil(t, raw, "expected raw item for raw assertions")
	}
	if expect.Ok != nil {
		if *expect.Ok {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	if err != nil {
		return
	}

	if len(expect.ItemContains) > 0 {
		for attr, want := range expect.ItemContains {
			have, ok := item[attr]
			require.True(t, ok, "missing attr %s in item", attr)
			attrDef := model.AttributeByName(attr)
			require.NotNil(t, attrDef, "unknown attr %s in model %s", attr, model.Name)
			assertValueMatches(t, *attrDef, want, have, raw[attr])
		}
	}

	if len(expect.ItemEquals) > 0 {
		assertItemEquals(t, expect.ItemEquals, item, raw, model)
	}

	if len(expect.ItemHasFields) > 0 {
		for _, attr := range expect.ItemHasFields {
			_, ok := item[attr]
			require.True(t, ok, "expected field %s", attr)
		}
	}

	if len(expect.ItemMissingFields) > 0 {
		for _, attr := range expect.ItemMissingFields {
			_, ok := raw[attr]
			require.False(t, ok, "expected missing raw field %s", attr)
		}
	}

	if len(expect.RawAttributeTypes) > 0 {
		for attr, wantType := range expect.RawAttributeTypes {
			av, ok := raw[attr]
			require.True(t, ok, "expected raw field %s", attr)
			require.Equal(t, wantType, attributeValueTypeName(av))
		}
	}

	if len(expect.ItemFieldEqualsVar) > 0 {
		for attr, varName := range expect.ItemFieldEqualsVar {
			require.Equal(t, r.vars[varName], item[attr], "field %s should equal var %s", attr, varName)
		}
	}
	if len(expect.ItemFieldNotEqualsVar) > 0 {
		for attr, varName := range expect.ItemFieldNotEqualsVar {
			require.NotEqual(t, r.vars[varName], item[attr], "field %s should differ from var %s", attr, varName)
		}
	}
}

func (r *Runner) assertReadResult(t require.TestingT, expect scenario.Expectation, result driver.ReadResult, err error, model spec.Model) {
	hasReadAssertion := expectationHasReadAssertion(expect)

	if expect.Error != "" {
		require.Error(t, err)
		require.Equal(t, driver.ErrorCode(expect.Error), driver.MapError(err))
		require.False(t, hasReadAssertion, "read assertions cannot be combined with error expectations")
		return
	}
	if hasReadAssertion {
		require.NoError(t, err, "expected successful read for read assertions")
	}
	if expect.Ok != nil {
		if *expect.Ok {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	if err != nil {
		return
	}

	if expect.ItemCount != nil {
		require.Len(t, result.Items, *expect.ItemCount)
	}

	if len(expect.ItemsContains) > 0 {
		require.GreaterOrEqual(t, len(result.Items), len(expect.ItemsContains), "not enough result items")
		for i, wantItem := range expect.ItemsContains {
			haveItem := result.Items[i]
			for attr, want := range wantItem {
				have, ok := haveItem[attr]
				require.True(t, ok, "missing attr %s in result item %d", attr, i)
				attrDef := model.AttributeByName(attr)
				require.NotNil(t, attrDef, "unknown attr %s in model %s", attr, model.Name)
				assertValueMatches(t, *attrDef, want, have, nil)
			}
		}
	}
	if expect.CursorEquals != nil {
		require.Equal(t, *expect.CursorEquals, result.Cursor)
	}
}

func expectationHasItemAssertion(expect scenario.Expectation) bool {
	return len(expect.ItemContains) > 0 ||
		len(expect.ItemEquals) > 0 ||
		len(expect.ItemHasFields) > 0 ||
		len(expect.ItemMissingFields) > 0 ||
		len(expect.RawAttributeTypes) > 0 ||
		len(expect.ItemFieldEqualsVar) > 0 ||
		len(expect.ItemFieldNotEqualsVar) > 0
}

func expectationHasRawItemAssertion(expect scenario.Expectation) bool {
	return len(expect.ItemMissingFields) > 0 || len(expect.RawAttributeTypes) > 0
}

func expectationHasReadAssertion(expect scenario.Expectation) bool {
	return expect.ItemCount != nil ||
		len(expect.ItemsContains) > 0 ||
		expect.CursorEquals != nil
}

func assertItemEquals(t require.TestingT, want map[string]any, item map[string]any, raw map[string]types.AttributeValue, model spec.Model) {
	if raw != nil {
		require.Len(t, raw, len(want), "raw item should contain exactly the expected attributes")
		for attr, wantValue := range want {
			rawValue, ok := raw[attr]
			require.True(t, ok, "missing raw attr %s", attr)
			attrDef := model.AttributeByName(attr)
			require.NotNil(t, attrDef, "unknown attr %s in model %s", attr, model.Name)
			assertValueMatches(t, *attrDef, wantValue, item[attr], rawValue)
		}
		return
	}

	require.Len(t, item, len(want), "item should contain exactly the expected attributes")
	for attr, wantValue := range want {
		have, ok := item[attr]
		require.True(t, ok, "missing attr %s", attr)
		attrDef := model.AttributeByName(attr)
		require.NotNil(t, attrDef, "unknown attr %s in model %s", attr, model.Name)
		assertValueMatches(t, *attrDef, wantValue, have, nil)
	}
}

func assertValueMatches(t require.TestingT, attr spec.Attribute, want any, have any, raw types.AttributeValue) {
	switch attr.Type {
	case "S":
		if raw != nil {
			rawS, ok := raw.(*types.AttributeValueMemberS)
			require.True(t, ok, "expected raw S for %s, got %T", attr.Attribute, raw)
			require.Equal(t, fmt.Sprintf("%v", want), rawS.Value)
			return
		}
		require.Equal(t, fmt.Sprintf("%v", want), fmt.Sprintf("%v", have))
	case "N":
		wantN, err := canonicalDecimalString(want)
		require.NoError(t, err)
		if raw != nil {
			rawN, ok := raw.(*types.AttributeValueMemberN)
			require.True(t, ok, "expected raw N for %s, got %T", attr.Attribute, raw)
			require.Equal(t, wantN, rawN.Value)
			return
		}
		haveN, err := canonicalDecimalString(have)
		require.NoError(t, err)
		require.Equal(t, wantN, haveN)
	case "SS":
		wantSS, err := asStringSet(want)
		require.NoError(t, err)
		if raw != nil {
			rawSS, ok := raw.(*types.AttributeValueMemberSS)
			require.True(t, ok, "expected raw SS for %s, got %T", attr.Attribute, raw)
			haveSS := append([]string(nil), rawSS.Value...)
			sort.Strings(haveSS)
			require.Equal(t, wantSS, haveSS)
			return
		}
		haveSS, err := asStringSet(have)
		require.NoError(t, err)
		require.Equal(t, wantSS, haveSS)
	default:
		require.Equal(t, want, have, "unhandled type %s", attr.Type)
	}
}

func canonicalDecimalString(v any) (string, error) {
	if v == nil {
		return "0", nil
	}
	switch n := v.(type) {
	case int:
		return strconv.FormatInt(int64(n), 10), nil
	case int8:
		return strconv.FormatInt(int64(n), 10), nil
	case int16:
		return strconv.FormatInt(int64(n), 10), nil
	case int32:
		return strconv.FormatInt(int64(n), 10), nil
	case int64:
		return strconv.FormatInt(n, 10), nil
	case uint:
		return strconv.FormatUint(uint64(n), 10), nil
	case uint8:
		return strconv.FormatUint(uint64(n), 10), nil
	case uint16:
		return strconv.FormatUint(uint64(n), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(n), 10), nil
	case uint64:
		return strconv.FormatUint(n, 10), nil
	case float64:
		if math.IsInf(n, 0) || math.IsNaN(n) {
			return "", fmt.Errorf("non-finite number %v", n)
		}
		return strconv.FormatFloat(n, 'f', -1, 64), nil
	case float32:
		f := float64(n)
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return "", fmt.Errorf("non-finite number %v", n)
		}
		return strconv.FormatFloat(f, 'f', -1, 32), nil
	case string:
		return n, nil
	default:
		return "", fmt.Errorf("expected DynamoDB decimal string, got %T", v)
	}
}

func asStringSet(v any) ([]string, error) {
	var items []string
	switch s := v.(type) {
	case []string:
		items = append([]string(nil), s...)
	case []any:
		for _, item := range s {
			items = append(items, fmt.Sprintf("%v", item))
		}
	default:
		return nil, fmt.Errorf("expected slice, got %T", v)
	}
	sort.Strings(items)
	return items, nil
}

func attributeValueTypeName(av types.AttributeValue) string {
	switch av.(type) {
	case *types.AttributeValueMemberS:
		return "S"
	case *types.AttributeValueMemberN:
		return "N"
	case *types.AttributeValueMemberB:
		return "B"
	case *types.AttributeValueMemberBOOL:
		return "BOOL"
	case *types.AttributeValueMemberNULL:
		return "NULL"
	case *types.AttributeValueMemberL:
		return "L"
	case *types.AttributeValueMemberM:
		return "M"
	case *types.AttributeValueMemberSS:
		return "SS"
	case *types.AttributeValueMemberNS:
		return "NS"
	case *types.AttributeValueMemberBS:
		return "BS"
	default:
		return fmt.Sprintf("%T", av)
	}
}

func (r *Runner) getRawItem(ctx context.Context, tableName string, model spec.Model, key map[string]any) (map[string]types.AttributeValue, error) {
	if key == nil {
		return nil, fmt.Errorf("%w: key is required", theorydbErrors.ErrMissingPrimaryKey)
	}
	pk := fmt.Sprintf("%v", key[model.Keys.Partition.Attribute])
	sk := ""
	if model.Keys.Sort != nil {
		sk = fmt.Sprintf("%v", key[model.Keys.Sort.Attribute])
	}

	keyAV := map[string]types.AttributeValue{
		model.Keys.Partition.Attribute: &types.AttributeValueMemberS{Value: pk},
	}
	if model.Keys.Sort != nil {
		keyAV[model.Keys.Sort.Attribute] = &types.AttributeValueMemberS{Value: sk}
	}

	out, err := r.ddb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(tableName),
		Key:            keyAV,
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, err
	}
	if len(out.Item) == 0 {
		return nil, theorydbErrors.ErrItemNotFound
	}
	return out.Item, nil
}

func (r *Runner) recreateTable(ctx context.Context, tableName string, model spec.Model) error {
	_, err := r.ddb.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(tableName)})
	if err != nil && !isResourceNotFound(err) {
		return err
	}

	waitNotExists := dynamodb.NewTableNotExistsWaiter(r.ddb)
	_ = waitNotExists.Wait(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(tableName)}, 10*time.Second)

	input, err := createTableInput(tableName, model)
	if err != nil {
		return err
	}
	if _, err := r.ddb.CreateTable(ctx, input); err != nil {
		return err
	}

	waitExists := dynamodb.NewTableExistsWaiter(r.ddb)
	return waitExists.Wait(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(tableName)}, 30*time.Second)
}

func isResourceNotFound(err error) bool {
	var rnfe *types.ResourceNotFoundException
	return errors.As(err, &rnfe)
}

func createTableInput(tableName string, model spec.Model) (*dynamodb.CreateTableInput, error) {
	defs := make(map[string]types.ScalarAttributeType)

	addDef := func(attr spec.KeyAttribute) error {
		switch attr.Type {
		case "S":
			defs[attr.Attribute] = types.ScalarAttributeTypeS
		case "N":
			defs[attr.Attribute] = types.ScalarAttributeTypeN
		case "B":
			defs[attr.Attribute] = types.ScalarAttributeTypeB
		default:
			return fmt.Errorf("unsupported key type %q for %s", attr.Type, attr.Attribute)
		}
		return nil
	}

	if err := addDef(model.Keys.Partition); err != nil {
		return nil, err
	}
	if model.Keys.Sort != nil {
		if err := addDef(*model.Keys.Sort); err != nil {
			return nil, err
		}
	}
	for _, idx := range model.Indexes {
		if err := addDef(idx.Partition); err != nil {
			return nil, err
		}
		if idx.Sort != nil {
			if err := addDef(*idx.Sort); err != nil {
				return nil, err
			}
		}
	}

	attrs := make([]types.AttributeDefinition, 0, len(defs))
	for name, typ := range defs {
		n := name
		attrs = append(attrs, types.AttributeDefinition{
			AttributeName: &n,
			AttributeType: typ,
		})
	}
	sort.Slice(attrs, func(i, j int) bool {
		return *attrs[i].AttributeName < *attrs[j].AttributeName
	})

	keySchema := []types.KeySchemaElement{
		{
			AttributeName: aws.String(model.Keys.Partition.Attribute),
			KeyType:       types.KeyTypeHash,
		},
	}
	if model.Keys.Sort != nil {
		keySchema = append(keySchema, types.KeySchemaElement{
			AttributeName: aws.String(model.Keys.Sort.Attribute),
			KeyType:       types.KeyTypeRange,
		})
	}

	var gsis []types.GlobalSecondaryIndex
	var lsis []types.LocalSecondaryIndex

	for _, idx := range model.Indexes {
		projection := types.Projection{ProjectionType: types.ProjectionTypeAll}
		switch idx.Projection.Type {
		case "", "ALL":
			projection.ProjectionType = types.ProjectionTypeAll
		case "KEYS_ONLY":
			projection.ProjectionType = types.ProjectionTypeKeysOnly
		case "INCLUDE":
			projection.ProjectionType = types.ProjectionTypeInclude
			projection.NonKeyAttributes = append([]string(nil), idx.Projection.Fields...)
		default:
			return nil, fmt.Errorf("unsupported projection type %q", idx.Projection.Type)
		}

		indexKeySchema := []types.KeySchemaElement{
			{
				AttributeName: aws.String(idx.Partition.Attribute),
				KeyType:       types.KeyTypeHash,
			},
		}
		if idx.Sort != nil {
			indexKeySchema = append(indexKeySchema, types.KeySchemaElement{
				AttributeName: aws.String(idx.Sort.Attribute),
				KeyType:       types.KeyTypeRange,
			})
		}

		if idx.Type == "LSI" {
			lsis = append(lsis, types.LocalSecondaryIndex{
				IndexName:  aws.String(idx.Name),
				KeySchema:  indexKeySchema,
				Projection: &projection,
			})
			continue
		}

		gsis = append(gsis, types.GlobalSecondaryIndex{
			IndexName:  aws.String(idx.Name),
			KeySchema:  indexKeySchema,
			Projection: &projection,
		})
	}

	return &dynamodb.CreateTableInput{
		TableName:              aws.String(tableName),
		AttributeDefinitions:   attrs,
		KeySchema:              keySchema,
		BillingMode:            types.BillingModePayPerRequest,
		GlobalSecondaryIndexes: gsis,
		LocalSecondaryIndexes:  lsis,
	}, nil
}

func RepoRootFromModuleDir() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("failed to locate runner source file")
	}

	// file = contract-tests/runners/go/internal/runner/runner.go
	// repo root is 5 levels up.
	dir := filepath.Dir(file)
	return filepath.Clean(filepath.Join(dir, "..", "..", "..", "..", "..")), nil
}
