package strategies

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewStrategy(t *testing.T) {
	tests := []struct {
		name       string
		config     StrategyConfig
		wantType   reflect.Type
		wantErr    bool
		wantErrStr string
	}{
		{
			name: "Create Fixed Window Strategy",
			config: StrategyConfig{
				Type:           "fixed_window",
				WindowDuration: time.Minute,
			},
			wantType: reflect.TypeOf(&FixedWindowStrategy{}),
			wantErr:  false,
		},
		{
			name: "Create Token Bucket Strategy",
			config: StrategyConfig{
				Type:         "token_bucket",
				BucketSize:   10,
				RefillRate:   time.Second,
				RefillAmount: 1,
			},
			wantType: reflect.TypeOf(&TokenBucketStrategy{}),
			wantErr:  false,
		},
		{
			name: "Unsupported Strategy Type",
			config: StrategyConfig{
				Type: "sliding_logs",
			},
			wantType:   nil,
			wantErr:    true,
			wantErrStr: "unsupported strategy type: sliding_logs",
		},
		{
			name: "Invalid Fixed Window Config",
			config: StrategyConfig{
				Type: "fixed_window",
			},
			wantType:   nil,
			wantErr:    true,
			wantErrStr: "fixed window window_duration must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewStrategy(tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
				if tt.wantErrStr != "" {
					assert.EqualError(t, err, tt.wantErrStr)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, got)
				assert.Equal(t, tt.wantType, reflect.TypeOf(got))
			}
		})
	}
}

func TestValidateStrategyConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  StrategyConfig
		wantErr bool
	}{
		{name: "Valid Fixed Window", config: StrategyConfig{Type: "fixed_window", WindowDuration: time.Second}, wantErr: false},
		{name: "Invalid Fixed Window", config: StrategyConfig{Type: "fixed_window", WindowDuration: 0}, wantErr: true},
		{name: "Valid Token Bucket", config: StrategyConfig{Type: "token_bucket", RefillRate: time.Second, RefillAmount: 1}, wantErr: false},
		{name: "Invalid Token Bucket (Rate)", config: StrategyConfig{Type: "token_bucket", RefillRate: -1}, wantErr: true},
		{name: "Invalid Token Bucket (Amount)", config: StrategyConfig{Type: "token_bucket", RefillAmount: -1}, wantErr: true},
		{name: "Unsupported Type", config: StrategyConfig{Type: "unknown"}, wantErr: true},
		{name: "Empty Type", config: StrategyConfig{Type: ""}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStrategyConfig(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetSupportedStrategies(t *testing.T) {
	t.Run("Get List of Supported Strategies", func(t *testing.T) {
		want := []string{"token_bucket", "fixed_window"}
		got := GetSupportedStrategies()
		assert.ElementsMatch(t, want, got)
	})
}

func TestGetRecommendedStrategy(t *testing.T) {
	tests := []struct {
		name              string
		requestsPerMinute int64
		burstSize         int64
		useCase           string
		wantType          string
	}{
		{name: "AI use case", requestsPerMinute: 100, burstSize: 20, useCase: "ai", wantType: "token_bucket"},
		{name: "General API use case", requestsPerMinute: 1000, burstSize: 100, useCase: "api", wantType: "token_bucket"},
		{name: "Default use case", requestsPerMinute: 60, burstSize: 10, useCase: "other", wantType: "token_bucket"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetRecommendedStrategy(tt.requestsPerMinute, tt.burstSize, tt.useCase)
			assert.Equal(t, tt.wantType, got.Type)
			assert.Equal(t, tt.burstSize, got.BucketSize)
			assert.Greater(t, got.RefillRate, time.Duration(0))
			assert.Greater(t, got.RefillAmount, int64(0))
		})
	}
}
