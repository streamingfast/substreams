package pbsubstreamsrpcv3

import (
	"os"
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func TestRequest_Validate_EstimatedMode(t *testing.T) {
	tests := []struct {
		name        string
		request     *Request
		envVar      string
		expectError bool
		errorMsg    string
	}{
		{
			name: "estimated_mode with noop_mode should fail",
			request: &Request{
				Package: &pbsubstreams.Package{
					Modules: &pbsubstreams.Modules{
						Modules: []*pbsubstreams.Module{
							{
								Name: "test_module",
								Kind: &pbsubstreams.Module_KindMap_{},
							},
						},
					},
				},
				OutputModule:  "test_module",
				EstimatedMode: true,
				NoopMode:      true,
			},
			expectError: true,
			errorMsg:    "estimated_mode and noop_mode are mutually exclusive",
		},
		{
			name: "estimated_mode with valid block range should pass",
			request: &Request{
				Package: &pbsubstreams.Package{
					Modules: &pbsubstreams.Modules{
						Modules: []*pbsubstreams.Module{
							{
								Name: "test_module",
								Kind: &pbsubstreams.Module_KindMap_{},
							},
						},
					},
				},
				OutputModule:   "test_module",
				EstimatedMode:  true,
				StartBlockNum:  100,
				StopBlockNum:   500, // Range of 400 blocks
			},
			expectError: false,
		},
		{
			name: "estimated_mode with excessive block range should fail",
			request: &Request{
				Package: &pbsubstreams.Package{
					Modules: &pbsubstreams.Modules{
						Modules: []*pbsubstreams.Module{
							{
								Name: "test_module",
								Kind: &pbsubstreams.Module_KindMap_{},
							},
						},
					},
				},
				OutputModule:   "test_module",
				EstimatedMode:  true,
				StartBlockNum:  100,
				StopBlockNum:   2000, // Range of 1900 blocks (exceeds default 1000)
			},
			expectError: true,
			errorMsg:    "estimated_mode block range (1900) exceeds maximum allowed range (1000)",
		},
		{
			name: "estimated_mode with custom env var limit should pass",
			request: &Request{
				Package: &pbsubstreams.Package{
					Modules: &pbsubstreams.Modules{
						Modules: []*pbsubstreams.Module{
							{
								Name: "test_module",
								Kind: &pbsubstreams.Module_KindMap_{},
							},
						},
					},
				},
				OutputModule:   "test_module",
				EstimatedMode:  true,
				StartBlockNum:  100,
				StopBlockNum:   2500, // Range of 2400 blocks
			},
			envVar:      "3000", // Set limit to 3000
			expectError: false,
		},
		{
			name: "estimated_mode with custom env var limit should fail",
			request: &Request{
				Package: &pbsubstreams.Package{
					Modules: &pbsubstreams.Modules{
						Modules: []*pbsubstreams.Module{
							{
								Name: "test_module",
								Kind: &pbsubstreams.Module_KindMap_{},
							},
						},
					},
				},
				OutputModule:   "test_module",
				EstimatedMode:  true,
				StartBlockNum:  100,
				StopBlockNum:   2600, // Range of 2500 blocks (exceeds custom 2000)
			},
			envVar:      "2000", // Set limit to 2000
			expectError: true,
			errorMsg:    "estimated_mode block range (2500) exceeds maximum allowed range (2000)",
		},
		{
			name: "estimated_mode without stop_block_num should pass",
			request: &Request{
				Package: &pbsubstreams.Package{
					Modules: &pbsubstreams.Modules{
						Modules: []*pbsubstreams.Module{
							{
								Name: "test_module",
								Kind: &pbsubstreams.Module_KindMap_{},
							},
						},
					},
				},
				OutputModule:   "test_module",
				EstimatedMode:  true,
				StartBlockNum:  100,
				StopBlockNum:   0, // No stop block specified
			},
			expectError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Set environment variable if specified
			if test.envVar != "" {
				os.Setenv("SUBSTREAMS_ESTIMATE_MODE_MAX_BLOCK_RANGE", test.envVar)
				defer os.Unsetenv("SUBSTREAMS_ESTIMATE_MODE_MAX_BLOCK_RANGE")
			} else {
				// Ensure env var is not set for default behavior
				os.Unsetenv("SUBSTREAMS_ESTIMATE_MODE_MAX_BLOCK_RANGE")
			}

			err := test.request.Validate()

			if test.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}
				if test.errorMsg != "" && err.Error() != test.errorMsg {
					t.Errorf("Expected error message %q, got %q", test.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestGetEstimateModeMaxBlockRange(t *testing.T) {
	tests := []struct {
		name     string
		envVar   string
		expected uint64
	}{
		{
			name:     "default value when env var not set",
			envVar:   "",
			expected: 1000,
		},
		{
			name:     "custom value from env var",
			envVar:   "5000",
			expected: 5000,
		},
		{
			name:     "invalid env var should return default",
			envVar:   "invalid",
			expected: 1000,
		},
		{
			name:     "zero value from env var",
			envVar:   "0",
			expected: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.envVar != "" {
				os.Setenv("SUBSTREAMS_ESTIMATE_MODE_MAX_BLOCK_RANGE", test.envVar)
				defer os.Unsetenv("SUBSTREAMS_ESTIMATE_MODE_MAX_BLOCK_RANGE")
			} else {
				os.Unsetenv("SUBSTREAMS_ESTIMATE_MODE_MAX_BLOCK_RANGE")
			}

			result := getEstimateModeMaxBlockRange()
			if result != test.expected {
				t.Errorf("Expected %d, got %d", test.expected, result)
			}
		})
	}
}
