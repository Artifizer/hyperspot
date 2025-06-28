package utils

import (
	"regexp"
	"strings"
)

func ExtractCode(code string, re *regexp.Regexp) string {
	if code == "" {
		return ""
	}

	// If no markdown code blocks are found, return an error
	if !strings.Contains(code, "```") {
		return ""
	}

	// Regex to match only Python code blocks (language identifier "python" or "py")
	matches := re.FindAllStringSubmatch(code, -1)

	// If no matches were found, return an error
	if len(matches) == 0 {
		return ""
	}

	var codeBlocks []string
	for _, match := range matches {
		if len(match) > 1 {
			// Trim trailing newlines for consistent output
			codeBlocks = append(codeBlocks, strings.TrimRight(match[1], "\n"))
		}
	}

	// Join all extracted code blocks with a single newline between them
	combinedCode := strings.Join(codeBlocks, "\n")
	return combinedCode
}

var goCodeRegex = regexp.MustCompile("(?s)```(?:go|golang)?\n(.*?)```")

func ExtractGoCode(code string) string {
	return ExtractCode(code, goCodeRegex)
}

var pyCodeRegex = regexp.MustCompile("(?s)```(?:python|py)?\\n(.*?)```")

func ExtractPythonCode(code string) string {
	return ExtractCode(code, pyCodeRegex)
}

func ExtractPythonException(output string) string {
	lines := strings.Split(output, "\n")
	if len(lines) == 0 {
		return ""
	}

	// Regular expression to match Python exception patterns like "ExceptionName: message"
	exceptionPattern := regexp.MustCompile(`([A-Za-z]+Error|[A-Za-z]+Exception): .*`)

	// First check for the standard error pattern
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if exceptionPattern.MatchString(line) {
			return line
		}
	}

	// If no clear exception is found, return empty string
	return ""
}

func CleanupString(s string) string {
	// Trim whitespace
	s = strings.TrimSpace(s)

	// Normalize newlines
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	return s
}
