package keycontract

// ContractVersion is the original sidecar version supported by TableTheory.
const ContractVersion = "0.1"

const (
	ContractVersionV01 = "0.1"
	ContractVersionV02 = "0.2"
)

const (
	// TransformTrim removes leading and trailing contract whitespace using the
	// explicit codepoint set documented in the v0.1 sidecar spec.
	TransformTrim = "trim"
	// TransformWildcardEmpty maps the empty string to "*". It does not trim by itself;
	// use TransformTrim before it when whitespace should be ignored.
	TransformWildcardEmpty = "wildcard_empty"
	// TransformLowercase applies ASCII-only lowercase. It does not use locale or
	// Unicode case folding; bytes outside A-Z are unchanged.
	TransformLowercase = "lowercase"
	// TransformURLEncode percent-encodes UTF-8 bytes with uppercase hex,
	// leaving only RFC 3986 unreserved bytes unchanged.
	TransformURLEncode = "url_encode"
)

// Contract is the language-neutral TableTheory model-contract sidecar.
// It is intentionally separate from DMS v0.1 core so derived keys can be
// adopted additively without changing DMS model semantics.
type Contract struct {
	Version     string          `yaml:"tabletheory_model_contract_version" json:"tabletheory_model_contract_version"`
	Namespace   string          `yaml:"namespace,omitempty" json:"namespace,omitempty"`
	DMSVersion  string          `yaml:"dms_version,omitempty" json:"dms_version,omitempty"`
	Models      []ModelContract `yaml:"models,omitempty" json:"models,omitempty"`
	DerivedKeys []DerivedKey    `yaml:"derived_keys" json:"derived_keys"`
}

// ModelContract links optional sidecar metadata to a DMS model. Runtime
// evaluation does not require this section; consumers can use it for discovery.
type ModelContract struct {
	Name        string   `yaml:"name" json:"name"`
	DMSModel    string   `yaml:"dms_model,omitempty" json:"dms_model,omitempty"`
	Table       string   `yaml:"table,omitempty" json:"table,omitempty"`
	DerivedKeys []string `yaml:"derived_keys,omitempty" json:"derived_keys,omitempty"`
}

// DerivedKey describes one derived key formula. The formula is product-agnostic:
// it is an ordered segment template plus input names, transforms, defaults, and
// omission rules.
type DerivedKey struct {
	Name        string    `yaml:"name" json:"name"`
	Helper      string    `yaml:"helper,omitempty" json:"helper,omitempty"`
	Description string    `yaml:"description,omitempty" json:"description,omitempty"`
	Join        string    `yaml:"join" json:"join"`
	Inputs      []Input   `yaml:"inputs,omitempty" json:"inputs,omitempty"`
	Segments    []Segment `yaml:"segments" json:"segments"`
	Fixtures    []Fixture `yaml:"fixtures,omitempty" json:"fixtures,omitempty"`
}

// Input declares a named evaluator input and optional TypeScript helper metadata.
type Input struct {
	Name     string `yaml:"name" json:"name"`
	TSName   string `yaml:"ts_name,omitempty" json:"ts_name,omitempty"`
	Type     string `yaml:"type,omitempty" json:"type,omitempty"`
	Optional bool   `yaml:"optional,omitempty" json:"optional,omitempty"`
}

// Segment is one ordered piece of a derived key.
type Segment struct {
	Literal    *string     `yaml:"literal,omitempty" json:"literal,omitempty"`
	Default    *string     `yaml:"default,omitempty" json:"default,omitempty"`
	Value      ValueSource `yaml:"value,omitempty" json:"value,omitempty"`
	Name       string      `yaml:"name,omitempty" json:"name,omitempty"`
	Prefix     string      `yaml:"prefix,omitempty" json:"prefix,omitempty"`
	Suffix     string      `yaml:"suffix,omitempty" json:"suffix,omitempty"`
	Transforms []string    `yaml:"transforms,omitempty" json:"transforms,omitempty"`
	OmitWhen   OmitWhen    `yaml:"omit_when,omitempty" json:"omit_when,omitempty"`
	Optional   bool        `yaml:"optional,omitempty" json:"optional,omitempty"`
}

// ValueSource identifies where a segment obtains its value. Use exactly one of
// input or literal unless the segment-level literal shortcut is used.
type ValueSource struct {
	Literal *string `yaml:"literal,omitempty" json:"literal,omitempty"`
	Input   string  `yaml:"input,omitempty" json:"input,omitempty"`
}

// OmitWhen controls conditional omission after transforms and defaults have been
// applied. Comparisons are exact string comparisons.
type OmitWhen struct {
	Values  []string `yaml:"values,omitempty" json:"values,omitempty"`
	Empty   bool     `yaml:"empty,omitempty" json:"empty,omitempty"`
	Default bool     `yaml:"default,omitempty" json:"default,omitempty"`
}

// Fixture is a golden input/output case for a derived key.
type Fixture struct {
	Name        string         `yaml:"name" json:"name"`
	Description string         `yaml:"description,omitempty" json:"description,omitempty"`
	Input       map[string]any `yaml:"input" json:"input"`
	Expect      string         `yaml:"expect" json:"expect"`
}
