package consistency

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultRetryConfig_RetryCondition(t *testing.T) {
	cfg := DefaultRetryConfig()
	require.NotNil(t, cfg)
	require.NotNil(t, cfg.RetryCondition)

	require.True(t, cfg.RetryCondition(nil, nil))
	require.True(t, cfg.RetryCondition(struct{}{}, errors.New("boom")))
	require.False(t, cfg.RetryCondition(struct{}{}, nil))
}
