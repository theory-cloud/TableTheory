package dms

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/theory-cloud/tabletheory/pkg/model"
	"github.com/theory-cloud/tabletheory/pkg/naming"
)

type Document struct {
	DMSVersion string  `yaml:"dms_version" json:"dms_version"`
	Namespace  string  `yaml:"namespace" json:"namespace"`
	Models     []Model `yaml:"models" json:"models"`
}

type Model struct {
	Name       string      `yaml:"name" json:"name"`
	Table      Table       `yaml:"table" json:"table"`
	Naming     Naming      `yaml:"naming" json:"naming"`
	Keys       Keys        `yaml:"keys" json:"keys"`
	Attributes []Attribute `yaml:"attributes" json:"attributes"`
	Indexes    []Index     `yaml:"indexes" json:"indexes"`
}

type Table struct {
	Name string `yaml:"name" json:"name"`
}

type Naming struct {
	Convention string `yaml:"convention" json:"convention"`
}

type Keys struct {
	Sort      *KeyAttribute `yaml:"sort" json:"sort"`
	Partition KeyAttribute  `yaml:"partition" json:"partition"`
}

type KeyAttribute struct {
	Attribute string `yaml:"attribute" json:"attribute"`
	Type      string `yaml:"type" json:"type"`
}

type Attribute struct {
	Attribute string `yaml:"attribute" json:"attribute"`
	Type      string `yaml:"type" json:"type"`
	Format    string `yaml:"format" json:"format"`

	Encryption any      `yaml:"encryption" json:"encryption"`
	Roles      []string `yaml:"roles" json:"roles"`

	Required  bool `yaml:"required" json:"required"`
	Optional  bool `yaml:"optional" json:"optional"`
	OmitEmpty bool `yaml:"omit_empty" json:"omit_empty"`

	JSON   bool `yaml:"json" json:"json"`
	Binary bool `yaml:"binary" json:"binary"`
}

type Index struct {
	Name       string        `yaml:"name" json:"name"`
	Type       string        `yaml:"type" json:"type"` // GSI | LSI
	Partition  KeyAttribute  `yaml:"partition" json:"partition"`
	Sort       *KeyAttribute `yaml:"sort" json:"sort"`
	Projection Projection    `yaml:"projection" json:"projection"`
}

type Projection struct {
	Type   string   `yaml:"type" json:"type"` // ALL | KEYS_ONLY | INCLUDE
	Fields []string `yaml:"fields" json:"fields"`
}

func ParseDocument(data []byte) (*Document, error) {
	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse DMS YAML/JSON: %w", err)
	}

	normalized, err := normalizeJSONCompatible(raw, "dms")
	if err != nil {
		return nil, err
	}

	b, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("marshal normalized DMS: %w", err)
	}

	var doc Document
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("decode DMS document: %w", err)
	}

	if err := validateDocument(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func FindModel(doc *Document, name string) (*Model, bool) {
	if doc == nil {
		return nil, false
	}
	for i := range doc.Models {
		if doc.Models[i].Name == name {
			return &doc.Models[i], true
		}
	}
	return nil, false
}

type CompareOptions struct {
	IgnoreTableName bool
}

func AssertModelsEquivalent(got, want Model, opts CompareOptions) error {
	ng := normalizeForCompare(got, opts)
	nw := normalizeForCompare(want, opts)
	if !reflect.DeepEqual(ng, nw) {
		gb, gerr := json.MarshalIndent(ng, "", "  ")
		wb, werr := json.MarshalIndent(nw, "", "  ")
		if gerr == nil && werr == nil {
			return fmt.Errorf("models not equivalent\nwant=%s\ngot=%s", string(wb), string(gb))
		}
		return fmt.Errorf("models not equivalent\nwant=%#v\ngot=%#v\nerrors: want=%v got=%v", nw, ng, werr, gerr)
	}
	return nil
}

func FromMetadata(meta *model.Metadata) (Model, error) {
	if meta == nil || meta.PrimaryKey == nil || meta.PrimaryKey.PartitionKey == nil {
		return Model{}, fmt.Errorf("metadata missing primary key")
	}

	out := Model{
		Name: meta.Type.Name(),
		Table: Table{
			Name: meta.TableName,
		},
		Naming: Naming{Convention: namingConventionString(meta.NamingConvention)},
		Keys: Keys{
			Partition: KeyAttribute{
				Attribute: meta.PrimaryKey.PartitionKey.DBName,
				Type:      scalarKeyTypeFromField(meta.PrimaryKey.PartitionKey.Type),
			},
		},
		Attributes: make([]Attribute, 0, len(meta.FieldsByDBName)),
		Indexes:    make([]Index, 0, len(meta.Indexes)),
	}

	if meta.PrimaryKey.SortKey != nil {
		out.Keys.Sort = &KeyAttribute{
			Attribute: meta.PrimaryKey.SortKey.DBName,
			Type:      scalarKeyTypeFromField(meta.PrimaryKey.SortKey.Type),
		}
	}

	for _, field := range meta.FieldsByDBName {
		attrType, err := attributeTypeFromField(field.Type, field.IsSet, field.Tags)
		if err != nil {
			return Model{}, fmt.Errorf("attribute %s: %w", field.DBName, err)
		}

		out.Attributes = append(out.Attributes, Attribute{
			Attribute: field.DBName,
			Type:      attrType,
			Roles:     rolesFromField(field),
			Required:  field.IsPK || field.IsSK,
			Optional:  !field.IsPK && !field.IsSK,
			OmitEmpty: field.OmitEmpty,
			JSON:      hasModifierTag(field.Tags, "json"),
			Binary:    hasModifierTag(field.Tags, "binary"),
			Encryption: func() any {
				if field.IsEncrypted {
					return map[string]any{"v": 1}
				}
				return nil
			}(),
		})
	}
	sort.Slice(out.Attributes, func(i, j int) bool { return out.Attributes[i].Attribute < out.Attributes[j].Attribute })

	for _, idx := range meta.Indexes {
		if idx.PartitionKey == nil {
			return Model{}, fmt.Errorf("index %s missing partition key", idx.Name)
		}

		pkType := scalarKeyTypeFromField(idx.PartitionKey.Type)
		var sk *KeyAttribute
		if idx.SortKey != nil {
			t := scalarKeyTypeFromField(idx.SortKey.Type)
			sk = &KeyAttribute{Attribute: idx.SortKey.DBName, Type: t}
		}

		proj := Projection{
			Type:   idx.ProjectionType,
			Fields: append([]string(nil), idx.ProjectedFields...),
		}
		if proj.Type == "" {
			proj.Type = "ALL"
		}

		out.Indexes = append(out.Indexes, Index{
			Name:       idx.Name,
			Type:       string(idx.Type),
			Partition:  KeyAttribute{Attribute: idx.PartitionKey.DBName, Type: pkType},
			Sort:       sk,
			Projection: proj,
		})
	}
	sort.Slice(out.Indexes, func(i, j int) bool { return out.Indexes[i].Name < out.Indexes[j].Name })

	return out, nil
}

func validateDocument(doc *Document) error {
	if doc == nil {
		return fmt.Errorf("DMS document is nil")
	}
	if doc.DMSVersion != "0.1" {
		return fmt.Errorf("unsupported dms_version: %q", doc.DMSVersion)
	}
	if len(doc.Models) == 0 {
		return fmt.Errorf("DMS document must include models[]")
	}
	for _, m := range doc.Models {
		if err := validateModel(m); err != nil {
			return err
		}
	}
	return nil
}

func normalizeJSONCompatible(value any, path string) (any, error) {
	switch v := value.(type) {
	case nil, string, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return v, nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("DMS contains non-finite number at %s", path)
		}
		return v, nil
	case []any:
		return normalizeJSONSlice(v, path)
	case map[string]any:
		return normalizeJSONStringMap(v, path)
	case map[any]any:
		return normalizeJSONAnyMap(v, path)
	default:
		return nil, fmt.Errorf("DMS contains non-JSON value at %s (%T)", path, v)
	}
}

type normalizedModel struct {
	Name       string            `json:"name"`
	TableName  string            `json:"table_name,omitempty"`
	Naming     string            `json:"naming,omitempty"`
	Keys       Keys              `json:"keys"`
	Attributes []normalizedAttr  `json:"attributes"`
	Indexes    []normalizedIndex `json:"indexes"`
}

type normalizedAttr struct {
	Attribute string   `json:"attribute"`
	Type      string   `json:"type"`
	Roles     []string `json:"roles,omitempty"`
	OmitEmpty bool     `json:"omit_empty,omitempty"`
	Required  bool     `json:"required,omitempty"`
	Optional  bool     `json:"optional,omitempty"`
	Encrypted bool     `json:"encrypted,omitempty"`
	JSON      bool     `json:"json,omitempty"`
	Binary    bool     `json:"binary,omitempty"`
}

type normalizedIndex struct {
	Name       string        `json:"name"`
	Type       string        `json:"type"`
	Partition  KeyAttribute  `json:"partition"`
	Sort       *KeyAttribute `json:"sort,omitempty"`
	Projection Projection    `json:"projection,omitempty"`
}

func normalizeForCompare(m Model, opts CompareOptions) normalizedModel {
	convention := m.Naming.Convention
	if convention == "" {
		convention = "camelCase"
	}

	out := normalizedModel{
		Name:   m.Name,
		Naming: convention,
		Keys:   m.Keys,
	}
	if !opts.IgnoreTableName {
		out.TableName = m.Table.Name
	}

	for _, a := range m.Attributes {
		roles := append([]string(nil), a.Roles...)
		sort.Strings(roles)
		out.Attributes = append(out.Attributes, normalizedAttr{
			Attribute: a.Attribute,
			Type:      a.Type,
			Roles:     roles,
			OmitEmpty: a.OmitEmpty,
			Required:  a.Required,
			Optional:  a.Optional,
			Encrypted: a.Encryption != nil,
			JSON:      a.JSON,
			Binary:    a.Binary,
		})
	}
	sort.Slice(out.Attributes, func(i, j int) bool { return out.Attributes[i].Attribute < out.Attributes[j].Attribute })

	for _, idx := range m.Indexes {
		fields := append([]string(nil), idx.Projection.Fields...)
		sort.Strings(fields)
		proj := idx.Projection
		proj.Fields = fields
		out.Indexes = append(out.Indexes, normalizedIndex{
			Name:       idx.Name,
			Type:       idx.Type,
			Partition:  idx.Partition,
			Sort:       idx.Sort,
			Projection: proj,
		})
	}
	sort.Slice(out.Indexes, func(i, j int) bool { return out.Indexes[i].Name < out.Indexes[j].Name })

	return out
}

func rolesFromField(f *model.FieldMetadata) []string {
	var roles []string
	if f.IsPK {
		roles = append(roles, "pk")
	}
	if f.IsSK {
		roles = append(roles, "sk")
	}
	if f.IsCreatedAt {
		roles = append(roles, "created_at")
	}
	if f.IsUpdatedAt {
		roles = append(roles, "updated_at")
	}
	if f.IsVersion {
		roles = append(roles, "version")
	}
	if f.IsTTL {
		roles = append(roles, "ttl")
	}
	sort.Strings(roles)
	return roles
}

func namingConventionString(c naming.Convention) string {
	switch c {
	case naming.SnakeCase:
		return "snake_case"
	case naming.CamelCase:
		fallthrough
	default:
		return "camelCase"
	}
}

func scalarKeyTypeFromField(t reflect.Type) string {
	t = derefType(t)
	switch t.Kind() {
	case reflect.String:
		return "S"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "N"
	case reflect.Slice:
		// []byte commonly represents binary keys.
		if t.Elem().Kind() == reflect.Uint8 {
			return "B"
		}
		return "B"
	default:
		// Fall back to S for unknown types.
		return "S"
	}
}

func attributeTypeFromField(t reflect.Type, isSet bool, tags map[string]string) (string, error) {
	t = derefType(t)

	if isSet {
		return attributeTypeFromSetField(t)
	}

	if attrType, ok := attributeTypeFromTags(tags); ok {
		return attrType, nil
	}

	return attributeTypeFromKind(t), nil
}

func attributeTypeFromTags(tags map[string]string) (string, bool) {
	if tags == nil {
		return "", false
	}
	if _, ok := tags["binary"]; ok {
		return "B", true
	}
	if _, ok := tags["json"]; ok {
		// JSON fields are stored as a string blob.
		return "S", true
	}
	return "", false
}

func hasModifierTag(tags map[string]string, key string) bool {
	if tags == nil {
		return false
	}
	_, ok := tags[key]
	return ok
}

func attributeTypeFromSetField(t reflect.Type) (string, error) {
	if t.Kind() != reflect.Slice && t.Kind() != reflect.Array {
		return "", fmt.Errorf("set fields must be slice/array (got %s)", t.Kind())
	}
	elem := derefType(t.Elem())
	switch elem.Kind() {
	case reflect.String:
		return "SS", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "NS", nil
	case reflect.Slice:
		if elem.Elem().Kind() == reflect.Uint8 {
			return "BS", nil
		}
		return "", fmt.Errorf("unsupported set element kind: %s", elem.Kind())
	default:
		return "", fmt.Errorf("unsupported set element kind: %s", elem.Kind())
	}
}

func attributeTypeFromKind(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "S"
	case reflect.Bool:
		return "BOOL"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "N"
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return "B"
		}
		return "L"
	case reflect.Map, reflect.Struct:
		return "M"
	default:
		return "S"
	}
}

func derefType(t reflect.Type) reflect.Type {
	for t != nil && t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func validateModel(m Model) error {
	if isBlank(m.Name) {
		return fmt.Errorf("DMS model missing name")
	}
	if isBlank(m.Table.Name) {
		return fmt.Errorf("DMS model %s: missing table.name", m.Name)
	}
	if err := validateModelKeys(m); err != nil {
		return err
	}
	if len(m.Attributes) == 0 {
		return fmt.Errorf("DMS model %s: missing attributes[]", m.Name)
	}
	seen, err := validateModelAttributes(m)
	if err != nil {
		return err
	}
	return validateModelKeyAttributesPresent(m, seen)
}

func validateModelKeys(m Model) error {
	if isBlank(m.Keys.Partition.Attribute) || isBlank(m.Keys.Partition.Type) {
		return fmt.Errorf("DMS model %s: missing keys.partition", m.Name)
	}
	if m.Keys.Sort == nil {
		return nil
	}
	if isBlank(m.Keys.Sort.Attribute) || isBlank(m.Keys.Sort.Type) {
		return fmt.Errorf("DMS model %s: invalid keys.sort", m.Name)
	}
	return nil
}

func validateModelAttributes(m Model) (map[string]struct{}, error) {
	seen := make(map[string]struct{}, len(m.Attributes))
	for _, a := range m.Attributes {
		if isBlank(a.Attribute) || isBlank(a.Type) {
			return nil, fmt.Errorf("DMS model %s: attribute missing attribute/type", m.Name)
		}
		if _, ok := seen[a.Attribute]; ok {
			return nil, fmt.Errorf("DMS model %s: duplicate attribute %s", m.Name, a.Attribute)
		}
		if a.Required && a.Optional {
			return nil, fmt.Errorf("DMS model %s: attribute %s cannot be both required and optional", m.Name, a.Attribute)
		}
		if a.JSON && a.Type != "S" {
			return nil, fmt.Errorf("DMS model %s: attribute %s json requires type S (got %s)", m.Name, a.Attribute, a.Type)
		}
		if a.Binary && a.Type != "B" {
			return nil, fmt.Errorf("DMS model %s: attribute %s binary requires type B (got %s)", m.Name, a.Attribute, a.Type)
		}
		if a.JSON && a.Binary {
			return nil, fmt.Errorf("DMS model %s: attribute %s cannot be both json and binary", m.Name, a.Attribute)
		}
		seen[a.Attribute] = struct{}{}
	}
	return seen, nil
}

func validateModelKeyAttributesPresent(m Model, seen map[string]struct{}) error {
	// Ensure keys exist in attributes[].
	if _, ok := seen[m.Keys.Partition.Attribute]; !ok {
		return fmt.Errorf("DMS model %s: missing partition key attribute %s", m.Name, m.Keys.Partition.Attribute)
	}
	if m.Keys.Sort == nil {
		return nil
	}
	if _, ok := seen[m.Keys.Sort.Attribute]; !ok {
		return fmt.Errorf("DMS model %s: missing sort key attribute %s", m.Name, m.Keys.Sort.Attribute)
	}
	return nil
}

func isBlank(s string) bool {
	return strings.TrimSpace(s) == ""
}

func normalizeJSONSlice(values []any, path string) ([]any, error) {
	out := make([]any, 0, len(values))
	for i := range values {
		elem, err := normalizeJSONCompatible(values[i], fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			return nil, err
		}
		out = append(out, elem)
	}
	return out, nil
}

func normalizeJSONStringMap(values map[string]any, path string) (map[string]any, error) {
	out := make(map[string]any, len(values))
	for k, vv := range values {
		elem, err := normalizeJSONCompatible(vv, path+"."+k)
		if err != nil {
			return nil, err
		}
		out[k] = elem
	}
	return out, nil
}

func normalizeJSONAnyMap(values map[any]any, path string) (map[string]any, error) {
	out := make(map[string]any, len(values))
	for kk, vv := range values {
		key, ok := kk.(string)
		if !ok {
			return nil, fmt.Errorf("DMS contains non-string key at %s", path)
		}
		elem, err := normalizeJSONCompatible(vv, path+"."+key)
		if err != nil {
			return nil, err
		}
		out[key] = elem
	}
	return out, nil
}
