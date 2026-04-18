package expr

// ConvertOptions configures generic helper marshaling behavior.
type ConvertOptions struct {
	// FlatAnonymousEmbedEncoding flattens exported promoted fields that
	// originate from anonymous embedded structs instead of preserving the
	// legacy nested anonymous-container helper shape.
	FlatAnonymousEmbedEncoding bool
}

type flatAnonymousEmbedEncodingLookup interface {
	FlatAnonymousEmbedEncodingEnabled() bool
}

func convertOptionsRequestFlatAnonymousEmbeds(opts ConvertOptions) bool {
	return opts.FlatAnonymousEmbedEncoding
}

func converterRequestsFlatAnonymousEmbeds(converter any) bool {
	if converter == nil {
		return false
	}

	lookup, ok := converter.(flatAnonymousEmbedEncodingLookup)
	return ok && lookup.FlatAnonymousEmbedEncodingEnabled()
}
