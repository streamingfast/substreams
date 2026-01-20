# Implementation Plan for Codegen Agent

## Objective
Implement enhanced keyboard navigation features for text input fields in the `substreams init` command as a PR on github.com/streamingfast/substreams. This includes:
- Arrow Up/Down keys to navigate through input history (previous text entries in the session)
- Tab completion for file paths based on the current directory structure
- Support for cycling through multiple completion options

## How to Use This Plan
- Follow steps sequentially, completing each before moving to the next
- Verify your work at each checkpoint using the specified commands
- If you encounter ambiguity, refer to the linked source files for context
- Run all verification commands from the project root directory
- Ensure all tests pass before considering a step complete
- Create a pull request when implementation is complete

## Context: Current Implementation

The `substreams init` command currently uses the `charmbracelet/huh` library (v0.6.0) for terminal UI interactions. Text inputs are handled via `huh.NewInput()` which creates basic input fields with validation but no history or tab completion features.

**Current text input handling location:**
- https://github.com/streamingfast/substreams/blob/develop/cmd/substreams/init.go#L431-L476

The code currently:
1. Creates a `huh.NewInput()` field with title, description, placeholder, and default value
2. Optionally adds validation using regex patterns
3. Runs the form with `huh.NewForm(huh.NewGroup(inputField)).WithTheme(huhTheme).WithAccessible(WITH_ACCESSIBLE).Run()`
4. Sends the result back to the server via protobuf

**Improvement Opportunity:** Upgrading to the latest version of `huh` may provide native support for features like FilePicker (for path completion) and potentially better key binding customization.

## Implementation Strategy

We will first upgrade the `huh` library to the latest version (v0.8.0) and leverage its built-in capabilities. The implementation has two approaches based on verified API capabilities:

### Approach 1: Use huh FilePicker for file paths (Verified Available)
**Status: Confirmed in huh v0.8.0 (since v0.4.0)**

The `huh.FilePicker` component provides:
- Built-in file and directory navigation
- Visual directory browsing
- Keyboard navigation (arrow keys, page up/down, etc.)
- File filtering by type
- Current directory selection

API methods include:
- `.CurrentDirectory(dir)` - Set starting directory
- `.FileAllowed(bool)` - Allow file selection
- `.DirAllowed(bool)` - Allow directory selection
- `.AllowedTypes([]string)` - Filter by file extensions
- `.Title()`, `.Description()` - Set labels
- `.Value(*string)` - Bind to variable

This eliminates the need for custom path completion for file-related inputs.

### Approach 2: Custom Bubble Tea component for text input history
**Status: Required (huh.Input does not expose custom key handlers)**

Since `huh.Input` does not provide hooks for custom key bindings, we must create a custom Bubble Tea component for text inputs that need:
- Up/Down arrow history navigation
- Session-based input tracking
- Compatibility with the existing huh theme and form flow

This component will:
- Implement the `huh.Field` interface
- Use `charmbracelet/bubbles/textinput` for the base input widget
- Integrate with the existing form themes and validation
- Replace `huh.NewInput()` only where history is needed

## Step 1: Upgrade huh and Research Implementation Approach

### Part A: Upgrade huh Library

First, upgrade the `huh` library to the latest version to access newer features.

1. **Update go.mod:**
```bash
# From project root
go get github.com/charmbracelet/huh@latest
go get github.com/charmbracelet/huh/spinner@latest
go mod tidy
```

2. **Verify the upgrade:**
```bash
go list -m github.com/charmbracelet/huh
# Should show a version newer than v0.6.0
```

3. **Build to ensure no breaking changes:**
```bash
go build ./cmd/substreams
```

Expected: Successful upgrade with no compilation errors. If there are breaking changes, document them and adjust the init.go code accordingly.

### Part B: Verification Tasks

1. **Verify FilePicker is available:**
```bash
# After upgrade, check if FilePicker type exists
cd /Users/maoueh/work/sf/substreams
go doc github.com/charmbracelet/huh.FilePicker

# Should show:
# type FilePicker struct { ... }
# func NewFilePicker() *FilePicker
# ... (methods)
```

2. **Verify huh.Input limitations:**
```bash
# Check if Input has custom key handler methods
go doc github.com/charmbracelet/huh.Input | grep -i "key\|handler\|bind"

# Expected: Only WithKeyMap(k *KeyMap) which sets the form-level keymap,
# not field-specific handlers. No methods like OnKeyPress or similar.
```

3. **Document findings:**
```bash
cat > /tmp/huh-investigation.txt << EOF
huh version: v0.8.0
FilePicker available: YES (since v0.4.0)
Input custom key handlers: NO
Chosen approach:
  - FilePicker for file paths (Approach 1)
  - Custom Bubble Tea component for text input history (Approach 2)
EOF
cat /tmp/huh-investigation.txt
```

### Verification
```bash
# Verify upgrade was successful
go list -m github.com/charmbracelet/huh
# Expected: github.com/charmbracelet/huh v0.8.0

# Verify FilePicker is importable
go build ./cmd/substreams
# Expected: Clean build
```

Expected: huh v0.8.0 is installed and FilePicker type is available

## Step 2: Implement Input History Tracking

### Implementation Details

Create a new file `./cmd/substreams/init_input_history.go` to manage input history:

```go
package main

import (
    "sync"
)

// InputHistory manages a session-based history of text inputs
type InputHistory struct {
    mu      sync.RWMutex
    entries []string
    cursor  int // Current position in history (-1 means not navigating)
    current string // Stores the text user was typing before navigating history
}

// NewInputHistory creates a new input history tracker
func NewInputHistory() *InputHistory {
    return &InputHistory{
        entries: make([]string, 0, 50), // Pre-allocate for efficiency
        cursor:  -1,
    }
}

// Add records a new input to history (called after user submits input)
func (h *InputHistory) Add(input string) {
    h.mu.Lock()
    defer h.mu.Unlock()

    // Don't add empty inputs or duplicates of the last entry
    if input == "" || (len(h.entries) > 0 && h.entries[len(h.entries)-1] == input) {
        return
    }

    h.entries = append(h.entries, input)
    h.cursor = -1 // Reset navigation state
}

// NavigateUp returns the previous entry in history
// Returns (value, changed) where changed indicates if navigation occurred
func (h *InputHistory) NavigateUp(currentValue string) (string, bool) {
    h.mu.Lock()
    defer h.mu.Unlock()

    if len(h.entries) == 0 {
        return currentValue, false
    }

    // First time navigating, save current input
    if h.cursor == -1 {
        h.current = currentValue
        h.cursor = len(h.entries) - 1
        return h.entries[h.cursor], true
    }

    // Already navigating, go further back
    if h.cursor > 0 {
        h.cursor--
        return h.entries[h.cursor], true
    }

    // At the oldest entry, no change
    return h.entries[h.cursor], false
}

// NavigateDown returns the next entry in history
// Returns (value, changed) where changed indicates if navigation occurred
func (h *InputHistory) NavigateDown(currentValue string) (string, bool) {
    h.mu.Lock()
    defer h.mu.Unlock()

    if h.cursor == -1 {
        // Not currently navigating history
        return currentValue, false
    }

    if h.cursor < len(h.entries)-1 {
        h.cursor++
        return h.entries[h.cursor], true
    }

    // Reached the end, return to user's current input
    h.cursor = -1
    return h.current, true
}

// Reset clears navigation state (call when input is submitted or cancelled)
func (h *InputHistory) Reset() {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.cursor = -1
    h.current = ""
}
```

### Integration Points

In `./cmd/substreams/init.go`:

1. **Add global history tracker** (near line 90 after `type UserState struct`):
```go
var globalInputHistory *InputHistory

func init() {
    // Existing init code...

    // Initialize input history tracker
    globalInputHistory = NewInputHistory()
}
```

2. **Store inputs after successful entry** (after line 469 where input is printed):
```go
// After fmt.Println(gray("┃"), input.Prompt+":", bold(returnValue))
globalInputHistory.Add(returnValue)
```

### Verification
```bash
# Run tests for the new input history module
go test ./cmd/substreams -run TestInputHistory -v
```

Expected: All input history unit tests pass

## Step 3: Remove Path Completion Logic (Not Needed)

**Decision:** Skip implementing custom path completion entirely.

**Rationale:**
- FilePicker (confirmed available in v0.8.0) provides better UX for file selection than Tab completion
- Tab completion in terminal UIs is less discoverable and harder to use than visual file browsers
- The original requirement for "Tab completion" is better served by FilePicker's arrow-key navigation
- Reduces code complexity and maintenance burden

**Implementation Impact:**
- Use FilePicker for all file/directory path inputs (Step 4)
- Focus custom component (Step 5) exclusively on history navigation (Up/Down arrows)
- No Tab key handling needed in the custom component

## Step 4: Integrate FilePicker for File Path Inputs (If Available)

**Conditional Step:** Only proceed with this step if Step 1 confirmed that the latest `huh` version includes a FilePicker component. Otherwise, skip to Step 5.

### Implementation Details

If FilePicker is available, we should use it specifically for inputs that request file paths. The `substreams init` flow includes several file path questions (e.g., ABI files, contract addresses from files).

### Identify File Path Inputs

In the conversation flow, file path inputs can be identified by:
1. The prompt text (contains words like "file", "path", "ABI", "contract")
2. A specific protobuf field indicating file input type
3. Heuristic detection in the default value or placeholder

Modify `./cmd/substreams/init.go` to detect and use FilePicker:

```go
case *pbconvo.SystemOutput_TextInput_:
    input := msg.TextInput

    // Check if this input is requesting a file path
    isFilePath := isFilePathInput(input.Prompt, input.Description, input.Placeholder)

    var returnValue string

    if isFilePath {
        // Use FilePicker for file path inputs
        returnValue, err = runFilePicker(input.Prompt, input.Description, input.DefaultValue)
        if err != nil {
            return fmt.Errorf("failed taking file input: %w", err)
        }
    } else {
        // Use enhanced text input with history for other inputs
        returnValue, err = runEnhancedTextInput(
            input.Prompt,
            input.Description,
            input.Placeholder,
            input.DefaultValue,
            input.ValidationRegexp,
            input.ValidationErrorMessage,
        )
        if err != nil {
            return fmt.Errorf("failed taking text input: %w", err)
        }
    }

    fmt.Println(gray("┃"), input.Prompt+":", bold(returnValue))
    fmt.Println("")

    // Record in history
    globalInputHistory.Add(returnValue)

    // Send to server
    if err := sendFunc(&pbconvo.UserInput{
        FromActionId: resp.ActionId,
        Entry: &pbconvo.UserInput_TextInput_{
            TextInput: &pbconvo.UserInput_TextInput{Value: strings.TrimRight(returnValue, " ")},
        },
    }); err != nil {
        return fmt.Errorf("error sending text input message: %w", err)
    }
```

Implement the helper functions:

```go
// isFilePathInput uses heuristics to determine if an input is requesting a file path
func isFilePathInput(prompt, description, placeholder string) bool {
    combined := strings.ToLower(prompt + " " + description + " " + placeholder)
    keywords := []string{"file", "path", "abi", "contract", "json", ".sol", ".yaml", "directory", "folder"}

    for _, keyword := range keywords {
        if strings.Contains(combined, keyword) {
            return true
        }
    }

    return false
}

// runFilePicker runs the huh FilePicker component
func runFilePicker(prompt, description, defaultPath string) (string, error) {
    // Determine starting directory
    startDir := "."
    if defaultPath != "" {
        if stat, err := os.Stat(defaultPath); err == nil && stat.IsDir() {
            startDir = defaultPath
        } else {
            startDir = filepath.Dir(defaultPath)
        }
    }

    var selectedPath string

    // Create FilePicker using huh API (adjust based on actual API)
    picker := huh.NewFilePicker().
        Title(prompt).
        Description(description).
        CurrentDirectory(startDir).
        Value(&selectedPath)

    err := huh.NewForm(huh.NewGroup(picker)).WithTheme(huhTheme).WithAccessible(WITH_ACCESSIBLE).Run()
    if err != nil {
        return "", err
    }

    return selectedPath, nil
}

// runEnhancedTextInput runs enhanced text input with history (implemented in Step 5)
func runEnhancedTextInput(prompt, description, placeholder, defaultValue, validationRegex, validationError string) (string, error) {
    // Implementation in Step 5
    return "", nil
}
```

### Key Detail: FilePicker Benefits

Using `huh.FilePicker` (if available) provides:
1. **Native tab completion**: Built into the component
2. **Visual directory navigation**: Shows directory contents
3. **File filtering**: Can filter by extension
4. **Consistent UX**: Matches other huh components
5. **Less custom code**: Reduces maintenance burden

### Verification
```bash
# Build and test with FilePicker
go build ./cmd/substreams

# Manual test
./substreams init
# When prompted for a file path, verify:
# - Directory navigation works
# - Tab completion works (if supported by FilePicker)
# - File selection works
```

Expected: FilePicker provides intuitive file selection experience

## Step 5: Create Enhanced Input Component for Text Inputs

**Decision:** Implement custom Bubble Tea component (huh.Input does not support custom key handlers).

Based on API verification, `huh.Input` only provides `.WithKeyMap(k *KeyMap)` which sets form-level keybindings, not field-specific handlers. We need a custom component for history navigation.

Create `./cmd/substreams/enhanced_input.go`:

```go
package main

import (
    "fmt"
    "regexp"
    "strings"

    "github.com/charmbracelet/bubbles/textinput"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
)

// EnhancedInput is a custom input component with history and tab completion
type EnhancedInput struct {
    textInput      textinput.Model
    history        *InputHistory
    prompt         string
    description    string
    validation     *regexp.Regexp
    validationErr  string
    submitted      bool
    cancelled      bool
}

// NewEnhancedInput creates a new enhanced input component
func NewEnhancedInput(prompt, description, placeholder, defaultValue string, validationRegex, validationError string) *EnhancedInput {
    ti := textinput.New()
    ti.Placeholder = placeholder
    ti.SetValue(defaultValue)
    ti.Focus()

    var validationRE *regexp.Regexp
    if validationRegex != "" {
        var err error
        validationRE, err = regexp.Compile(validationRegex)
        if err != nil {
            // Log error but continue without validation
            validationRE = nil
        }
    }

    return &EnhancedInput{
        textInput:      ti,
        history:        globalInputHistory,
        prompt:         prompt,
        description:    description,
        validation:     validationRE,
        validationErr:  validationError,
    }
}

// Init implements tea.Model
func (e *EnhancedInput) Init() tea.Cmd {
    return textinput.Blink
}

// Update implements tea.Model
func (e *EnhancedInput) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmd tea.Cmd

    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch msg.String() {
        case "ctrl+c", "esc":
            e.cancelled = true
            return e, tea.Quit

        case "enter":
            value := strings.TrimRight(e.textInput.Value(), " ")

            // Validate if validation is set
            if e.validation != nil && !e.validation.MatchString(value) {
                // Show validation error (could use a status message)
                return e, nil
            }

            e.submitted = true
            e.history.Add(value)
            e.history.Reset()
            return e, tea.Quit

        case "up":
            if value, changed := e.history.NavigateUp(e.textInput.Value()); changed {
                e.textInput.SetValue(value)
                // Move cursor to end
                e.textInput.CursorEnd()
            }
            return e, nil

        case "down":
            if value, changed := e.history.NavigateDown(e.textInput.Value()); changed {
                e.textInput.SetValue(value)
                e.textInput.CursorEnd()
            }
            return e, nil

        // Note: Tab completion removed - use FilePicker for file paths instead
        }
    }

    e.textInput, cmd = e.textInput.Update(msg)
    return e, cmd
}

// View implements tea.Model
func (e *EnhancedInput) View() string {
    titleStyle := lipgloss.NewStyle().Bold(true)
    descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

    view := ""
    if e.prompt != "" {
        view += titleStyle.Render(e.prompt) + "\n"
    }
    if e.description != "" {
        view += descStyle.Render(e.description) + "\n"
    }
    view += e.textInput.View() + "\n"

    return view
}

// Run executes the enhanced input and returns the value and any error
func (e *EnhancedInput) Run() (string, error) {
    p := tea.NewProgram(e)
    finalModel, err := p.Run()
    if err != nil {
        return "", err
    }

    finalInput := finalModel.(*EnhancedInput)
    if finalInput.cancelled {
        return "", fmt.Errorf("input cancelled")
    }

    return finalInput.textInput.Value(), nil
}
```

Then modify `./cmd/substreams/init.go` to use it:

```go
case *pbconvo.SystemOutput_TextInput_:
    input := msg.TextInput

    enhancedInput := NewEnhancedInput(
        input.Prompt,
        input.Description,
        input.Placeholder,
        input.DefaultValue,
        input.ValidationRegexp,
        input.ValidationErrorMessage,
    )

    returnValue, err := enhancedInput.Run()
    if err != nil {
        return fmt.Errorf("failed taking input: %w", err)
    }

    fmt.Println(gray("┃"), input.Prompt+":", bold(returnValue))
    fmt.Println("")

    if err := sendFunc(&pbconvo.UserInput{
        FromActionId: resp.ActionId,
        Entry: &pbconvo.UserInput_TextInput_{
            TextInput: &pbconvo.UserInput_TextInput{Value: strings.TrimRight(returnValue, " ")},
        },
    }); err != nil {
        return fmt.Errorf("error sending text input message: %w", err)
    }
```

### Key Detail: Implementation Decision

After verifying the huh v0.8.0 API, the implementation approach is determined:

**Confirmed Facts:**
1. ✅ `huh.FilePicker` exists (since v0.4.0) - use for file path inputs
2. ❌ `huh.Input` does not support custom key handlers - requires custom component

**Implementation Path:**
- Use the custom Bubble Tea `EnhancedInput` component shown above
- It integrates with existing huh themes while providing history navigation
- FilePicker handles all file path selection needs

No need for Option A since `huh.Input.WithKeyMap()` only accepts form-level KeyMap, not per-field handlers.

## Step 5: Add Unit Tests

Create `./cmd/substreams/init_input_history_test.go`:

```go
package main

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestInputHistory_Add(t *testing.T) {
    history := NewInputHistory()

    history.Add("first")
    history.Add("second")
    history.Add("second") // Duplicate should not be added
    history.Add("")       // Empty should not be added

    assert.Len(t, history.entries, 2)
    assert.Equal(t, "first", history.entries[0])
    assert.Equal(t, "second", history.entries[1])
}

func TestInputHistory_NavigateUp(t *testing.T) {
    history := NewInputHistory()
    history.Add("first")
    history.Add("second")
    history.Add("third")

    // First up should return most recent
    value, changed := history.NavigateUp("current")
    require.True(t, changed)
    assert.Equal(t, "third", value)

    // Second up should return previous
    value, changed = history.NavigateUp("current")
    require.True(t, changed)
    assert.Equal(t, "second", value)

    // Third up should return first
    value, changed = history.NavigateUp("current")
    require.True(t, changed)
    assert.Equal(t, "first", value)

    // Fourth up should stay at first
    value, changed = history.NavigateUp("current")
    assert.False(t, changed)
    assert.Equal(t, "first", value)
}

func TestInputHistory_NavigateDown(t *testing.T) {
    history := NewInputHistory()
    history.Add("first")
    history.Add("second")
    history.Add("third")

    // Navigate up to start
    history.NavigateUp("current")
    history.NavigateUp("current")
    history.NavigateUp("current")

    // Now navigate down
    value, changed := history.NavigateDown("current")
    require.True(t, changed)
    assert.Equal(t, "second", value)

    value, changed = history.NavigateDown("current")
    require.True(t, changed)
    assert.Equal(t, "third", value)

    // At the end, should return original current value
    value, changed = history.NavigateDown("current")
    require.True(t, changed)
    assert.Equal(t, "current", value)

    // Further down should not change
    value, changed = history.NavigateDown("current")
    assert.False(t, changed)
}

func TestInputHistory_Empty(t *testing.T) {
    history := NewInputHistory()

    value, changed := history.NavigateUp("test")
    assert.False(t, changed)
    assert.Equal(t, "test", value)
}
```

Create `./cmd/substreams/path_completion_test.go`:

**Note:** These tests are now OPTIONAL since we're using FilePicker instead of custom path completion. If you skip Step 3 entirely (recommended), skip these tests too.

```go
package main

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestPathCompletion_BasicCompletion(t *testing.T) {
    // Create temporary test directory structure
    tmpDir := t.TempDir()

    // Create test files and directories
    require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "testdir"), 0755))
    require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "testfile.txt"), []byte("test"), 0644))
    require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "testfile2.txt"), []byte("test"), 0644))

    // Change to temp directory for testing
    originalWd, err := os.Getwd()
    require.NoError(t, err)
    defer os.Chdir(originalWd)
    require.NoError(t, os.Chdir(tmpDir))

    pc := NewPathCompletion()

    // Test completing "test" should find multiple matches
    completed, hasMore := pc.Complete("test")
    assert.True(t, hasMore, "Should have multiple matches")
    assert.Contains(t, []string{"testdir/", "testfile.txt", "testfile2.txt"}, filepath.Base(completed))
}

func TestPathCompletion_DirectoryCompletion(t *testing.T) {
    tmpDir := t.TempDir()

    // Create nested structure
    require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "parent", "child1"), 0755))
    require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "parent", "child2"), 0755))

    originalWd, err := os.Getwd()
    require.NoError(t, err)
    defer os.Chdir(originalWd)
    require.NoError(t, os.Chdir(tmpDir))

    pc := NewPathCompletion()

    // Complete directory path with trailing separator
    completed, hasMore := pc.Complete("parent/")
    assert.True(t, hasMore)
    assert.Contains(t, completed, "child")
}

func TestPathCompletion_NoMatches(t *testing.T) {
    tmpDir := t.TempDir()

    originalWd, err := os.Getwd()
    require.NoError(t, err)
    defer os.Chdir(originalWd)
    require.NoError(t, os.Chdir(tmpDir))

    pc := NewPathCompletion()

    // No matches should return input unchanged
    completed, hasMore := pc.Complete("nonexistent")
    assert.False(t, hasMore)
    assert.Equal(t, "nonexistent", completed)
}

func TestPathCompletion_CycleMatches(t *testing.T) {
    tmpDir := t.TempDir()

    require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("test"), 0644))
    require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("test"), 0644))

    originalWd, err := os.Getwd()
    require.NoError(t, err)
    defer os.Chdir(originalWd)
    require.NoError(t, os.Chdir(tmpDir))

    pc := NewPathCompletion()

    // First tab
    first, _ := pc.Complete("file")

    // Second tab should give different result
    second, _ := pc.Complete("file")

    // Should cycle through options
    assert.NotEqual(t, first, second)

    // Third tab should cycle back or to next option
    third, _ := pc.Complete("file")
    assert.True(t, third == first || third != second)
}
```

### Verification
```bash
# Run all new unit tests
go test ./cmd/substreams -v -run "TestInputHistory|TestPathCompletion"
```

Expected: All tests pass with clear output showing each test case

## Step 6: Integration Testing

Create `./cmd/substreams/enhanced_input_integration_test.go`:

```go
package main

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/require"
)

func TestEnhancedInput_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }

    // This test validates that the enhanced input components
    // integrate properly with the init flow

    tmpDir := t.TempDir()
    require.NoError(t, os.Chdir(tmpDir))

    // Create test directory structure
    require.NoError(t, os.MkdirAll("contracts", 0755))
    require.NoError(t, os.WriteFile("contracts/Token.sol", []byte("contract"), 0644))

    // Initialize history
    history := NewInputHistory()

    // Simulate user entering paths
    history.Add("./contracts/")
    history.Add("./contracts/Token.sol")

    // Test history navigation
    value, changed := history.NavigateUp("current")
    require.True(t, changed)
    require.Equal(t, "./contracts/Token.sol", value)

    // Test path completion
    pc := NewPathCompletion()
    completed, hasMore := pc.Complete("./contract")
    require.True(t, hasMore || completed == "./contracts/")
    require.Contains(t, completed, "contracts")
}
```

### Manual Testing Procedure

Since this feature involves interactive terminal input, manual testing is essential:

1. **Build the binary:**
```bash
go build -o substreams-test ./cmd/substreams
```

2. **Test input history:**
```bash
# Run init command
./substreams-test init

# When prompted for text input:
# 1. Type "first input" and press Enter
# 2. At next text input, press Up Arrow
#    - Should show "first input"
# 3. Type "second input" and press Enter
# 4. At next text input, press Up Arrow twice
#    - Should show "first input"
# 5. Press Down Arrow
#    - Should show "second input"
# 6. Press Down Arrow again
#    - Should return to empty/current input
```

3. **Test path completion:**
```bash
# Create test directory structure
mkdir -p test-project/contracts
touch test-project/contracts/Token.sol
touch test-project/contracts/NFT.sol

# Run init command
./substreams-test init

# When prompted for a file path:
# 1. Type "test-pro" and press Tab
#    - Should complete to "test-project/"
# 2. Type "c" and press Tab
#    - Should complete to "test-project/contracts/"
# 3. Type "T" and press Tab
#    - Should complete to "test-project/contracts/Token.sol"
# 4. Delete back to "contracts/" and type "T", press Tab, Tab again
#    - Should cycle between Token.sol and back to T
```

### Verification Checklist

Run through the complete verification:

```bash
# Unit tests
go test ./cmd/substreams -v -run "TestInputHistory|TestPathCompletion"

# Integration tests
go test ./cmd/substreams -v -run "TestEnhancedInput_Integration"

# Build succeeds
go build ./cmd/substreams

# Existing tests still pass
go test ./cmd/substreams -v

# No regressions in init command without new features
./substreams-test init  # Complete a normal flow without using arrows/tab
```

Expected: All tests pass, build succeeds, and manual testing confirms features work as specified

## Step 7: Documentation and Polish

### Update Documentation

Add documentation for the new features to the `docs/` directory. Since `substreams init` is documented in multiple places, update:

1. **CLI Reference** (`./docs/references/cli/command-line-interface.md`):

Add a note about the enhanced input features around line 27-33:

```markdown
### **`init`**

The `init` command allows you to initialize a Substreams project for several blockchains. It is a conversational-like command: you will be asked several questions and a project with the specified features will be created for you.

The options included in the `init` command will evolve over time, but every blockchain should, at least, contain one option.

```bash
substreams init
```

**Enhanced Input Features:**
- **Arrow Keys:** Press Up/Down arrows to navigate through previously entered text inputs in the current session
- **Tab Completion:** Press Tab to auto-complete file paths based on your current directory structure. Press Tab multiple times to cycle through matching options
```

2. **Create a new how-to guide** (`./docs/how-to-guides/cli/init-keyboard-shortcuts.md`):

```markdown
# Keyboard Shortcuts in substreams init

The `substreams init` command supports enhanced keyboard navigation to make project initialization faster and more efficient.

## Input History

When answering text input questions, you can use the arrow keys to recall previous answers from the current session:

- **Up Arrow (↑):** Navigate backward through your input history
- **Down Arrow (↓):** Navigate forward through your input history, or return to your current input

### Example

```
? Enter contract address: 0x1234...
? Enter starting block: 12345
? Enter contract address: [Press ↑]
# Shows: 0x1234...
```

## Path Completion

When entering file paths, press Tab to auto-complete based on files and directories in your current location:

- **First Tab:** Complete the path or show first match
- **Additional Tabs:** Cycle through all matching paths
- **Directory Indicator:** Completed directories end with `/` to indicate you can continue

### Example

```
? Enter ABI file path: ./cont[Tab]
# Completes to: ./contracts/
? Enter ABI file path: ./contracts/To[Tab]
# Completes to: ./contracts/Token.json
```

### Tips

1. Tab completion works with both relative and absolute paths
2. Hidden files (starting with `.`) only appear if you type `.` first
3. Path completion recognizes `~` for home directory and environment variables
4. History is session-based - it clears when you exit the command
```

### Code Comments

Add helpful comments to the new code modules:

1. In `input_history.go`, add a package-level comment:
```go
// Package main provides enhanced input handling for the substreams init command.
// The InputHistory type manages a session-based history of text inputs that users
// can navigate using arrow keys, similar to shell command history.
```

2. In `path_completion.go`, add a package-level comment:
```go
// Package main provides path completion functionality for text inputs.
// The PathCompletion type handles Tab completion for file system paths,
// supporting both files and directories with smart matching.
```

### Error Handling

Ensure graceful degradation:

1. If path completion fails (e.g., permission issues), the input should still work without completion
2. If history tracking fails, the input should still accept manual entry
3. Log errors at debug level so they don't disrupt the user experience

Add this helper in `./cmd/substreams/init.go`:

```go
// safeComplete wraps path completion with error recovery
func safeComplete(pc *PathCompletion, input string) string {
    defer func() {
        if r := recover(); r != nil {
            // Log but don't crash
            if INIT_TRACE {
                fmt.Fprintf(os.Stderr, "Path completion error: %v\n", r)
            }
        }
    }()

    completed, _ := pc.Complete(input)
    return completed
}
```

## Verification Steps

After implementing all components:

### 1. Unit Tests
```bash
# Run all tests in the cmd/substreams package
go test ./cmd/substreams -v

# Specifically test the new components
go test ./cmd/substreams -v -run "TestInputHistory|TestPathCompletion"
```
Expected: All tests pass, coverage > 80% for new code

### 2. Build Verification
```bash
# Clean build
go clean
go build ./cmd/substreams

# Verify binary works
./substreams version
```
Expected: Clean build with no warnings

### 3. Integration Test
```bash
# Run the integration test
go test ./cmd/substreams -v -run "TestEnhancedInput_Integration"
```
Expected: Integration test passes

### 4. Manual Testing

Complete a full `substreams init` flow testing all features:

```bash
# Create test environment
mkdir /tmp/substreams-test
cd /tmp/substreams-test
mkdir -p contracts
echo '{}' > contracts/TestContract.json

# Run init
substreams init

# Test checklist:
# [ ] Text inputs work normally without special keys
# [ ] Up arrow shows previous input
# [ ] Down arrow navigates forward through history
# [ ] Down at end returns to current input
# [ ] Tab completes directory names
# [ ] Tab completes file names
# [ ] Multiple tabs cycle through options
# [ ] Tab does nothing when no matches
# [ ] Validation still works correctly
# [ ] Can complete full init flow successfully
```

### 5. Regression Testing

Ensure existing functionality still works:

```bash
# Test all substreams commands still work
./substreams build --help
./substreams run --help
./substreams gui --help

# Test init without using new features
./substreams init
# Complete normally without pressing arrow keys or tab
```

Expected: All existing commands work as before

## Project Standards Checklist

Before submitting the PR:

- [ ] Code follows Go conventions (gofmt, golint)
- [ ] All functions have appropriate documentation comments
- [ ] Error messages are descriptive and actionable
- [ ] Logging uses the existing zap logger when appropriate
- [ ] New code has >80% test coverage
- [ ] Integration with existing `huh` theme is maintained
- [ ] Accessibility mode (WITH_ACCESSIBLE) is respected
- [ ] No breaking changes to existing init flow
- [ ] Documentation is updated
- [ ] CHANGELOG.md entry is added (see format in existing entries)
- [ ] `go.mod` and `go.sum` updated with new huh version

## CHANGELOG Entry

Add this to `./docs/release-notes/change-log.md` under the "Unreleased" section:

```markdown
### Dependencies

* Upgraded `github.com/charmbracelet/huh` to latest version for enhanced terminal UI capabilities

### CLI

* The `substreams init` command now supports enhanced keyboard navigation for text inputs:
  - Use Up/Down arrow keys to navigate through previous text inputs in the current session
  - Use Tab key to auto-complete file paths based on the current directory structure
  - Multiple Tab presses cycle through matching path options
  - File path inputs now use FilePicker component (if available in huh) for better UX
```

## Key Implementation Gotchas

1. **Library Upgrade First:** Always upgrade `huh` library first and check for breaking changes before implementing features
2. **FilePicker Discovery:** If FilePicker is available, use it for file paths instead of custom completion logic
3. **Thread Safety:** The `InputHistory` is accessed by the UI goroutine, hence the mutex
4. **Path Normalization:** Always use `filepath` functions, never string manipulation for paths
5. **Cursor Position:** When programmatically setting input values, remember to move cursor to end
6. **Session Scope:** History is per-session, not persisted across command invocations
7. **Hidden Files:** macOS convention is to hide `.` files unless explicitly typed
8. **Relative Paths:** Maintain the same path style (relative vs absolute) that user started with
9. **Heuristic Detection:** File path detection uses keywords; test thoroughly with actual init flows
10. **Graceful Fallback:** If FilePicker doesn't exist or fails, fallback to enhanced text input

## Success Criteria

The feature is complete when:

1. All unit tests pass with >80% coverage
2. Manual testing confirms all three features work:
   - Up/Down for history navigation
   - Tab for path completion
   - Multiple tabs cycle through options
3. No regressions in existing `substreams init` functionality
4. Documentation is clear and includes examples
5. Code review addresses any feedback
6. PR is merged into the develop branch

## Additional Notes

### Future Enhancements (Out of Scope)

These features could be added later but are not part of this PR:

- Persistent history across sessions (stored in `~/.config/substreams/init-history`)
- Smart history: suggest previous inputs based on the current question
- Fuzzy path matching (e.g., "ctr" matches "contracts")
- Show completion options in a dropdown menu
- Ctrl+R style reverse history search

### Performance Considerations

- Input history is limited to 50 entries (see `NewInputHistory` pre-allocation)
- Path completion reads directory synchronously (could be async for large dirs)
- No disk I/O during navigation, only during path completion

### Compatibility

This implementation should work on:
- macOS (primary development platform)
- Linux (standard terminal emulators)
- Windows (may need testing for path separator handling)

Windows-specific note: The `filepath` package handles path separators correctly, but Tab completion may need additional testing on Windows terminals.
