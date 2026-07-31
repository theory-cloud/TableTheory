package expr

// ConvertOptions configures generic helper marshaling behavior.
type ConvertOptions struct {
	// Converter provides custom conversion hooks while sharing the generic
	// AttributeValue conversion implementation.
	Converter ConverterLookup

	// FlatAnonymousEmbedEncoding flattens exported promoted fields that
	// originate from anonymous embedded structs instead of preserving the
	// legacy nested anonymous-container helper shape.
	FlatAnonymousEmbedEncoding bool

	// LegacyStructFieldNames preserves pkg/types.Converter's historical
	// untagged struct-field behavior: exported fields without theorydb tags use
	// their Go field names rather than default camelCase/json-derived names.
	LegacyStructFieldNames bool

	// OmitZeroFieldsByDefault preserves pkg/types.Converter's historical
	// reflect.Value.IsZero behavior for untagged legacy struct fields. Explicitly
	// tagged fields follow their omitempty option; non-nil empty collections are
	// preserved unless omitempty is explicit.
	OmitZeroFieldsByDefault bool

	// FixedFloatFormat preserves pkg/types.Converter's historical decimal-string
	// formatting for floats instead of expression helpers' compact %g format.
	FixedFloatFormat bool
}

func convertOptionsRequestFlatAnonymousEmbeds(opts ConvertOptions) bool {
	return opts.FlatAnonymousEmbedEncoding
}
