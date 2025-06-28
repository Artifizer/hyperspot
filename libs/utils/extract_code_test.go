package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractPythonCode(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name: "simple code",
			input: "Here's some Python code:\n" +
				"```python\n" +
				"def hello():\n" +
				"    print(\"Hello, world!\")" +
				"```",
			expected:    "def hello():\n    print(\"Hello, world!\")",
			expectError: false,
		},
		{
			name: "code without language tag",
			input: "Here's some code:\n" +
				"```\n" +
				"def hello():\n" +
				"\tprint(\"Hello, world!\")\n" +
				"```",
			expected:    "def hello():\n\tprint(\"Hello, world!\")",
			expectError: false,
		},
		{
			name: "multiple code blocks, take both",
			input: "Code block 1:\n" +
				"```python\n" +
				"def first():\n" +
				"\treturn 1\n" +
				"```\n" +
				"Code block 2:\n" +
				"```python\n" +
				"def second():\n" +
				"\treturn 2\n" +
				"```",
			expected:    "def first():\n\treturn 1\ndef second():\n\treturn 2",
			expectError: false,
		},
		{
			name:        "no code blocks",
			input:       "This is just text with no code blocks.",
			expected:    "",
			expectError: true,
		},
		{
			name: "code block with leading/trailing whitespace",
			input: "```python\n" +
				"def hello():\n" +
				"\tprint(\"Hello, world!\")\n" +
				"```",
			expected:    "def hello():\n\tprint(\"Hello, world!\")",
			expectError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ExtractPythonCode(test.input)

			if test.expectError {
				assert.Empty(t, result)
			} else {
				assert.Equal(t, test.expected, result)
			}
		})
	}
}

func TestExtractPythonException(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "simple exception",
			input: "Traceback (most recent call last):\n" +
				"  File \"test.py\", line 5, in <module>\n" +
				"    raise ValueError(\"This is an error\")\n" +
				"ValueError: This is an error",
			expected: "ValueError: This is an error",
		},
		{
			name: "no exception",
			input: `This is just some output
			with no exception traceback.`,
			expected: "",
		},
		{
			name: "exception with context",
			input: `Some output before the error

			Traceback (most recent call last):
			  File "test.py", line 10, in <module>
				result = divide(10, 0)
			  File "test.py", line 2, in divide
				return a / b
			ZeroDivisionError: division by zero

			Some output after the error`,
			expected: "ZeroDivisionError: division by zero",
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ExtractPythonException(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestCleanupString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "trim whitespace",
			input:    "  hello world  ",
			expected: "hello world",
		},
		{
			name:     "normalize newlines",
			input:    "line1\r\nline2\rline3\nline4",
			expected: "line1\nline2\nline3\nline4",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only whitespace",
			input:    "   \t\n\r  ",
			expected: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := CleanupString(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestExtractCode_EmptyOutput(t *testing.T) {
	// Test with empty input
	input := ""
	result := ExtractPythonCode(input)
	assert.Empty(t, result, "ExtractCode should return empty string for empty input")

	// Test with input that doesn't contain any code blocks
	input = "This is just plain text without any code blocks."
	result = ExtractPythonCode(input)
	assert.Empty(t, result, "ExtractCode should return empty string when no code blocks exist")

	// Test with input that has empty code blocks
	input = "Here's an empty code block: ```go\n```"
	result = ExtractPythonCode(input)
	assert.Empty(t, result, "ExtractCode should return empty string for empty code blocks")
}

func TestExtractGoCode(t *testing.T) {
	// Test basic Go code extraction
	input := "Here's some Go code:\n```go\nfunc main() {\n\tfmt.Println(\"Hello, world!\")\n}\n```"
	expected := "func main() {\n\tfmt.Println(\"Hello, world!\")\n}"
	result := ExtractGoCode(input)
	assert.Equal(t, expected, result, "ExtractGoCode should extract Go code correctly")

	// Test with multiple Go code blocks (should extract all)
	input = "First block:\n```go\nvar x = 1\n```\nSecond block:\n```go\nvar y = 2\n```"
	expected = "var x = 1\nvar y = 2"
	result = ExtractGoCode(input)
	assert.Equal(t, expected, result, "ExtractGoCode should extract and concatenate multiple Go code blocks")

	// Test with mixed language code blocks (should only extract Go)
	input = "Go code:\n```go\nvar x = 1\n```\nPython code:\n```python\nx = 1\n```"
	expected = "var x = 1"
	result = ExtractGoCode(input)
	assert.Equal(t, expected, result, "ExtractGoCode should only extract Go code blocks")

	// Test with empty input
	input = ""
	result = ExtractGoCode(input)
	assert.Empty(t, result, "ExtractGoCode should return empty string for empty input")

	// Test with no Go code blocks
	input = "Only Python code:\n```python\nx = 1\n```"
	result = ExtractGoCode(input)
	assert.Empty(t, result, "ExtractGoCode should return empty string when no Go code blocks exist")
}
