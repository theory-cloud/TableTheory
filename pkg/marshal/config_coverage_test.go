package marshal

import (
	"testing"

	"github.com/stretchr/testify/require"

	pkgTypes "github.com/theory-cloud/tabletheory/v3/pkg/types"
)

func TestConfig_SetGlobalConfig_RoundTrip(t *testing.T) {
	original := GetGlobalConfig()
	t.Cleanup(func() {
		SetGlobalConfig(original)
	})

	cfg := DefaultConfig()
	cfg.WarnOnUnsafeUsage = false

	SetGlobalConfig(cfg)
	require.Equal(t, cfg, GetGlobalConfig())
}

func TestMarshalerFactory_WithConverter_UsesProvidedConverter(t *testing.T) {
	t.Setenv("DYNAMORM_FORCE_SAFE_MARSHALER", "false")

	cfg := DefaultConfig()
	cfg.MarshalerType = UnsafeMarshalerType
	cfg.AllowUnsafeMarshaler = true
	cfg.RequireExplicitUnsafeAck = true
	cfg.WarnOnUnsafeUsage = false

	converter := pkgTypes.NewConverter()
	factory := NewMarshalerFactory(cfg).WithConverter(converter)

	ack := CreateSecurityAcknowledgment("coverage@test")
	marshaler, err := factory.NewMarshalerWithAcknowledgment(ack)
	require.NoError(t, err)

	unsafeMarshaler, ok := marshaler.(*Marshaler)
	require.True(t, ok, "expected unsafe marshaler implementation, got %T", marshaler)
	require.Same(t, converter, unsafeMarshaler.converter)
}

func TestMarshalerFactory_SafeConverterAndNilNow(t *testing.T) {
	cfg := DefaultConfig()
	converter := pkgTypes.NewConverter()

	factory := NewMarshalerFactory(cfg).WithConverter(converter).WithNowFunc(nil)
	marshaler, err := factory.NewMarshaler()
	require.NoError(t, err)

	safeMarshaler, ok := marshaler.(*SafeMarshaler)
	require.True(t, ok, "expected safe marshaler implementation, got %T", marshaler)
	require.Same(t, converter, safeMarshaler.converter)
	require.NotNil(t, safeMarshaler.now)
}

func TestMarshalerFactory_UnsafeAckMissingSignature(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MarshalerType = UnsafeMarshalerType
	cfg.AllowUnsafeMarshaler = true
	cfg.RequireExplicitUnsafeAck = true
	cfg.WarnOnUnsafeUsage = false

	ack := &SecurityAcknowledgment{
		AcknowledgeMemoryCorruptionRisk: true,
		AcknowledgeSecurityVulnerable:   true,
		AcknowledgeDeprecationWarning:   true,
	}
	_, err := NewMarshalerFactory(cfg).NewMarshalerWithAcknowledgment(ack)
	require.ErrorContains(t, err, "developer signature required")
}

func TestMarshalerFactory_ForceSafeOverride(t *testing.T) {
	t.Setenv("DYNAMORM_FORCE_SAFE_MARSHALER", "true")

	cfg := DefaultConfig()
	cfg.MarshalerType = UnsafeMarshalerType
	cfg.AllowUnsafeMarshaler = true
	cfg.RequireExplicitUnsafeAck = false
	cfg.WarnOnUnsafeUsage = false

	factory := NewMarshalerFactory(cfg)
	marshaler, err := factory.NewMarshalerWithAcknowledgment(CreateSecurityAcknowledgment("coverage@test"))
	require.NoError(t, err)
	require.IsType(t, &SafeMarshaler{}, marshaler)
}
