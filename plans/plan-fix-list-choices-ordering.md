# Implementation Plan for Codegen Agent

## Objective
Fix bug where list choices are rendered in random order on the client side. The server sends choices in a specific order, but the `substreams init` command displays them randomly due to improper map iteration in the client code. Submit PR to `github.com/streamingfast/substreams`.

## How to Use This Plan
- Follow steps sequentially, completing each before moving to the next
- Verify your work at each checkpoint using the specified commands
- If you encounter ambiguity, refer to the linked source files for context
- Run all verification commands from the project root directory
- Ensure all tests pass before considering a step complete
- Create a pull request when implementation is complete

---

## Problem Analysis

### Root Cause
The client-side code in `./cmd/substreams/init.go` uses Go maps to group and iterate over choices. Go maps have undefined iteration order, causing list selections to appear in random order each time.

### Affected Areas
There are **two distinct ordering issues** in the client code:

1. **Protocol Selection (lines 211-231)**: Groups generators by protocol using a map, then iterates over it
2. **Generator Selection within Protocol**: The generators within each group maintain order (slice), but the groups themselves are randomized

### Data Flow
```
Server (substreams-codegen)                    Client (substreams)
===========================                    ==================
ListConversationHandlers()
  -> sorts by Weight (deterministic)
  -> returns []*ConversationHandler

DiscoveryResponse.Generators                   resp.Msg.Generators (protobuf repeated - ordered)
  (protobuf repeated field - preserves order)    |
                                                 v
                                               filteredProtocols map[string]*BlockchainProtocolSelector
                                                 |
                                                 v  (ORDER LOST HERE)
                                               for _, value := range filteredProtocols { ... }
```

---

## Implementation Steps

### Step 1: Fix Protocol Selection Order Preservation

**File to modify**: `./cmd/substreams/init.go` (in the `substreams` repository)

**Current problematic code** (lines 204-231):
```go
type BlockchainProtocolSelector struct {
    Id         string
    Title      string
    Generators []*pbconvo.DiscoveryResponse_Generator
}

filteredProtocols := make(map[string]*BlockchainProtocolSelector)
for _, gen := range resp.Msg.Generators {
    selector, groupExists := filteredProtocols[gen.Group]
    if groupExists {
        selector.Generators = append(selector.Generators, gen)
    } else {
        filteredProtocols[gen.Group] = &BlockchainProtocolSelector{
            Id:         gen.Group,
            Title:      gen.Group,
            Generators: []*pbconvo.DiscoveryResponse_Generator{gen},
        }
    }
}

protocolOptions := make([]huh.Option[*BlockchainProtocolSelector], 0, len(filteredProtocols))
for _, value := range filteredProtocols {
    protocolOptions = append(protocolOptions, huh.Option[*BlockchainProtocolSelector]{
        Key:   value.Title,
        Value: value,
    })
}
```

**Fixed code**:
```go
type BlockchainProtocolSelector struct {
    Id         string
    Title      string
    Generators []*pbconvo.DiscoveryResponse_Generator
}

// Use a slice to maintain insertion order and a map for lookup
var protocolOrder []string
filteredProtocols := make(map[string]*BlockchainProtocolSelector)
for _, gen := range resp.Msg.Generators {
    selector, groupExists := filteredProtocols[gen.Group]
    if groupExists {
        selector.Generators = append(selector.Generators, gen)
    } else {
        protocolOrder = append(protocolOrder, gen.Group)
        filteredProtocols[gen.Group] = &BlockchainProtocolSelector{
            Id:         gen.Group,
            Title:      gen.Group,
            Generators: []*pbconvo.DiscoveryResponse_Generator{gen},
        }
    }
}

protocolOptions := make([]huh.Option[*BlockchainProtocolSelector], 0, len(filteredProtocols))
for _, group := range protocolOrder {
    value := filteredProtocols[group]
    protocolOptions = append(protocolOptions, huh.Option[*BlockchainProtocolSelector]{
        Key:   value.Title,
        Value: value,
    })
}
```

**Key changes**:
1. Added `protocolOrder []string` slice to track the order in which groups are first encountered
2. When a new group is encountered, append its name to `protocolOrder`
3. Iterate over `protocolOrder` instead of the map to build `protocolOptions`
4. Use `protocolOrder` to look up values from the map in the correct order

### Step 2: Verify Generator Selection Order

The generator selection within a protocol group already uses a slice (`selector.Generators`), so order is preserved there. However, verify that lines 246-263 maintain order:

**Current code** (lines 246-263):
```go
generatorOptions := make([]huh.Option[*pbconvo.DiscoveryResponse_Generator], 0, len(selector.Generators))
for _, gen := range selector.Generators {
    endpoint := ""
    if gen.Endpoint != "" {
        endpoint = " (" + gen.Endpoint + ")"
    }

    key := fmt.Sprintf("%-20s - %s", gen.Id, gen.Title)
    if endpoint != "" {
        key = fmt.Sprintf("%-20s (%-40s) - %s", gen.Id, endpoint, gen.Title)
    }

    entry := huh.Option[*pbconvo.DiscoveryResponse_Generator]{
        Key:   key,
        Value: gen,
    }
    generatorOptions = append(generatorOptions, entry)
}
```

This code is correct - it iterates over a slice in order. **No changes needed here.**

### Step 3: Verify the optionsValueToKey Map Usage

Check lines 391-403 for the `SystemOutput_ListSelect_` handling:

**Current code** (lines 391-403):
```go
var options []huh.Option[string]
optionsValueToKey := make(map[string]string)
for i := 0; i < len(input.Labels); i++ {
    entry := huh.Option[string]{
        Key:   input.Labels[i],
        Value: input.Values[i],
    }
    options = append(options, entry)
    optionsValueToKey[entry.Value] = entry.Key
}
```

This code is correct - it iterates using an index over parallel slices (`Labels` and `Values`), maintaining order. The `optionsValueToKey` map is only used for reverse lookup after selection, not for iteration. **No changes needed here.**

---

## Verification Steps

After implementing the fix:

### 1. Build Verification
```bash
# In the substreams repository
go build ./cmd/substreams
```
Expected: Clean build with no errors or warnings

### 2. Unit Tests
```bash
# Run existing tests
go test ./cmd/... -v
```
Expected: All tests pass

### 3. Manual Testing - Critical
Run the `substreams init` command multiple times and verify ordering is consistent:

```bash
# Run init command multiple times
./substreams init --state-file /tmp/test1.json
# Cancel after seeing protocol selection
# Note the order of protocols shown

./substreams init --state-file /tmp/test2.json
# Cancel after seeing protocol selection
# Verify order matches the first run
```

**Expected behavior**:
- Protocol selection should always show protocols in the same order (e.g., EVM first, then Injective, Solana, etc. - based on server-side weight ordering)
- Generator selection within a protocol should always show generators in the same order
- Sink/consumption choices should always appear in the same order

### 4. Integration Testing with Local Server (Optional)
If you have access to run the `substreams-codegen` server locally:

```bash
# Start local codegen server
cd /path/to/substreams-codegen
go run ./cmd/server

# In another terminal, test with local server
SUBSTREAMS_CODEGEN_ENDPOINT=http://localhost:8080 ./substreams init
```

---

## Testing Strategy

### Add Unit Test for Order Preservation

Create a new test file or add to existing test file `./cmd/substreams/init_test.go`:

```go
package main

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    pbconvo "github.com/streamingfast/substreams/pb/sf/codegen/conversation/v1"
)

func TestProtocolOrderPreservation(t *testing.T) {
    // Simulate generators received from server in specific order
    generators := []*pbconvo.DiscoveryResponse_Generator{
        {Id: "evm-events-calls", Group: "EVM", Title: "EVM Events/Calls"},
        {Id: "evm-hello-world", Group: "EVM", Title: "EVM Hello World"},
        {Id: "sol-anchor", Group: "Solana", Title: "Solana Anchor"},
        {Id: "sol-hello-world", Group: "Solana", Title: "Solana Hello World"},
        {Id: "starknet-events", Group: "Starknet", Title: "Starknet Events"},
    }

    // Reproduce the grouping logic
    type BlockchainProtocolSelector struct {
        Id         string
        Title      string
        Generators []*pbconvo.DiscoveryResponse_Generator
    }

    var protocolOrder []string
    filteredProtocols := make(map[string]*BlockchainProtocolSelector)
    for _, gen := range generators {
        selector, groupExists := filteredProtocols[gen.Group]
        if groupExists {
            selector.Generators = append(selector.Generators, gen)
        } else {
            protocolOrder = append(protocolOrder, gen.Group)
            filteredProtocols[gen.Group] = &BlockchainProtocolSelector{
                Id:         gen.Group,
                Title:      gen.Group,
                Generators: []*pbconvo.DiscoveryResponse_Generator{gen},
            }
        }
    }

    // Verify order is preserved
    require.Len(t, protocolOrder, 3)
    assert.Equal(t, "EVM", protocolOrder[0])
    assert.Equal(t, "Solana", protocolOrder[1])
    assert.Equal(t, "Starknet", protocolOrder[2])

    // Verify generators within each group maintain order
    evmGens := filteredProtocols["EVM"].Generators
    require.Len(t, evmGens, 2)
    assert.Equal(t, "evm-events-calls", evmGens[0].Id)
    assert.Equal(t, "evm-hello-world", evmGens[1].Id)

    solanaGens := filteredProtocols["Solana"].Generators
    require.Len(t, solanaGens, 2)
    assert.Equal(t, "sol-anchor", solanaGens[0].Id)
    assert.Equal(t, "sol-hello-world", solanaGens[1].Id)
}
```

---

## Project Standards Checklist
- [ ] Code follows Go conventions and existing patterns in the repository
- [ ] No new dependencies added
- [ ] Variable names are clear and descriptive (`protocolOrder` clearly indicates purpose)
- [ ] Comments explain the "why" (preserving server-sent order)
- [ ] Existing tests still pass
- [ ] New test added for order preservation

---

## Commit Boundaries

### Commit 1: Fix protocol selection ordering
**Files changed**: `./cmd/substreams/init.go`
**Message**: `fix(init): preserve protocol selection order from server`

This commit adds a `protocolOrder` slice to track insertion order when grouping generators by protocol, ensuring the protocol selection menu displays options in the order the server intended (sorted by weight).

### Commit 2: Add test for order preservation
**Files changed**: `./cmd/substreams/init_test.go` (new or modified)
**Message**: `test(init): add test for protocol order preservation`

This commit adds a unit test verifying that protocol grouping preserves the order received from the server.

---

## Additional Notes

### Why the Server Order Matters
The `substreams-codegen` server (`ListConversationHandlers()` in `./registry.go`) explicitly sorts handlers by weight:
- EVM: weight 80+
- Injective: weight 70+
- Solana: weight 60+
- Starknet: weight 50+
- etc.

This ordering is intentional - it presents the most commonly used protocols first. The client must preserve this ordering to provide a consistent user experience.

### Alternative Approach Considered (Not Recommended)
An alternative would be to sort protocols alphabetically on the client side. This was rejected because:
1. It overrides the server's intentional weight-based ordering
2. It doesn't match user expectations (popular chains should appear first)
3. It would require additional sorting logic

### Related Files Reference
- Server-side ordering: https://github.com/streamingfast/substreams-codegen/blob/develop/registry.go#L40-L50
- Server-side discovery: https://github.com/streamingfast/substreams-codegen/blob/develop/server/handler_convo.go#L42-L60
- Client-side issue: https://github.com/streamingfast/substreams/blob/develop/cmd/substreams/init.go#L211-L231
