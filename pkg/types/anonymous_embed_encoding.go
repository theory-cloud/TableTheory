package types

// WithFlatAnonymousEmbedEncoding opts this converter into flat promoted-field
// encoding for exported anonymous embedded structs during helper marshaling.
//
// The default remains the legacy nested anonymous-container shape so existing
// production writes are unchanged unless callers opt in explicitly.
func (c *Converter) WithFlatAnonymousEmbedEncoding() *Converter {
	if c == nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.flatAnonymousEmbedEncoding = true
	return c
}

// FlatAnonymousEmbedEncodingEnabled reports whether this converter is
// configured to flatten exported promoted fields that originate from anonymous
// embedded structs during helper marshaling.
func (c *Converter) FlatAnonymousEmbedEncodingEnabled() bool {
	if c == nil {
		return false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.flatAnonymousEmbedEncoding
}
