package scenario

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Scenario struct {
	Name                 string           `yaml:"name"`
	DMSVersion           string           `yaml:"dms_version"`
	RequiresCapabilities []string         `yaml:"requires_capabilities"`
	Model                string           `yaml:"model"`
	Table                Table            `yaml:"table"`
	Encryption           EncryptionConfig `yaml:"encryption"`
	Steps                []Step           `yaml:"steps"`
	SeedRuntime          string           `yaml:"seed_runtime"`
	SeedSteps            []Step           `yaml:"seed_steps"`
	ReadSteps            []Step           `yaml:"read_steps"`
}

type Table struct {
	Name string `yaml:"name"`
}

type EncryptionConfig struct {
	Provider string `yaml:"provider"`
	Seed     string `yaml:"seed"`
}

type Step struct {
	Op                  string                `yaml:"op"`
	Model               string                `yaml:"model"`
	IfNotExists         bool                  `yaml:"if_not_exists"`
	Fields              []string              `yaml:"fields"`
	ProtectedAttributes []string              `yaml:"protected_attributes"`
	Item                map[string]any        `yaml:"item"`
	Key                 map[string]any        `yaml:"key"`
	Query               *ReadRequest          `yaml:"query"`
	Scan                *ReadRequest          `yaml:"scan"`
	Count               *CountRequest         `yaml:"count"`
	TransactGet         *TransactGetRequest   `yaml:"transact_get"`
	BatchGet            *BatchGetRequest      `yaml:"batch_get"`
	BatchWrite          *BatchWriteRequest    `yaml:"batch_write"`
	TransactWrite       *TransactWriteRequest `yaml:"transact_write"`
	Actual              *TransitionActual     `yaml:"actual"`
	Event               *TransitionEvent      `yaml:"event"`
	Ms                  int                   `yaml:"ms"`
	Save                map[string]string     `yaml:"save"`
	Expect              Expectation           `yaml:"expect"`
}

type CountRequest struct {
	Query *ReadRequest `yaml:"query"`
	Scan  *ReadRequest `yaml:"scan"`
}

type TransactGetRequest struct {
	Items []KeyedItem `yaml:"items"`
}

type BatchGetRequest struct {
	Keys []map[string]any `yaml:"keys"`
}

type BatchWriteRequest struct {
	Puts    []map[string]any `yaml:"puts"`
	Deletes []map[string]any `yaml:"deletes"`
}

type TransactWriteRequest struct {
	Actions []TransactWriteAction `yaml:"actions"`
}

type KeyedItem struct {
	Model string         `yaml:"model"`
	Key   map[string]any `yaml:"key"`
}

type TransactWriteAction struct {
	Kind                      string            `yaml:"kind"`
	Model                     string            `yaml:"model"`
	Item                      map[string]any    `yaml:"item"`
	Key                       map[string]any    `yaml:"key"`
	Set                       map[string]any    `yaml:"set"`
	ConditionExpression       string            `yaml:"condition_expression"`
	ExpressionAttributeNames  map[string]string `yaml:"expression_attribute_names"`
	ExpressionAttributeValues map[string]any    `yaml:"expression_attribute_values"`
	IfNotExists               bool              `yaml:"if_not_exists"`
}

type ReadRequest struct {
	Index          string          `yaml:"index"`
	Partition      *ReadCondition  `yaml:"partition"`
	Sort           *ReadCondition  `yaml:"sort"`
	Filter         []ReadCondition `yaml:"filter"`
	SortDirection  string          `yaml:"sort_direction"`
	Limit          int             `yaml:"limit"`
	Projection     []string        `yaml:"projection"`
	Cursor         string          `yaml:"cursor"`
	ConsistentRead *bool           `yaml:"consistent_read"`
}

type ReadCondition struct {
	Attribute string `yaml:"attribute"`
	Operator  string `yaml:"operator"`
	Value     any    `yaml:"value"`
	Values    []any  `yaml:"values"`
}

type TransitionActual struct {
	Model           string         `yaml:"model"`
	Key             map[string]any `yaml:"key"`
	Set             map[string]any `yaml:"set"`
	ExpectedVersion *int64         `yaml:"expected_version"`
}

type TransitionEvent struct {
	Model string         `yaml:"model"`
	Item  map[string]any `yaml:"item"`
}

type Expectation struct {
	Ok                    *bool             `yaml:"ok"`
	Error                 string            `yaml:"error"`
	Errors                []string          `yaml:"errors"`
	ItemContains          map[string]any    `yaml:"item_contains"`
	ItemEquals            map[string]any    `yaml:"item_equals"`
	RawItemContains       map[string]any    `yaml:"raw_item_contains"`
	ItemsContains         []map[string]any  `yaml:"items_contains"`
	ItemsMissingFields    []string          `yaml:"items_missing_fields"`
	ItemCount             *int              `yaml:"item_count"`
	Count                 *int              `yaml:"count"`
	ItemAbsent            *bool             `yaml:"item_absent"`
	CursorEquals          *string           `yaml:"cursor_equals"`
	ItemHasFields         []string          `yaml:"item_has_fields"`
	ItemMissingFields     []string          `yaml:"item_missing_fields"`
	RawAttributeTypes     map[string]string `yaml:"raw_attribute_types"`
	ItemFieldEqualsVar    map[string]string `yaml:"item_field_equals_var"`
	ItemFieldNotEqualsVar map[string]string `yaml:"item_field_not_equals_var"`
}

func LoadFile(path string) (*Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read scenario: %w", err)
	}

	var s Scenario
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse scenario: %w", err)
	}

	if s.DMSVersion != "0.2" {
		return nil, fmt.Errorf("unsupported dms_version: %q", s.DMSVersion)
	}
	if s.Name == "" {
		return nil, fmt.Errorf("scenario name is required")
	}
	if s.Model == "" {
		return nil, fmt.Errorf("scenario model is required")
	}
	if err := validateSteps(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func validateSteps(s *Scenario) error {
	if s.SeedRuntime != "" {
		if len(s.SeedSteps) == 0 {
			return fmt.Errorf("seed_steps are required when seed_runtime is set")
		}
		if len(s.ReadSteps) == 0 {
			return fmt.Errorf("read_steps are required when seed_runtime is set")
		}
	}
	if s.SeedRuntime == "" && len(s.Steps) == 0 {
		return fmt.Errorf("scenario steps are required")
	}

	for label, steps := range map[string][]Step{
		"steps":      s.Steps,
		"seed_steps": s.SeedSteps,
		"read_steps": s.ReadSteps,
	} {
		for i, step := range steps {
			if err := validateStep(label, i, step); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateStep(label string, i int, step Step) error {
	prefix := fmt.Sprintf("%s[%d]", label, i)
	if step.Op == "" {
		return fmt.Errorf("%s: op is required", prefix)
	}
	switch step.Op {
	case "sleep":
		return validateExpectation(prefix, step.Expect)
	case "create", "update", "save":
		if len(step.Item) == 0 {
			return fmt.Errorf("%s %s: item is required", prefix, step.Op)
		}
	case "get", "get_optional", "delete":
		if len(step.Key) == 0 {
			return fmt.Errorf("%s %s: key is required", prefix, step.Op)
		}
	case "query":
		if err := validateQueryReadRequest(step.Query, fmt.Sprintf("%s query", prefix)); err != nil {
			return err
		}
	case "scan":
		if err := validateScanReadRequest(step.Scan, fmt.Sprintf("%s scan", prefix)); err != nil {
			return err
		}
	case "count":
		if step.Count == nil {
			return fmt.Errorf("%s count: count is required", prefix)
		}
		hasQuery := step.Count.Query != nil
		hasScan := step.Count.Scan != nil
		if hasQuery == hasScan {
			return fmt.Errorf("%s count: exactly one of count.query or count.scan is required", prefix)
		}
		if hasQuery {
			if err := validateQueryReadRequest(step.Count.Query, fmt.Sprintf("%s count.query", prefix)); err != nil {
				return err
			}
		} else {
			if err := validateScanReadRequest(step.Count.Scan, fmt.Sprintf("%s count.scan", prefix)); err != nil {
				return err
			}
		}
	case "transact_get":
		if step.TransactGet == nil || len(step.TransactGet.Items) == 0 {
			return fmt.Errorf("%s transact_get: transact_get.items are required", prefix)
		}
		for j, item := range step.TransactGet.Items {
			if len(item.Key) == 0 {
				return fmt.Errorf("%s transact_get.items[%d]: key is required", prefix, j)
			}
		}
	case "batch_get":
		if step.BatchGet == nil || len(step.BatchGet.Keys) == 0 {
			return fmt.Errorf("%s batch_get: batch_get.keys are required", prefix)
		}
	case "batch_write":
		if step.BatchWrite == nil || (len(step.BatchWrite.Puts) == 0 && len(step.BatchWrite.Deletes) == 0) {
			return fmt.Errorf("%s batch_write: batch_write.puts or batch_write.deletes are required", prefix)
		}
	case "transact_write":
		if step.TransactWrite == nil || len(step.TransactWrite.Actions) == 0 {
			return fmt.Errorf("%s transact_write: transact_write.actions are required", prefix)
		}
		for j, action := range step.TransactWrite.Actions {
			if action.Kind == "" {
				return fmt.Errorf("%s transact_write.actions[%d]: kind is required", prefix, j)
			}
		}
	case "transition_append_event":
		if step.Actual == nil {
			return fmt.Errorf("%s transition_append_event: actual is required", prefix)
		}
		if step.Actual.Model == "" {
			return fmt.Errorf("%s transition_append_event: actual.model is required", prefix)
		}
		if len(step.Actual.Key) == 0 {
			return fmt.Errorf("%s transition_append_event: actual.key is required", prefix)
		}
		if len(step.Actual.Set) == 0 {
			return fmt.Errorf("%s transition_append_event: actual.set is required", prefix)
		}
		if step.Event == nil {
			return fmt.Errorf("%s transition_append_event: event is required", prefix)
		}
		if step.Event.Model == "" {
			return fmt.Errorf("%s transition_append_event: event.model is required", prefix)
		}
		if len(step.Event.Item) == 0 {
			return fmt.Errorf("%s transition_append_event: event.item is required", prefix)
		}
	case "validate_provenance":
		if len(step.Item) == 0 {
			return fmt.Errorf("%s validate_provenance: item is required", prefix)
		}
	default:
		return fmt.Errorf("%s: unsupported op %q", prefix, step.Op)
	}
	return validateExpectation(prefix, step.Expect)
}

func validateExpectation(prefix string, expect Expectation) error {
	for _, field := range []struct {
		name  string
		value map[string]any
	}{
		{name: "item_contains", value: expect.ItemContains},
		{name: "item_equals", value: expect.ItemEquals},
		{name: "raw_item_contains", value: expect.RawItemContains},
	} {
		if field.value != nil && len(field.value) == 0 {
			return fmt.Errorf("%s expect.%s must not be empty", prefix, field.name)
		}
	}
	for _, field := range []struct {
		name  string
		value map[string]string
	}{
		{name: "raw_attribute_types", value: expect.RawAttributeTypes},
		{name: "item_field_equals_var", value: expect.ItemFieldEqualsVar},
		{name: "item_field_not_equals_var", value: expect.ItemFieldNotEqualsVar},
	} {
		if field.value != nil && len(field.value) == 0 {
			return fmt.Errorf("%s expect.%s must not be empty", prefix, field.name)
		}
	}
	for _, field := range []struct {
		name  string
		value []string
	}{
		{name: "errors", value: expect.Errors},
		{name: "items_missing_fields", value: expect.ItemsMissingFields},
		{name: "item_has_fields", value: expect.ItemHasFields},
		{name: "item_missing_fields", value: expect.ItemMissingFields},
	} {
		if field.value != nil && len(field.value) == 0 {
			return fmt.Errorf("%s expect.%s must not be empty", prefix, field.name)
		}
	}
	if expect.ItemsContains != nil {
		if len(expect.ItemsContains) == 0 {
			return fmt.Errorf("%s expect.items_contains must not be empty", prefix)
		}
		for j, item := range expect.ItemsContains {
			if len(item) == 0 {
				return fmt.Errorf("%s expect.items_contains[%d] must not be empty", prefix, j)
			}
		}
	}

	hasErrorExpectation := expect.Error != "" || len(expect.Errors) > 0
	hasDataAssertion := expectationHasItemAssertion(expect) ||
		expectationHasRawItemAssertion(expect) ||
		expectationHasReadAssertion(expect)
	if hasErrorExpectation && hasDataAssertion {
		return fmt.Errorf("%s expect: item/read assertions cannot be combined with error expectations", prefix)
	}
	return nil
}

func expectationHasItemAssertion(expect Expectation) bool {
	return len(expect.ItemContains) > 0 ||
		len(expect.ItemEquals) > 0 ||
		len(expect.ItemHasFields) > 0 ||
		len(expect.ItemMissingFields) > 0 ||
		len(expect.RawAttributeTypes) > 0 ||
		len(expect.ItemFieldEqualsVar) > 0 ||
		len(expect.ItemFieldNotEqualsVar) > 0
}

func expectationHasRawItemAssertion(expect Expectation) bool {
	return len(expect.ItemMissingFields) > 0 ||
		len(expect.RawAttributeTypes) > 0 ||
		len(expect.RawItemContains) > 0
}

func expectationHasReadAssertion(expect Expectation) bool {
	return expect.ItemCount != nil ||
		expect.Count != nil ||
		len(expect.ItemsContains) > 0 ||
		len(expect.ItemsMissingFields) > 0 ||
		expect.CursorEquals != nil
}

func validateQueryReadRequest(req *ReadRequest, prefix string) error {
	if req == nil {
		return fmt.Errorf("%s: query is required", prefix)
	}
	if req.Partition == nil {
		return fmt.Errorf("%s: query.partition is required", prefix)
	}
	if err := validateReadCondition(req.Partition, fmt.Sprintf("%s.partition", prefix)); err != nil {
		return err
	}
	if req.Sort != nil {
		if err := validateReadCondition(req.Sort, fmt.Sprintf("%s.sort", prefix)); err != nil {
			return err
		}
	}
	for j := range req.Filter {
		if err := validateReadCondition(&req.Filter[j], fmt.Sprintf("%s.filter[%d]", prefix, j)); err != nil {
			return err
		}
	}
	return nil
}

func validateScanReadRequest(req *ReadRequest, prefix string) error {
	if req == nil {
		return fmt.Errorf("%s: scan is required", prefix)
	}
	for j := range req.Filter {
		if err := validateReadCondition(&req.Filter[j], fmt.Sprintf("%s.filter[%d]", prefix, j)); err != nil {
			return err
		}
	}
	return nil
}

func validateReadCondition(cond *ReadCondition, prefix string) error {
	if cond == nil {
		return fmt.Errorf("%s is required", prefix)
	}
	if cond.Attribute == "" {
		return fmt.Errorf("%s.attribute is required", prefix)
	}
	if cond.Operator == "" {
		return fmt.Errorf("%s.operator is required", prefix)
	}
	if cond.Value == nil && len(cond.Values) == 0 && !readOperatorAllowsNoValue(cond.Operator) {
		return fmt.Errorf("%s.value or %s.values is required", prefix, prefix)
	}
	return nil
}

func readOperatorAllowsNoValue(operator string) bool {
	switch strings.ToLower(operator) {
	case "exists", "attribute_exists", "not_exists", "attribute_not_exists":
		return true
	default:
		return false
	}
}

func MissingCapabilities(required []string, supported []string) []string {
	if len(required) == 0 {
		return nil
	}
	supportedSet := make(map[string]struct{}, len(supported))
	for _, capability := range supported {
		supportedSet[capability] = struct{}{}
	}
	var missing []string
	for _, capability := range required {
		if _, ok := supportedSet[capability]; !ok {
			missing = append(missing, capability)
		}
	}
	return missing
}
