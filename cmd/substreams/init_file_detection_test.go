package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsFilePathInput(t *testing.T) {
	tests := []struct {
		name        string
		prompt      string
		description string
		placeholder string
		expected    bool
	}{
		{
			name:        "ABI file prompt",
			prompt:      "Enter ABI file path",
			description: "Path to the contract ABI file",
			placeholder: "/path/to/contract.json",
			expected:    true,
		},
		{
			name:        "Contract file prompt",
			prompt:      "Contract source file",
			description: "Solidity contract file",
			placeholder: "Contract.sol",
			expected:    true,
		},
		{
			name:        "Directory prompt",
			prompt:      "Output directory",
			description: "Where to save the generated files",
			placeholder: "./output",
			expected:    true,
		},
		{
			name:        "YAML config file",
			prompt:      "Configuration file",
			description: "YAML configuration file path",
			placeholder: "config.yaml",
			expected:    true,
		},
		{
			name:        "JSON file",
			prompt:      "Data file",
			description: "JSON data file",
			placeholder: "data.json",
			expected:    true,
		},
		{
			name:        "Folder path",
			prompt:      "Source folder",
			description: "Folder containing source files",
			placeholder: "./src",
			expected:    true,
		},
		{
			name:        "Regular text input",
			prompt:      "Enter your name",
			description: "Your full name",
			placeholder: "John Doe",
			expected:    false,
		},
		{
			name:        "Network name",
			prompt:      "Network",
			description: "Blockchain network name",
			placeholder: "ethereum",
			expected:    false,
		},
		{
			name:        "Contract address",
			prompt:      "Contract address",
			description: "Ethereum contract address",
			placeholder: "0x1234...",
			expected:    false,
		},
		{
			name:        "Block number",
			prompt:      "Starting block",
			description: "Block number to start from",
			placeholder: "12345",
			expected:    false,
		},
		{
			name:        "Empty inputs",
			prompt:      "",
			description: "",
			placeholder: "",
			expected:    false,
		},
		{
			name:        "Case insensitive file detection",
			prompt:      "Enter FILE Path",
			description: "PATH to the file",
			placeholder: "",
			expected:    true,
		},
		{
			name:        "Path in description only",
			prompt:      "Configuration",
			description: "Enter the path to your config",
			placeholder: "",
			expected:    true,
		},
		{
			name:        "File extension in placeholder",
			prompt:      "Config",
			description: "Configuration",
			placeholder: "example.yaml",
			expected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isFilePathInput(tt.prompt, tt.description, tt.placeholder)
			assert.Equal(t, tt.expected, result,
				"Expected %v for prompt='%s', description='%s', placeholder='%s'",
				tt.expected, tt.prompt, tt.description, tt.placeholder)
		})
	}
}

func TestIsFilePathInput_EdgeCases(t *testing.T) {
	// Test with keywords as part of larger words
	assert.False(t, isFilePathInput("Configuration", "Configure the system", "config-value"))

	// Test with file-like but not file keywords
	assert.False(t, isFilePathInput("Profile", "User profile information", "user-profile"))

	// Test with multiple file keywords
	assert.True(t, isFilePathInput("ABI file path", "Contract ABI file directory", "contract.json"))

	// Test with partial matches
	assert.True(t, isFilePathInput("filepath", "directory path", ""))
	assert.True(t, isFilePathInput("", "", "filename.sol"))

	// Test contract-specific cases
	assert.True(t, isFilePathInput("Contract file", "Select contract file", ""))
	assert.False(t, isFilePathInput("Contract address", "Enter contract address", "0x123"))
}
