// Package marshal provides safe marshaling for DynamoDB without unsafe operations
package marshal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSecurityConfigurationBasic tests basic security configuration
func TestSecurityConfigurationBasic(t *testing.T) {
	t.Run("DefaultConfigIsSafe", func(t *testing.T) {
		config := DefaultConfig()
		assert.Equal(t, SafeMarshalerType, config.MarshalerType)
		assert.False(t, config.AllowUnsafeMarshaler)
		assert.True(t, config.RequireExplicitUnsafeAck)
		assert.True(t, config.WarnOnUnsafeUsage)
	})

	t.Run("ValidateSecureConfig", func(t *testing.T) {
		config := Config{
			MarshalerType:            SafeMarshalerType,
			AllowUnsafeMarshaler:     false,
			RequireExplicitUnsafeAck: true,
			WarnOnUnsafeUsage:        true,
		}
		err := ValidateConfig(config)
		assert.NoError(t, err)
	})

	t.Run("RejectInvalidUnsafeConfig", func(t *testing.T) {
		config := Config{
			MarshalerType:        UnsafeMarshalerType,
			AllowUnsafeMarshaler: false, // Invalid: unsafe type but not allowed
		}
		err := ValidateConfig(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsafe marshaler type specified but not explicitly allowed")
	})
}

// TestMarshalerFactoryBasic tests basic factory behavior
func TestMarshalerFactoryBasic(t *testing.T) {
	t.Run("FactoryDefaultsToSafe", func(t *testing.T) {
		factory := NewMarshalerFactory(DefaultConfig())
		marshaler, err := factory.NewMarshaler()
		assert.NoError(t, err)
		assert.IsType(t, &SafeMarshaler{}, marshaler)
	})

	t.Run("UnsafeMarshalerRequiresExplicitAllow", func(t *testing.T) {
		config := Config{
			MarshalerType:        UnsafeMarshalerType,
			AllowUnsafeMarshaler: false,
		}
		factory := NewMarshalerFactory(config)
		_, err := factory.NewMarshaler()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsafe marshaler not allowed")
	})

	t.Run("UnsafeMarshalerRequiresAcknowledgment", func(t *testing.T) {
		config := Config{
			MarshalerType:            UnsafeMarshalerType,
			AllowUnsafeMarshaler:     true,
			RequireExplicitUnsafeAck: true,
		}
		factory := NewMarshalerFactory(config)

		// Without acknowledgment should fail
		_, err := factory.NewMarshaler()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "requires explicit security acknowledgment")

		// With acknowledgment should succeed
		ack := CreateSecurityAcknowledgment("security-test@company.com")
		marshaler, err := factory.NewMarshalerWithAcknowledgment(ack)
		assert.NoError(t, err)
		assert.IsType(t, &Marshaler{}, marshaler) // Original unsafe marshaler
	})

	t.Run("IncompleteAcknowledgmentRejected", func(t *testing.T) {
		config := Config{
			MarshalerType:            UnsafeMarshalerType,
			AllowUnsafeMarshaler:     true,
			RequireExplicitUnsafeAck: true,
		}
		factory := NewMarshalerFactory(config)

		// Incomplete acknowledgment
		ack := &SecurityAcknowledgment{
			AcknowledgeMemoryCorruptionRisk: true,
			AcknowledgeSecurityVulnerable:   false, // Missing
			AcknowledgeDeprecationWarning:   true,
			DeveloperSignature:              "test@company.com",
		}

		_, err := factory.NewMarshalerWithAcknowledgment(ack)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "incomplete security acknowledgment")
	})
}

// TestSafeMarshalerBasic tests basic safe marshaler functionality
func TestSafeMarshalerBasic(t *testing.T) {
	marshaler := NewSafeMarshaler()

	t.Run("MarshalerCreated", func(t *testing.T) {
		assert.NotNil(t, marshaler)
		assert.IsType(t, &SafeMarshaler{}, marshaler)
	})

	t.Run("HandlesNilPointer", func(t *testing.T) {
		var nilModel *TestModel
		_, err := marshaler.MarshalItem(nilModel, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nil pointer")
	})

	t.Run("RejectsNonStruct", func(t *testing.T) {
		_, err := marshaler.MarshalItem("not a struct", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be a struct")
	})
}

// TestSecurityAcknowledgment tests acknowledgment creation
func TestSecurityAcknowledgment(t *testing.T) {
	t.Run("CreateAcknowledgment", func(t *testing.T) {
		ack := CreateSecurityAcknowledgment("test-developer@company.com")

		assert.True(t, ack.AcknowledgeMemoryCorruptionRisk)
		assert.True(t, ack.AcknowledgeSecurityVulnerable)
		assert.True(t, ack.AcknowledgeDeprecationWarning)
		assert.Equal(t, "test-developer@company.com", ack.DeveloperSignature)
		assert.True(t, ack.Timestamp > 0)
	})
}

// TestSecurityStats tests security monitoring
func TestSecurityStats(t *testing.T) {
	t.Run("GetSecurityStats", func(t *testing.T) {
		stats := GetSecurityStats()
		assert.GreaterOrEqual(t, stats.UnsafeUsageCount, int64(0))
		assert.GreaterOrEqual(t, stats.SecurityWarnings, int64(0))
		assert.NotEmpty(t, stats.CurrentConfig.MarshalerType)
	})
}

// TestModel for testing
type TestModel struct {
	ID   string `theorydb:"pk"`
	Name string `theorydb:"attr:name"`
}
