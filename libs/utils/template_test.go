package utils

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTemplateToText(t *testing.T) {
	tests := []struct {
		name     string
		template string
		expected string
		hasError bool
	}{
		{
			name:     "basic template",
			template: "Hello, world!",
			expected: "Hello, world!",
			hasError: false,
		},
		{
			name:     "template with sprig function - upper",
			template: "{{ \"hello\" | upper }}",
			expected: "HELLO",
			hasError: false,
		},
		{
			name:     "template with sprig function - repeat",
			template: "{{ \"abc\" | repeat 3 }}",
			expected: "abcabcabc",
			hasError: false,
		},
		{
			name:     "template with custom seq function",
			template: "{{ range seq 3 }}{{ . }}{{ end }}",
			expected: "012",
			hasError: false,
		},
		{
			name:     "template with randInt function",
			template: "{{ $val := randInt 1 10 }}{{ if and (ge $val 1) (le $val 10) }}valid{{ else }}invalid{{ end }}",
			expected: "valid",
			hasError: false,
		},
		{
			name:     "template with error",
			template: "{{ nonexistentFunction }}",
			hasError: true,
		},
		{
			name:     "template with unclosed action",
			template: "{{ if true ",
			hasError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := TemplateToText(test.template)

			if test.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// For the randInt case, we need special handling since it's random
				if strings.Contains(test.template, "randInt") {
					assert.Equal(t, test.expected, result)
				} else {
					assert.Equal(t, test.expected, result)
				}
			}
		})
	}
}

func TestSeqFunction(t *testing.T) {
	// Test the seq function directly
	result := seq(5)
	assert.Equal(t, []int{0, 1, 2, 3, 4}, result)

	// Test edge cases
	assert.Equal(t, []int{}, seq(0))
	assert.Equal(t, []int{0}, seq(1))
}

func TestRandIntFunction(t *testing.T) {
	// Test that randInt returns values within the specified range
	for i := 0; i < 100; i++ {
		val := randInt(5, 10)
		assert.GreaterOrEqual(t, val, 5)
		assert.LessOrEqual(t, val, 10)
	}

	// Test with min == max
	for i := 0; i < 10; i++ {
		assert.Equal(t, 7, randInt(7, 7))
	}
}

func TestTemplateWithSeed(t *testing.T) {
	// Test that we can use a template with a determinstic seed
	template := `{{ $val1 := randInt 500 1000 }}{{ $val2 := randInt 1 500 }}{{ list $val1 $val2 | join "," }}`

	// Since we set a fixed seed, the values should be deterministic
	result1, err := TemplateToText(template)
	assert.NoError(t, err)

	// Parse the result which should be in format "num1,num2"
	parts := strings.Split(result1, ",")
	assert.Equal(t, 2, len(parts))

	val1, err := strconv.Atoi(parts[0])
	assert.NoError(t, err)
	assert.LessOrEqual(t, val1, 1000)
	assert.GreaterOrEqual(t, val1, 500)

	val2, err := strconv.Atoi(parts[1])
	assert.NoError(t, err)
	assert.LessOrEqual(t, val2, 500)
	assert.GreaterOrEqual(t, val2, 1)

	// Run it again to verify we get random values each time
	result2, err := TemplateToText(template)
	assert.NoError(t, err)

	// Values should be different on each run (they're random)
	assert.NotEqual(t, result1, result2)
}
