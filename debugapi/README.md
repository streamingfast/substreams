# Substreams Debug API

A simple HTTP API for runtime debugging and management of Substreams tier2.

## Making Requests with curl

### Simulate Overloaded State

Get the current overloaded simulation state:

```bash
curl -s http://localhost:8081/debug/simulate_overloaded | jq
```

Example response:
```json
{
  "overloaded": false
}
```

Set the service to simulate being overloaded:

```bash
curl -s -X POST -H "Content-Type: application/json" -d '{"overloaded": true}' http://localhost:8081/debug/simulate_overloaded | jq
```

Alternative using query parameter:

```bash
curl -s -X POST http://localhost:8081/debug/simulate_overloaded?value=true | jq
```

Example response:
```json
{
  "overloaded": true
}
```

### Force Garbage Collection

Trigger an immediate garbage collection:

```bash
curl -s -X POST http://localhost:8081/debug/gc | jq
```

Example response:
```json
{
  "before": "Alloc = 235 MiB, Heap inuse = 245 MiB",
  "after": "Alloc = 198 MiB, Heap inuse = 210 MiB"
}
```

### Manage Active Requests

List all currently active requests:

```bash
curl -s http://localhost:8081/debug/requests | jq
```

Example response will show details of all active requests.

Cancel a specific request:

```bash
curl -s -X DELETE -H "Content-Type: application/json" -d '{
  "trace_id": "abc123",
  "output_module_hash": "def456"
}' http://localhost:8081/debug/requests | jq
```

Example response:
```json
{
  "matches": ["request-1", "request-2"]
}
```

For more specific cancellation, you can include optional parameters:

```bash
curl -s -X DELETE -H "Content-Type: application/json" -d '{
  "trace_id": "abc123",
  "output_module_hash": "def456",
  "segment_number": 5,
  "segment_size": 100,
  "stage": 2
}' http://localhost:8081/debug/requests | jq
```
