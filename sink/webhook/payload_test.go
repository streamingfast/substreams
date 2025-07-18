package webhook

import (
	"encoding/json"
	"testing"
	"time"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestNewWebhookPayload(t *testing.T) {
	// Test data
	moduleName := "test_module"
	timestamp := timestamppb.New(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC))
	clock := &pbsubstreams.Clock{
		Id:        "f843bc26cea0cbd50b09699546a8a97de6a1727646c17a857c5d8d868fc26142",
		Number:    53448530,
		Timestamp: timestamp,
	}
	msgType := "type.googleapis.com/sf.substreams.ethereum.v1.Events"
	data := json.RawMessage(`{"field1":"value1","field2":42}`)

	// Create payload
	payload, err := NewWebhookPayload(moduleName, clock, msgType, data)
	require.NoError(t, err)
	require.NotNil(t, payload)

	// Verify clock fields
	assert.Equal(t, uint64(53448530), payload.Clock.Number)
	assert.Equal(t, "f843bc26cea0cbd50b09699546a8a97de6a1727646c17a857c5d8d868fc26142", payload.Clock.ID)
	assert.Equal(t, "2023-01-01T00:00:00Z", payload.Clock.Timestamp)

	// Verify manifest fields
	assert.Equal(t, moduleName, payload.Manifest.ModuleName)
	assert.Equal(t, "sf.substreams.ethereum.v1.Events", payload.Manifest.Type) // Type prefix should be stripped

	// Verify data
	assert.Equal(t, data, payload.Data)
}

func TestNewWebhookPayload_NilClock(t *testing.T) {
	// Test data
	moduleName := "test_module"
	msgType := "type.googleapis.com/sf.substreams.ethereum.v1.Events"
	data := json.RawMessage(`{"test":"data"}`)

	// Create payload with nil clock
	payload, err := NewWebhookPayload(moduleName, nil, msgType, data)
	require.NoError(t, err)
	require.NotNil(t, payload)

	// Verify clock fields are defaults
	assert.Equal(t, uint64(0), payload.Clock.Number)
	assert.Equal(t, "", payload.Clock.ID)
	assert.Equal(t, "", payload.Clock.Timestamp)

	// Verify manifest fields
	assert.Equal(t, moduleName, payload.Manifest.ModuleName)
	assert.Equal(t, "sf.substreams.ethereum.v1.Events", payload.Manifest.Type)

	// Verify data
	assert.Equal(t, data, payload.Data)
}

func TestNewWebhookPayload_TypePrefixStripping(t *testing.T) {
	testCases := []struct {
		name         string
		inputType    string
		expectedType string
	}{
		{
			name:         "with type.googleapis.com prefix",
			inputType:    "type.googleapis.com/sf.substreams.ethereum.v1.Events",
			expectedType: "sf.substreams.ethereum.v1.Events",
		},
		{
			name:         "without prefix",
			inputType:    "sf.substreams.ethereum.v1.Events",
			expectedType: "sf.substreams.ethereum.v1.Events",
		},
		{
			name:         "custom type with prefix",
			inputType:    "type.googleapis.com/custom.v1.MyType",
			expectedType: "custom.v1.MyType",
		},
		{
			name:         "simple type",
			inputType:    "MyType",
			expectedType: "MyType",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := NewWebhookPayload("test_module", nil, tc.inputType, json.RawMessage(`{}`))
			require.NoError(t, err)
			assert.Equal(t, tc.expectedType, payload.Manifest.Type)
		})
	}
}

func TestWebhookPayload_ToJSON(t *testing.T) {
	// Test data
	moduleName := "filtered_events"
	timestamp := timestamppb.New(time.Date(2024, 2, 12, 22, 23, 51, 0, time.UTC))
	clock := &pbsubstreams.Clock{
		Id:        "f843bc26cea0cbd50b09699546a8a97de6a1727646c17a857c5d8d868fc26142",
		Number:    53448530,
		Timestamp: timestamp,
	}
	msgType := "type.googleapis.com/sf.substreams.ethereum.v1.Events"
	data := json.RawMessage(`{"events":[{"address":"0x123","data":"0xabc"}]}`)

	// Create payload
	payload, err := NewWebhookPayload(moduleName, clock, msgType, data)
	require.NoError(t, err)

	// Convert to JSON
	jsonBytes, err := payload.ToJSON()
	require.NoError(t, err)

	// Parse back to verify structure
	var result map[string]interface{}
	err = json.Unmarshal(jsonBytes, &result)
	require.NoError(t, err)

	// Verify top-level structure
	assert.Contains(t, result, "clock")
	assert.Contains(t, result, "manifest")
	assert.Contains(t, result, "data")

	// Verify clock structure
	clockData, ok := result["clock"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "2024-02-12T22:23:51Z", clockData["timestamp"])
	assert.Equal(t, float64(53448530), clockData["number"]) // JSON numbers are float64
	assert.Equal(t, "f843bc26cea0cbd50b09699546a8a97de6a1727646c17a857c5d8d868fc26142", clockData["id"])

	// Verify manifest structure
	manifestData, ok := result["manifest"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, moduleName, manifestData["moduleName"])
	assert.Equal(t, "sf.substreams.ethereum.v1.Events", manifestData["type"])

	// Verify data is nested correctly
	dataResult, ok := result["data"].(map[string]interface{})
	require.True(t, ok)
	events, ok := dataResult["events"].([]interface{})
	require.True(t, ok)
	assert.Len(t, events, 1)
}

func TestWebhookPayload_JSONFormat(t *testing.T) {
	// Test that the JSON format matches the expected structure exactly
	moduleName := "module_name"
	timestamp := timestamppb.New(time.Date(2024, 2, 12, 22, 23, 51, 0, time.UTC))
	clock := &pbsubstreams.Clock{
		Id:        "f843bc26cea0cbd50b09699546a8a97de6a1727646c17a857c5d8d868fc26142",
		Number:    53448530,
		Timestamp: timestamp,
	}
	msgType := "type.googleapis.com/sf.substreams.ethereum.v1.Events"
	data := json.RawMessage(`{"test":"data"}`)

	payload, err := NewWebhookPayload(moduleName, clock, msgType, data)
	require.NoError(t, err)

	jsonBytes, err := payload.ToJSON()
	require.NoError(t, err)

	// Parse to verify structure matches expected format
	var result map[string]interface{}
	err = json.Unmarshal(jsonBytes, &result)
	require.NoError(t, err)

	// Verify the structure matches the specification
	expectedStructure := map[string]interface{}{
		"clock": map[string]interface{}{
			"timestamp": "2024-02-12T22:23:51Z",
			"number":    float64(53448530),
			"id":        "f843bc26cea0cbd50b09699546a8a97de6a1727646c17a857c5d8d868fc26142",
		},
		"manifest": map[string]interface{}{
			"moduleName": "module_name",
			"type":       "sf.substreams.ethereum.v1.Events",
		},
		"data": map[string]interface{}{
			"test": "data",
		},
	}

	assert.Equal(t, expectedStructure, result)
}

func TestWebhookPayload_EmptyData(t *testing.T) {
	// Test with empty data
	moduleName := "empty_module"
	clock := &pbsubstreams.Clock{
		Id:        "test_id",
		Number:    0,
		Timestamp: timestamppb.New(time.Now()),
	}
	msgType := "type.googleapis.com/empty.Type"
	data := json.RawMessage(`{}`)

	payload, err := NewWebhookPayload(moduleName, clock, msgType, data)
	require.NoError(t, err)

	jsonBytes, err := payload.ToJSON()
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(jsonBytes, &result)
	require.NoError(t, err)

	// Verify clock structure
	clockData, ok := result["clock"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(0), clockData["number"])
	assert.Equal(t, "test_id", clockData["id"])
	assert.NotEmpty(t, clockData["timestamp"])

	// Verify manifest structure
	manifestData, ok := result["manifest"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, moduleName, manifestData["moduleName"])
	assert.Equal(t, "empty.Type", manifestData["type"])

	// Verify data is empty object
	dataResult, ok := result["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Empty(t, dataResult)
}

func TestWebhookPayload_TimestampFormats(t *testing.T) {
	testCases := []struct {
		name     string
		time     time.Time
		expected string
	}{
		{
			name:     "UTC time",
			time:     time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			expected: "2023-01-01T00:00:00Z",
		},
		{
			name:     "time with nanoseconds",
			time:     time.Date(2023, 1, 1, 12, 30, 45, 123456789, time.UTC),
			expected: "2023-01-01T12:30:45Z",
		},
		{
			name:     "different timezone",
			time:     time.Date(2023, 6, 15, 14, 30, 0, 0, time.FixedZone("EST", -5*60*60)),
			expected: "2023-06-15T19:30:00Z",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clock := &pbsubstreams.Clock{
				Id:        "test_id",
				Number:    123,
				Timestamp: timestamppb.New(tc.time),
			}
			payload, err := NewWebhookPayload("test", clock, "test.Type", json.RawMessage(`{}`))
			require.NoError(t, err)

			assert.Equal(t, tc.expected, payload.Clock.Timestamp)
		})
	}
}

func TestWebhookPayload_JSONTags(t *testing.T) {
	// Test that struct tags are correctly applied
	clock := &pbsubstreams.Clock{
		Id:        "test_id",
		Number:    12345,
		Timestamp: timestamppb.New(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)),
	}

	payload, err := NewWebhookPayload("test_module", clock, "test.Type", json.RawMessage(`{"test":"data"}`))
	require.NoError(t, err)

	jsonBytes, err := payload.ToJSON()
	require.NoError(t, err)

	// Verify the JSON contains the expected field names
	jsonStr := string(jsonBytes)
	assert.Contains(t, jsonStr, `"clock":`)
	assert.Contains(t, jsonStr, `"manifest":`)
	assert.Contains(t, jsonStr, `"data":`)
	assert.Contains(t, jsonStr, `"timestamp":`)
	assert.Contains(t, jsonStr, `"number":`)
	assert.Contains(t, jsonStr, `"id":`)
	assert.Contains(t, jsonStr, `"moduleName":`)
	assert.Contains(t, jsonStr, `"type":`)

	// Verify it doesn't contain struct field names with wrong casing
	assert.NotContains(t, jsonStr, `"Clock":`)
	assert.NotContains(t, jsonStr, `"Manifest":`)
	assert.NotContains(t, jsonStr, `"Data":`)
}

func TestWebhookPayload_EndToEndExample(t *testing.T) {
	// Example showing the new format that matches the specification
	moduleName := "filtered_events"
	timestamp := timestamppb.New(time.Date(2024, 2, 12, 22, 23, 51, 0, time.UTC))
	clock := &pbsubstreams.Clock{
		Id:        "f843bc26cea0cbd50b09699546a8a97de6a1727646c17a857c5d8d868fc26142",
		Number:    53448530,
		Timestamp: timestamp,
	}
	msgType := "type.googleapis.com/sf.substreams.ethereum.v1.Events"

	// Sample event data that would come from substreams
	eventData := json.RawMessage(`{
		"events": [
			{
				"address": "0x1234567890abcdef",
				"topics": ["0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"],
				"data": "0x000000000000000000000000000000000000000000000001158e460913d00000"
			}
		]
	}`)

	// Create new payload format
	payload, err := NewWebhookPayload(moduleName, clock, msgType, eventData)
	require.NoError(t, err)

	jsonBytes, err := payload.ToJSON()
	require.NoError(t, err)

	// Verify the new format matches expected structure
	var result map[string]interface{}
	err = json.Unmarshal(jsonBytes, &result)
	require.NoError(t, err)

	// Verify clock structure
	clockData, ok := result["clock"].(map[string]interface{})
	require.True(t, ok, "clock should be a JSON object")
	assert.Equal(t, "2024-02-12T22:23:51Z", clockData["timestamp"])
	assert.Equal(t, float64(53448530), clockData["number"])
	assert.Equal(t, "f843bc26cea0cbd50b09699546a8a97de6a1727646c17a857c5d8d868fc26142", clockData["id"])

	// Verify manifest structure
	manifestData, ok := result["manifest"].(map[string]interface{})
	require.True(t, ok, "manifest should be a JSON object")
	assert.Equal(t, "filtered_events", manifestData["moduleName"])
	assert.Equal(t, "sf.substreams.ethereum.v1.Events", manifestData["type"])

	// Verify data structure
	dataResult, ok := result["data"].(map[string]interface{})
	require.True(t, ok, "data should be a JSON object")

	events, ok := dataResult["events"].([]interface{})
	require.True(t, ok, "events should be an array")
	require.Len(t, events, 1, "should have one event")

	event := events[0].(map[string]interface{})
	assert.Equal(t, "0x1234567890abcdef", event["address"])

	// Print example for documentation
	t.Logf("New webhook payload format:\n%s", string(jsonBytes))

	// Verify the payload does NOT contain old format fields
	jsonStr := string(jsonBytes)
	assert.NotContains(t, jsonStr, `"module":`)
	assert.NotContains(t, jsonStr, `"block":`)
	assert.NotContains(t, jsonStr, `"payload":`)

	// Parse JSON to verify structure - no top-level timestamp, module, or block fields
	var parsed map[string]interface{}
	err = json.Unmarshal(jsonBytes, &parsed)
	require.NoError(t, err)

	// Verify old format fields are not present at top level
	_, hasTopLevelTimestamp := parsed["timestamp"]
	assert.False(t, hasTopLevelTimestamp, "should not have top-level timestamp field")
	_, hasTopLevelModule := parsed["module"]
	assert.False(t, hasTopLevelModule, "should not have top-level module field")
	_, hasTopLevelBlock := parsed["block"]
	assert.False(t, hasTopLevelBlock, "should not have top-level block field")
	_, hasTopLevelPayload := parsed["payload"]
	assert.False(t, hasTopLevelPayload, "should not have top-level payload field")

	// Verify new format fields are present
	assert.Contains(t, parsed, "clock")
	assert.Contains(t, parsed, "manifest")
	assert.Contains(t, parsed, "data")
}

func TestWebhookPayload_NilTimestamp(t *testing.T) {
	// Test with clock that has nil timestamp
	clock := &pbsubstreams.Clock{
		Id:        "test_id",
		Number:    12345,
		Timestamp: nil,
	}

	payload, err := NewWebhookPayload("test_module", clock, "test.Type", json.RawMessage(`{}`))
	require.NoError(t, err)

	// Verify timestamp is empty string when nil
	assert.Equal(t, "", payload.Clock.Timestamp)
	assert.Equal(t, uint64(12345), payload.Clock.Number)
	assert.Equal(t, "test_id", payload.Clock.ID)
}
