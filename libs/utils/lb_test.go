package utils

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewLineBuffer(t *testing.T) {
	t.Run("BasicWriteAndRead", func(t *testing.T) {
		ctx := context.Background()
		lb := NewLineBuffer(ctx)

		// Write some data
		_, err := lb.Write([]byte("line1\nline2\nline3\n"))
		assert.NoError(t, err, "Write should not fail")

		// Close to signal end of data
		err = lb.Close()
		assert.NoError(t, err, "Close should not fail")

		// Get lines
		lines := lb.CloseAndGetLines()
		assert.Equal(t, 3, len(lines), "Should have 3 lines")
		assert.Equal(t, "line1", lines[0])
		assert.Equal(t, "line2", lines[1])
		assert.Equal(t, "line3", lines[2])
	})

	// Test multiple writes
	t.Run("MultipleWrites", func(t *testing.T) {
		ctx := context.Background()
		lb := NewLineBuffer(ctx)

		// Write data in chunks
		_, err := lb.Write([]byte("first"))
		assert.NoError(t, err)

		_, err = lb.Write([]byte(" part\nsecond"))
		assert.NoError(t, err)

		_, err = lb.Write([]byte(" line\nthird line\n"))
		assert.NoError(t, err)

		lines := lb.CloseAndGetLines()
		assert.Equal(t, 3, len(lines), "Should have 3 lines")
		assert.Equal(t, "first part", lines[0])
		assert.Equal(t, "second line", lines[1])
		assert.Equal(t, "third line", lines[2])
	})

	// Test empty input
	t.Run("EmptyInput", func(t *testing.T) {
		ctx := context.Background()
		lb := NewLineBuffer(ctx)

		lines := lb.CloseAndGetLines()
		assert.Equal(t, 0, len(lines), "Should have no lines")
	})

	// Test single line without newline
	t.Run("NoTrailingNewline", func(t *testing.T) {
		ctx := context.Background()
		lb := NewLineBuffer(ctx)

		_, err := lb.Write([]byte("single line without newline"))
		assert.NoError(t, err)

		lines := lb.CloseAndGetLines()
		assert.Equal(t, 1, len(lines), "Should have 1 line")
		assert.Equal(t, "single line without newline", lines[0])
	})

	// Test empty lines
	t.Run("EmptyLines", func(t *testing.T) {
		ctx := context.Background()
		lb := NewLineBuffer(ctx)

		_, err := lb.Write([]byte("line1\n\nline3\n\n"))
		assert.NoError(t, err)

		lines := lb.CloseAndGetLines()
		assert.Equal(t, 4, len(lines), "Should have 4 lines including empty ones")
		assert.Equal(t, "line1", lines[0])
		assert.Equal(t, "", lines[1])
		assert.Equal(t, "line3", lines[2])
		assert.Equal(t, "", lines[3])
	})

	// Test context cancellation
	t.Run("ContextCancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		lb := NewLineBuffer(ctx)

		// Write some data
		_, err := lb.Write([]byte("line1\nline2\n"))
		assert.NoError(t, err)

		// Cancel context
		cancel()

		// Small delay to let goroutine process cancellation
		time.Sleep(10 * time.Millisecond)

		// Lines should still return what was written before cancellation
		lines := lb.CloseAndGetLines()
		assert.Equal(t, 2, len(lines), "Should have 2 lines even after cancellation")
		assert.Equal(t, "line1", lines[0])
		assert.Equal(t, "line2", lines[1])
	})

	// Test concurrent writes
	t.Run("ConcurrentWrites", func(t *testing.T) {
		ctx := context.Background()
		lb := NewLineBuffer(ctx)

		var wg sync.WaitGroup
		numGoroutines := 5
		linesPerGoroutine := 10

		// Start multiple goroutines writing lines
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < linesPerGoroutine; j++ {
					line := fmt.Sprintf("goroutine%d-line%d\n", id, j)
					_, err := lb.Write([]byte(line))
					assert.NoError(t, err)
				}
			}(i)
		}

		wg.Wait()

		lines := lb.CloseAndGetLines()
		assert.Equal(t, numGoroutines*linesPerGoroutine, len(lines),
			"Should have all lines from all goroutines")

		// Check that all expected lines are present (order may vary)
		lineSet := make(map[string]bool)
		for _, line := range lines {
			lineSet[line] = true
		}

		for i := 0; i < numGoroutines; i++ {
			for j := 0; j < linesPerGoroutine; j++ {
				expectedLine := fmt.Sprintf("goroutine%d-line%d", i, j)
				assert.True(t, lineSet[expectedLine],
					"Should contain line: %s", expectedLine)
			}
		}
	})

	// Test timeout scenario
	t.Run("TimeoutScenario", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		lb := NewLineBuffer(ctx)

		// Write some initial data
		_, err := lb.Write([]byte("line1\nline2\n"))
		assert.NoError(t, err)

		// Wait for timeout
		time.Sleep(100 * time.Millisecond)

		// Should still be able to get the lines that were written
		lines := lb.CloseAndGetLines()
		assert.Equal(t, 2, len(lines))
		assert.Equal(t, "line1", lines[0])
		assert.Equal(t, "line2", lines[1])
	})

	// Test large data
	t.Run("LargeData", func(t *testing.T) {
		ctx := context.Background()
		lb := NewLineBuffer(ctx)

		// Write many lines
		var expected []string
		var data strings.Builder
		for i := 0; i < 1000; i++ {
			line := fmt.Sprintf("line number %d", i)
			expected = append(expected, line)
			data.WriteString(line + "\n")
		}

		_, err := lb.Write([]byte(data.String()))
		assert.NoError(t, err)

		lines := lb.CloseAndGetLines()
		assert.Equal(t, len(expected), len(lines))
		for i, expectedLine := range expected {
			assert.Equal(t, expectedLine, lines[i], "Line %d should match", i)
		}
	})
}
