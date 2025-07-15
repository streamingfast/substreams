# Substreams Webhook Sink Examples

This document provides examples of how to use the webhook sink with the new retry functionality.

## Basic Usage

```bash
# Basic webhook call with default settings
substreams sink webhook https://api.example.com/webhook manifest.yaml module_name

# With custom start block
substreams sink webhook https://api.example.com/webhook manifest.yaml module_name --start-block 12345
```

## Retry Configuration

The webhook sink now supports configurable retry behavior for handling transient failures:

### Default Settings
- **Max Retries**: 3 attempts
- **Timeout**: 30 seconds per request
- **Max Retry Interval**: 30 seconds (exponential backoff cap)

### Custom Retry Settings

```bash
# Increase retry attempts for unreliable endpoints
substreams sink webhook https://api.example.com/webhook manifest.yaml module_name \
  --webhook-max-retries 5

# Shorter timeout for faster failure detection
substreams sink webhook https://api.example.com/webhook manifest.yaml module_name \
  --webhook-timeout 10s

# Longer maximum retry interval for heavily loaded endpoints
substreams sink webhook https://api.example.com/webhook manifest.yaml module_name \
  --webhook-max-retry-interval 60s

# Disable retries completely
substreams sink webhook https://api.example.com/webhook manifest.yaml module_name \
  --webhook-max-retries 0
```

### Combined Configuration

```bash
# Production-ready configuration with aggressive retries
substreams sink webhook https://api.example.com/webhook manifest.yaml module_name \
  --webhook-max-retries 5 \
  --webhook-timeout 15s \
  --webhook-max-retry-interval 45s \
  --state-file ./production.cursor
```

## Retry Behavior

### What Gets Retried
- **Network errors** (connection failures, timeouts)
- **Server errors** (5xx HTTP status codes)
- **Temporary service unavailability**

### What Doesn't Get Retried
- **Client errors** (4xx HTTP status codes like 400, 401, 403, 404)
- **Request creation failures** (invalid URLs)
- **Context cancellation**

### Exponential Backoff
The retry mechanism uses exponential backoff with jitter:
- First retry: ~1 second
- Second retry: ~2 seconds  
- Third retry: ~4 seconds
- Maximum interval is capped by `--webhook-max-retry-interval`

## State Management

```bash
# Custom state file location
substreams sink webhook https://api.example.com/webhook manifest.yaml module_name \
  --state-file ./custom/path/state.cursor

# Disable state persistence (start from scratch each time)
substreams sink webhook https://api.example.com/webhook manifest.yaml module_name \
  --state-file ""
```

## Error Handling

The webhook sink continues processing even if webhook calls fail after all retries. This ensures that:
- The substreams processing doesn't get blocked by webhook failures
- Block processing continues uninterrupted
- Cursor state is still saved for successful blocks

## Monitoring and Logging

The webhook sink provides detailed logging for troubleshooting:

```
INFO calling webhook block=12345 url=https://api.example.com/webhook
WARN webhook call failed for block 12345: webhook returned server error status 503 for block 12345
INFO calling webhook block=12345 url=https://api.example.com/webhook (retry 1/3)
INFO webhook call successful block=12345 url=https://api.example.com/webhook
```

## Best Practices

1. **Set appropriate timeouts**: Use shorter timeouts (5-15s) for real-time processing
2. **Configure retries based on endpoint reliability**: More retries for less reliable services
3. **Monitor webhook endpoint performance**: Adjust settings based on observed behavior
4. **Use state files**: Always specify a state file for production deployments
5. **Handle failures gracefully**: Implement idempotent webhook handlers that can handle duplicate calls

## Webhook Payload Format

The webhook receives JSON payloads in the following format:

```json
{
  "module": "module_name",
  "block": 12345,
  "timestamp": "2023-01-01T00:00:00Z",
  "type": "type.googleapis.com/sf.substreams.ethereum.v1.Events",
  "payload": {
    // Your module's output data
  }
}
```

Make sure your webhook endpoint can handle:
- POST requests with `Content-Type: application/json`
- Potential duplicate calls (implement idempotency)
- Proper HTTP status code responses (2xx for success, 4xx for permanent errors, 5xx for retryable errors)