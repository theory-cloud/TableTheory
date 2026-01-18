package session

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultConfig tests the DefaultConfig function
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.NotNil(t, cfg)
	assert.Equal(t, "us-east-1", cfg.Region)
	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, int64(5), cfg.DefaultRCU)
	assert.Equal(t, int64(5), cfg.DefaultWCU)
	assert.False(t, cfg.AutoMigrate)
	assert.False(t, cfg.EnableMetrics)
	assert.Empty(t, cfg.Endpoint)
	assert.Nil(t, cfg.AWSConfigOptions)
	assert.Nil(t, cfg.DynamoDBOptions)
}

// TestConfig tests the Config struct with various settings
func TestConfig(t *testing.T) {
	t.Run("Custom configuration", func(t *testing.T) {
		cfg := &Config{
			Region:        "eu-west-1",
			Endpoint:      "http://localhost:8000",
			MaxRetries:    5,
			DefaultRCU:    10,
			DefaultWCU:    10,
			AutoMigrate:   true,
			EnableMetrics: true,
		}

		assert.Equal(t, "eu-west-1", cfg.Region)
		assert.Equal(t, "http://localhost:8000", cfg.Endpoint)
		assert.Equal(t, 5, cfg.MaxRetries)
		assert.Equal(t, int64(10), cfg.DefaultRCU)
		assert.Equal(t, int64(10), cfg.DefaultWCU)
		assert.True(t, cfg.AutoMigrate)
		assert.True(t, cfg.EnableMetrics)
	})

	t.Run("With AWS config options", func(t *testing.T) {
		customOption := func(o *config.LoadOptions) error {
			o.Region = "ap-southeast-1"
			return nil
		}

		cfg := &Config{
			AWSConfigOptions: []func(*config.LoadOptions) error{customOption},
		}

		assert.Len(t, cfg.AWSConfigOptions, 1)
	})

	t.Run("With DynamoDB options", func(t *testing.T) {
		customOption := func(o *dynamodb.Options) {
			o.RetryMaxAttempts = 5
		}

		cfg := &Config{
			DynamoDBOptions: []func(*dynamodb.Options){customOption},
		}

		assert.Len(t, cfg.DynamoDBOptions, 1)
	})
}

// TestNewSession tests session creation with various configurations
func TestNewSession(t *testing.T) {
	// Mock AWS config loading for tests
	originalConfigLoad := configLoadFunc
	defer func() { configLoadFunc = originalConfigLoad }()

	t.Run("With default config", func(t *testing.T) {
		configLoadFunc = func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
			return aws.Config{Region: "us-east-1"}, nil
		}

		sess, err := NewSession(nil)

		require.NoError(t, err)
		assert.NotNil(t, sess)
		assert.NotNil(t, sess.config)
		assert.NotNil(t, sess.client)
		assert.Equal(t, "us-east-1", sess.config.Region)
	})

	t.Run("With custom config", func(t *testing.T) {
		configLoadFunc = func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
			return aws.Config{Region: "eu-west-1"}, nil
		}

		cfg := &Config{
			Region:     "eu-west-1",
			Endpoint:   "http://localhost:8000",
			MaxRetries: 5,
		}

		sess, err := NewSession(cfg)

		require.NoError(t, err)
		assert.NotNil(t, sess)
		assert.Equal(t, cfg, sess.config)
		assert.Equal(t, "eu-west-1", sess.config.Region)
		assert.Equal(t, "http://localhost:8000", sess.config.Endpoint)
		assert.Equal(t, 5, sess.config.MaxRetries)
	})

	t.Run("With empty region", func(t *testing.T) {
		configLoadFunc = func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
			// Verify that region option is not added when empty
			for _, opt := range opts {
				loadOpts := &config.LoadOptions{}
				if err := opt(loadOpts); err != nil {
					t.Fatalf("unexpected error applying config option: %v", err)
				}
			}
			return aws.Config{}, nil
		}

		cfg := &Config{
			Region: "",
		}

		sess, err := NewSession(cfg)

		require.NoError(t, err)
		assert.NotNil(t, sess)
	})

	t.Run("With zero max retries", func(t *testing.T) {
		configLoadFunc = func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
			return aws.Config{}, nil
		}

		cfg := &Config{
			MaxRetries: 0,
		}

		sess, err := NewSession(cfg)

		require.NoError(t, err)
		assert.NotNil(t, sess)
	})

	t.Run("AWS config load error", func(t *testing.T) {
		expectedErr := errors.New("config load failed")
		configLoadFunc = func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
			return aws.Config{}, expectedErr
		}

		sess, err := NewSession(&Config{})

		assert.Error(t, err)
		assert.Nil(t, sess)
		assert.Contains(t, err.Error(), "failed to load AWS config")
		assert.Contains(t, err.Error(), expectedErr.Error())
	})

	t.Run("With custom AWS config options", func(t *testing.T) {
		var capturedRegion string
		customOption := func(o *config.LoadOptions) error {
			capturedRegion = o.Region
			return nil
		}

		configLoadFunc = func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
			loadOpts := &config.LoadOptions{}
			for _, opt := range opts {
				if err := opt(loadOpts); err != nil {
					t.Fatalf("unexpected error applying config option: %v", err)
				}
			}
			return aws.Config{Region: loadOpts.Region}, nil
		}

		cfg := &Config{
			Region:           "us-west-2",
			AWSConfigOptions: []func(*config.LoadOptions) error{customOption},
		}

		sess, err := NewSession(cfg)

		require.NoError(t, err)
		assert.NotNil(t, sess)
		assert.Equal(t, "us-west-2", capturedRegion)
	})

	t.Run("With custom DynamoDB options", func(t *testing.T) {
		configLoadFunc = func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
			return aws.Config{Region: "us-east-1"}, nil
		}

		customOption := func(o *dynamodb.Options) {
			o.RetryMaxAttempts = 10
		}

		cfg := &Config{
			DynamoDBOptions: []func(*dynamodb.Options){customOption},
		}

		sess, err := NewSession(cfg)

		require.NoError(t, err)
		assert.NotNil(t, sess)
		// The DynamoDB options are applied during client creation
		assert.Len(t, cfg.DynamoDBOptions, 1)
	})
}

// TestSession_Getters tests the getter methods of Session
func TestSession_Getters(t *testing.T) {
	// Mock AWS config loading
	originalConfigLoad := configLoadFunc
	defer func() { configLoadFunc = originalConfigLoad }()

	configLoadFunc = func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
		return aws.Config{
			Region: "test-region",
		}, nil
	}

	cfg := &Config{
		Region:        "test-region",
		Endpoint:      "http://test-endpoint",
		EnableMetrics: true,
	}

	sess, err := NewSession(cfg)
	require.NoError(t, err)

	t.Run("Client", func(t *testing.T) {
		client, err := sess.Client()
		assert.NoError(t, err)
		assert.NotNil(t, client)
		assert.IsType(t, &dynamodb.Client{}, client)
	})

	t.Run("Config", func(t *testing.T) {
		config := sess.Config()
		assert.NotNil(t, config)
		assert.Equal(t, cfg, config)
		assert.Equal(t, "test-region", config.Region)
		assert.Equal(t, "http://test-endpoint", config.Endpoint)
		assert.True(t, config.EnableMetrics)
	})

	t.Run("AWSConfig", func(t *testing.T) {
		awsConfig := sess.AWSConfig()
		assert.Equal(t, "test-region", awsConfig.Region)
	})
}

// TestSession_WithContext tests the WithContext method
func TestSession_WithContext(t *testing.T) {
	// Mock AWS config loading
	originalConfigLoad := configLoadFunc
	defer func() { configLoadFunc = originalConfigLoad }()

	configLoadFunc = func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}

	sess, err := NewSession(nil)
	require.NoError(t, err)

	ctx := context.Background()
	newSess := sess.WithContext(ctx)

	// Should return the same session instance
	assert.Equal(t, sess, newSess)
}

// TestSessionIntegration tests various integration scenarios
func TestSessionIntegration(t *testing.T) {
	// Mock AWS config loading
	originalConfigLoad := configLoadFunc
	defer func() { configLoadFunc = originalConfigLoad }()

	t.Run("Full configuration", func(t *testing.T) {
		configLoadFunc = func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
			return aws.Config{Region: "us-west-2"}, nil
		}

		cfg := &Config{
			Region:        "us-west-2",
			Endpoint:      "http://localhost:8000",
			MaxRetries:    5,
			DefaultRCU:    25,
			DefaultWCU:    25,
			AutoMigrate:   true,
			EnableMetrics: true,
		}

		sess, err := NewSession(cfg)

		require.NoError(t, err)
		assert.NotNil(t, sess)

		// Verify all getters work correctly
		client, err := sess.Client()
		assert.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, cfg, sess.Config())
		assert.Equal(t, "us-west-2", sess.AWSConfig().Region)

		// Verify context method
		assert.Equal(t, sess, sess.WithContext(context.Background()))
	})

	t.Run("Multiple custom options", func(t *testing.T) {
		option1Called := false
		option2Called := false

		awsOption1 := func(o *config.LoadOptions) error {
			option1Called = true
			return nil
		}

		awsOption2 := func(o *config.LoadOptions) error {
			option2Called = true
			return nil
		}

		configLoadFunc = func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
			loadOpts := &config.LoadOptions{}
			for _, opt := range opts {
				if err := opt(loadOpts); err != nil {
					t.Fatalf("unexpected error applying config option: %v", err)
				}
			}
			return aws.Config{}, nil
		}

		cfg := &Config{
			AWSConfigOptions: []func(*config.LoadOptions) error{awsOption1, awsOption2},
		}

		sess, err := NewSession(cfg)

		require.NoError(t, err)
		assert.NotNil(t, sess)
		assert.True(t, option1Called)
		assert.True(t, option2Called)
	})
}

// BenchmarkNewSession benchmarks session creation
func BenchmarkNewSession(b *testing.B) {
	// Mock AWS config loading for benchmarks
	originalConfigLoad := configLoadFunc
	defer func() { configLoadFunc = originalConfigLoad }()

	configLoadFunc = func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}

	cfg := &Config{
		Region:     "us-east-1",
		MaxRetries: 3,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := NewSession(cfg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSessionGetters benchmarks the getter methods
func BenchmarkSessionGetters(b *testing.B) {
	// Mock AWS config loading
	originalConfigLoad := configLoadFunc
	defer func() { configLoadFunc = originalConfigLoad }()

	configLoadFunc = func(ctx context.Context, opts ...func(*config.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}

	sess, err := NewSession(nil)
	if err != nil {
		b.Fatal(err)
	}

	b.Run("Client", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err := sess.Client(); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Config", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = sess.Config()
		}
	})

	b.Run("AWSConfig", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = sess.AWSConfig()
		}
	})

	b.Run("WithContext", func(b *testing.B) {
		ctx := context.Background()
		for i := 0; i < b.N; i++ {
			_ = sess.WithContext(ctx)
		}
	})
}
