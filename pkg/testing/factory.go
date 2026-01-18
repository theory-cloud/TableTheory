// Package testing provides utilities for testing applications that use TableTheory.
// It includes mock factories, test helpers, and common testing scenarios.
package testing

import (
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

// DBFactory is an interface for creating TableTheory database instances.
// It allows for dependency injection in middleware and makes testing easier.
// This version returns ExtendedDB for full compatibility.
type DBFactory interface {
	// CreateDB creates a new database instance with the given configuration
	CreateDB(config session.Config) (core.ExtendedDB, error)
}

// Note: In the future, we may add a CoreDBFactory that returns a subset
// of ExtendedDB methods for simpler use cases. For now, use DBFactory
// which returns the full ExtendedDB interface.

// DefaultDBFactory creates real TableTheory instances for production use
type DefaultDBFactory struct{}

// CreateDB creates a real TableTheory database connection
func (f *DefaultDBFactory) CreateDB(config session.Config) (core.ExtendedDB, error) {
	_ = config
	// In the real implementation, this would call tabletheory.New(config)
	// which returns an ExtendedDB instance
	// For now, we'll return a placeholder
	return nil, nil
}

// MockDBFactory creates mock TableTheory instances for testing
type MockDBFactory struct {
	// MockDB is the mock database instance that will be returned
	MockDB core.ExtendedDB

	// Error can be set to simulate connection failures
	Error error

	// OnCreateDB is called when CreateDB is invoked, useful for assertions
	OnCreateDB func(config session.Config)
}

// CreateDB returns the configured mock database or error
func (f *MockDBFactory) CreateDB(config session.Config) (core.ExtendedDB, error) {
	if f.OnCreateDB != nil {
		f.OnCreateDB(config)
	}

	if f.Error != nil {
		return nil, f.Error
	}

	return f.MockDB, nil
}

// NewMockDBFactory creates a new MockDBFactory with a default MockExtendedDB
func NewMockDBFactory() *MockDBFactory {
	return &MockDBFactory{
		MockDB: mocks.NewMockExtendedDB(),
	}
}

// WithMockDB sets a specific mock database instance
func (f *MockDBFactory) WithMockDB(mockDB core.ExtendedDB) *MockDBFactory {
	f.MockDB = mockDB
	return f
}

// WithError configures the factory to return an error
func (f *MockDBFactory) WithError(err error) *MockDBFactory {
	f.Error = err
	return f
}

// FactoryConfig provides configuration options for database factories
type FactoryConfig struct {
	Middleware    []Middleware
	EnableLogging bool
	EnableMetrics bool
}

// Middleware represents a database operation middleware
type Middleware func(next Operation) Operation

// Operation represents a database operation that can be intercepted
type Operation func() error

// ConfigurableDBFactory is a factory that supports additional configuration
type ConfigurableDBFactory interface {
	DBFactory

	// WithConfig applies configuration to the factory
	WithConfig(config FactoryConfig) ConfigurableDBFactory
}

// ConfigurableMockDBFactory extends MockDBFactory with configuration support
type ConfigurableMockDBFactory struct {
	*MockDBFactory
	config FactoryConfig
}

// NewConfigurableMockDBFactory creates a new configurable mock factory
func NewConfigurableMockDBFactory() *ConfigurableMockDBFactory {
	return &ConfigurableMockDBFactory{
		MockDBFactory: NewMockDBFactory(),
		config:        FactoryConfig{},
	}
}

// WithConfig applies configuration to the factory
func (f *ConfigurableMockDBFactory) WithConfig(config FactoryConfig) ConfigurableDBFactory {
	f.config = config
	return f
}

// CreateDB creates a mock database with applied configuration
func (f *ConfigurableMockDBFactory) CreateDB(config session.Config) (core.ExtendedDB, error) {
	// Apply any configuration-specific behavior here
	if f.config.EnableLogging {
		if f.OnCreateDB == nil {
			f.OnCreateDB = func(_ session.Config) {
				// Placeholder hook for logging; callers can replace this function to capture config.
			}
		}
	}

	return f.MockDBFactory.CreateDB(config)
}

// TestDBFactory is a specialized factory for testing that tracks all created instances
type TestDBFactory struct {
	CreateFunc func(config session.Config) (core.ExtendedDB, error)
	Instances  []core.ExtendedDB
}

// CreateDB creates a database instance and tracks it
func (f *TestDBFactory) CreateDB(config session.Config) (core.ExtendedDB, error) {
	if f.CreateFunc != nil {
		db, err := f.CreateFunc(config)
		if err == nil && db != nil {
			f.Instances = append(f.Instances, db)
		}
		return db, err
	}

	// Default behavior: create a mock
	mockDB := mocks.NewMockExtendedDB()
	f.Instances = append(f.Instances, mockDB)
	return mockDB, nil
}

// Reset clears all tracked instances
func (f *TestDBFactory) Reset() {
	f.Instances = nil
}

// GetLastInstance returns the most recently created instance
func (f *TestDBFactory) GetLastInstance() core.ExtendedDB {
	if len(f.Instances) == 0 {
		return nil
	}
	return f.Instances[len(f.Instances)-1]
}

// SimpleMockFactory provides a simple way to create mock factories for testing
func SimpleMockFactory(setupFunc func(db *mocks.MockExtendedDB)) DBFactory {
	mockDB := mocks.NewMockExtendedDB()
	if setupFunc != nil {
		setupFunc(mockDB)
	}
	return &MockDBFactory{MockDB: mockDB}
}
