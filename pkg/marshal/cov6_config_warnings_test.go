package marshal

import (
	"io"
	"log"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarshalerFactory_WarnOnUnsafeUsage_IncrementsCounters_COV6(t *testing.T) {
	t.Setenv("DYNAMORM_FORCE_SAFE_MARSHALER", "false")

	prevWriter := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(io.Discard)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	})

	cfg := DefaultConfig()
	cfg.MarshalerType = UnsafeMarshalerType
	cfg.AllowUnsafeMarshaler = true
	cfg.RequireExplicitUnsafeAck = false
	cfg.WarnOnUnsafeUsage = true

	startUnsafe := atomic.LoadInt64(&unsafeUsageCounter)
	startWarnings := atomic.LoadInt64(&securityWarnings)

	factory := NewMarshalerFactory(cfg)
	marshaler, err := factory.NewMarshalerWithAcknowledgment(CreateSecurityAcknowledgment("cov6@test"))
	require.NoError(t, err)
	require.IsType(t, &Marshaler{}, marshaler)

	require.Greater(t, atomic.LoadInt64(&unsafeUsageCounter), startUnsafe)
	require.Greater(t, atomic.LoadInt64(&securityWarnings), startWarnings)
}

func TestValidateConfig_WarnsWhenAckRequiredButWarningsDisabled_COV6(t *testing.T) {
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(io.Discard)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	})

	cfg := Config{
		MarshalerType:            UnsafeMarshalerType,
		AllowUnsafeMarshaler:     true,
		RequireExplicitUnsafeAck: true,
		WarnOnUnsafeUsage:        false,
	}

	require.NoError(t, ValidateConfig(cfg))
}

func TestMarshalerFactory_NewMarshaler_UnknownTypeErrors_COV6(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MarshalerType = "unknown"

	_, err := NewMarshalerFactory(cfg).NewMarshaler()
	require.Error(t, err)
}
