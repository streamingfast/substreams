package webhook

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestNewWebhookPayload(t *testing.T) {
	// Test data
	moduleName := "test_module"
	blockNum := uint64(12345)
	timestamp := timestamppb.New(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC))
	msgType := "type.googleapis.com/sf.substreams.ethereum.v1.Events"
	data := json.RawMessage(`{"field1":"value1","field2":42}`)

	// Create payload
	payload, err := NewWebhookPayload(moduleName, blockNum, timestamp, msgType, data)
	require.NoError(t, err)
	require.NotNil(t, payload)

	// Verify fields
	assert.Equal(t, moduleName, payload.Module)
	assert.Equal(t, blockNum, payload.Block)
	assert.Equal(t, "2023-01-01T00:00:00Z", payload.Timestamp)
	assert.Equal(t, msgType, payload.Type)
	assert.Equal(t, data, payload.Payload)
}

func TestNewWebhookPayload_NilTimestamp(t *testing.T) {
	// Test data
	moduleName := "test_module"
	blockNum := uint64(12345)
	msgType := "type.googleapis.com/sf.substreams.ethereum.v1.Events"
	data := json.RawMessage(`{"test":"data"}`)

	// Create payload with nil timestamp
	payload, err := NewWebhookPayload(moduleName, blockNum, nil, msgType, data)
	require.NoError(t, err)
	require.NotNil(t, payload)

	// Verify timestamp is empty string
	assert.Equal(t, "", payload.Timestamp)
	assert.Equal(t, moduleName, payload.Module)
	assert.Equal(t, blockNum, payload.Block)
	assert.Equal(t, msgType, payload.Type)
	assert.Equal(t, data, payload.Payload)
}

func TestWebhookPayload_ToJSON(t *testing.T) {
	// Test data
	moduleName := "filtered_events"
	blockNum := uint64(22926774)
	timestamp := timestamppb.New(time.Date(2023, 1, 1, 12, 30, 45, 0, time.UTC))
	msgType := "type.googleapis.com/sf.substreams.ethereum.v1.Events"
	data := json.RawMessage(`{"events":[{"address":"0x123","data":"0xabc"}]}`)

	// Create payload
	payload, err := NewWebhookPayload(moduleName, blockNum, timestamp, msgType, data)
	require.NoError(t, err)

	// Convert to JSON
	jsonBytes, err := payload.ToJSON()
	require.NoError(t, err)

	// Parse back to verify structure
	var result map[string]interface{}
	err = json.Unmarshal(jsonBytes, &result)
	require.NoError(t, err)

	// Verify JSON structure
	assert.Equal(t, moduleName, result["module"])
	assert.Equal(t, float64(blockNum), result["block"]) // JSON numbers are float64
	assert.Equal(t, "2023-01-01T12:30:45Z", result["timestamp"])
	assert.Equal(t, msgType, result["type"])

	// Verify payload is nested correctly
	payloadData, ok := result["payload"].(map[string]interface{})
	require.True(t, ok)
	events, ok := payloadData["events"].([]interface{})
	require.True(t, ok)
	assert.Len(t, events, 1)
}

func TestWebhookPayload_JSONFormat(t *testing.T) {
	// Test that the JSON format matches the expected structure
	moduleName := "test_module"
	blockNum := uint64(12345)
	timestamp := timestamppb.New(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC))
	msgType := "type.googleapis.com/sf.substreams.ethereum.v1.Events"
	data := json.RawMessage(`{"test":"data"}`)

	payload, err := NewWebhookPayload(moduleName, blockNum, timestamp, msgType, data)
	require.NoError(t, err)

	jsonBytes, err := payload.ToJSON()
	require.NoError(t, err)

	expectedJSON := `{"module":"test_module","block":12345,"timestamp":"2023-01-01T00:00:00Z","type":"type.googleapis.com/sf.substreams.ethereum.v1.Events","payload":{"test":"data"}}`

	// Compare JSON strings (order might vary, so we parse and compare)
	var expected, actual map[string]interface{}
	err = json.Unmarshal([]byte(expectedJSON), &expected)
	require.NoError(t, err)

	err = json.Unmarshal(jsonBytes, &actual)
	require.NoError(t, err)

	assert.Equal(t, expected, actual)
}

func TestWebhookPayload_EmptyData(t *testing.T) {
	// Test with empty data
	moduleName := "empty_module"
	blockNum := uint64(0)
	timestamp := timestamppb.New(time.Now())
	msgType := "type.googleapis.com/empty.Type"
	data := json.RawMessage(`{}`)

	payload, err := NewWebhookPayload(moduleName, blockNum, timestamp, msgType, data)
	require.NoError(t, err)

	jsonBytes, err := payload.ToJSON()
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(jsonBytes, &result)
	require.NoError(t, err)

	assert.Equal(t, moduleName, result["module"])
	assert.Equal(t, float64(0), result["block"])
	assert.Equal(t, msgType, result["type"])
	assert.NotEmpty(t, result["timestamp"])

	// Verify payload is empty object
	payloadData, ok := result["payload"].(map[string]interface{})
	require.True(t, ok)
	assert.Empty(t, payloadData)
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
			timestamp := timestamppb.New(tc.time)
			payload, err := NewWebhookPayload("test", 123, timestamp, "test.Type", json.RawMessage(`{}`))
			require.NoError(t, err)

			assert.Equal(t, tc.expected, payload.Timestamp)
		})
	}
}

func TestWebhookPayload_JSONTags(t *testing.T) {
	// Test that struct tags are correctly applied
	payload := &WebhookPayload{
		Module:    "test_module",
		Block:     12345,
		Timestamp: "2023-01-01T00:00:00Z",
		Type:      "test.Type",
		Payload:   json.RawMessage(`{"test":"data"}`),
	}

	jsonBytes, err := payload.ToJSON()
	require.NoError(t, err)

	// Verify the JSON contains the expected field names (not struct field names)
	jsonStr := string(jsonBytes)
	assert.Contains(t, jsonStr, `"module":`)
	assert.Contains(t, jsonStr, `"block":`)
	assert.Contains(t, jsonStr, `"timestamp":`)
	assert.Contains(t, jsonStr, `"type":`)
	assert.Contains(t, jsonStr, `"payload":`)

	// Verify it doesn't contain struct field names with wrong casing
	assert.NotContains(t, jsonStr, `"Module":`)
	assert.NotContains(t, jsonStr, `"Block":`)
	assert.NotContains(t, jsonStr, `"Timestamp":`)
	assert.NotContains(t, jsonStr, `"Type":`)
	assert.NotContains(t, jsonStr, `"Payload":`)
}

func TestWebhookPayload_EndToEndExample(t *testing.T) {
	// Example showing the transition from old format to new format
	// This demonstrates what the webhook will actually send

	// Simulate real substreams data
	moduleName := "filtered_events"
	blockNum := uint64(22926774)
	timestamp := timestamppb.New(time.Date(2023, 12, 15, 10, 30, 45, 0, time.UTC))
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
	payload, err := NewWebhookPayload(moduleName, blockNum, timestamp, msgType, eventData)
	require.NoError(t, err)

	jsonBytes, err := payload.ToJSON()
	require.NoError(t, err)

	// Verify the new format matches expected structure
	var result map[string]interface{}
	err = json.Unmarshal(jsonBytes, &result)
	require.NoError(t, err)

	// Verify all expected fields are present with correct values
	assert.Equal(t, "filtered_events", result["module"])
	assert.Equal(t, float64(22926774), result["block"])
	assert.Equal(t, "2023-12-15T10:30:45Z", result["timestamp"])
	assert.Equal(t, "type.googleapis.com/sf.substreams.ethereum.v1.Events", result["type"])

	// Verify payload structure
	payloadData, ok := result["payload"].(map[string]interface{})
	require.True(t, ok, "payload should be a JSON object")

	events, ok := payloadData["events"].([]interface{})
	require.True(t, ok, "events should be an array")
	require.Len(t, events, 1, "should have one event")

	event := events[0].(map[string]interface{})
	assert.Equal(t, "0x1234567890abcdef", event["address"])

	// Print example for documentation
	t.Logf("New webhook payload format:\n%s", string(jsonBytes))

	// Verify the payload does NOT contain old format fields
	jsonStr := string(jsonBytes)
	assert.NotContains(t, jsonStr, `"@module"`)
	assert.NotContains(t, jsonStr, `"@block"`)
	assert.NotContains(t, jsonStr, `"@type"`)
	assert.NotContains(t, jsonStr, `"@data"`)
}
