package orm

import (
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestTokenizeFilter tests the tokenizeFilter function with various inputs
func TestTokenizeFilter(t *testing.T) {
	tests := []struct {
		name     string
		filter   string
		expected []Token
		wantErr  bool
		errMsg   string
	}{
		{
			name:   "basic filter",
			filter: "name eq John and age gt 30",
			expected: []Token{
				{Type: "value", Value: "name"},
				{Type: "operator", Value: "eq"},
				{Type: "value", Value: "John"},
				{Type: "operator", Value: "and"},
				{Type: "value", Value: "age"},
				{Type: "operator", Value: "gt"},
				{Type: "value", Value: "30"},
			},
		},
		{
			name:   "with parentheses",
			filter: "(status eq active)",
			expected: []Token{
				{Type: "paren", Value: "("},
				{Type: "value", Value: "status"},
				{Type: "operator", Value: "eq"},
				{Type: "value", Value: "active"},
				{Type: "paren", Value: ")"},
			},
		},
		{
			name:    "unsupported has operator",
			filter:  "tags has mobile",
			wantErr: true,
			errMsg:  "unsupported operator: 'has'",
		},
		{
			name:    "unsupported in operator",
			filter:  "status in (active, waiting)",
			wantErr: true,
			errMsg:  "unsupported operator: 'in'",
		},
		{
			name:    "unsupported contains operator",
			filter:  "name contains test",
			wantErr: true,
			errMsg:  "unsupported operator: 'contains'",
		},
		{
			name:   "with hyphens in value",
			filter: "name like deepseek-r1-distill",
			expected: []Token{
				{Type: "value", Value: "name"},
				{Type: "operator", Value: "like"},
				{Type: "value", Value: "deepseek-r1-distill"},
			},
		},
		{
			name:   "with underscore in field name",
			filter: "model_name eq test",
			expected: []Token{
				{Type: "value", Value: "model_name"},
				{Type: "operator", Value: "eq"},
				{Type: "value", Value: "test"},
			},
		},
		{
			name:   "with hyphens and underscores",
			filter: "model_type eq llama-2_base",
			expected: []Token{
				{Type: "value", Value: "model_type"},
				{Type: "operator", Value: "eq"},
				{Type: "value", Value: "llama-2_base"},
			},
		},
		{
			name:   "with hyphen and decimal point",
			filter: "name like qwen3-1.7",
			expected: []Token{
				{Type: "value", Value: "name"},
				{Type: "operator", Value: "like"},
				{Type: "value", Value: "qwen3-1.7"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, err := tokenizeFilter(tt.filter)
			if tt.wantErr {
				if err == nil {
					t.Errorf("tokenizeFilter() expected error but got none")
					return
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("tokenizeFilter() error = %v, want error containing %v", err, tt.errMsg)
				}
				return
			}
			if err != nil {
				t.Errorf("tokenizeFilter() unexpected error = %v", err)
				return
			}
			if len(tokens) != len(tt.expected) {
				t.Errorf("Expected %d tokens, got %d tokens", len(tt.expected), len(tokens))
				return
			}
			for i, expTok := range tt.expected {
				if tokens[i].Type != expTok.Type || tokens[i].Value != expTok.Value {
					t.Errorf("Token %d mismatch. Expected %+v, got %+v", i, expTok, tokens[i])
				}
			}
		})
	}
}

// TestParseTokens tests parseTokens with various token combinations
func TestParseTokens(t *testing.T) {
	tests := []struct {
		name    string
		tokens  []Token
		wantErr bool
	}{
		{
			name: "simple expression",
			tokens: []Token{
				{Type: "value", Value: "name"},
				{Type: "operator", Value: "eq"},
				{Type: "value", Value: "John"},
			},
			wantErr: false,
		},
		{
			name: "invalid operator placement",
			tokens: []Token{
				{Type: "operator", Value: "and"},
				{Type: "value", Value: "name"},
			},
			wantErr: true,
		},
		{
			name: "mismatched parentheses",
			tokens: []Token{
				{Type: "paren", Value: "("},
				{Type: "value", Value: "name"},
				{Type: "operator", Value: "eq"},
				{Type: "value", Value: "John"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTokens(tt.tokens)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTokens() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Dummy model used for generating SQL in QueryToGorm tests
type Dummy struct {
	Name   string
	Age    int
	Status string
}

// TestQueryToGorm tests the QueryToGorm function with allowedFields validation
func TestQueryToGorm(t *testing.T) {
	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DryRun: true})
	if err != nil {
		t.Fatalf("Failed to connect to test DB: %v", err)
	}

	allowedFields := []string{"name", "age", "status"}

	tests := []struct {
		name         string
		filter       string
		allowFields  []string
		expectSQL    []string
		expectError  bool
		errorMessage string
	}{
		{
			name:        "basic allowed field",
			filter:      "name eq John",
			allowFields: allowedFields,
			expectSQL:   []string{"name = ?"},
		},
		{
			name:         "disallowed field",
			filter:       "password eq secret",
			allowFields:  allowedFields,
			expectError:  true,
			errorMessage: "field 'password' is not allowed in filter query",
		},
		{
			name:        "multiple allowed fields",
			filter:      "age gt 20 and status eq active",
			allowFields: allowedFields,
			expectSQL:   []string{"age > ?", "status = ?"},
		},
		{
			name:        "complex query with allowed fields",
			filter:      "(name like John or age gt 25) and status eq active",
			allowFields: allowedFields,
			expectSQL:   []string{"name LIKE ?", "age > ?", "status = ?"},
		},
		{
			name:         "one disallowed field in complex query",
			filter:       "name eq John and role eq admin",
			allowFields:  allowedFields,
			expectError:  true,
			errorMessage: "field 'role' is not allowed in filter query",
		},
		{
			name:        "like operator with hyphen and dot in value",
			filter:      "name like qwen3-1.7",
			allowFields: allowedFields,
			expectSQL:   []string{"name LIKE ?"},
		},
		{
			name:        "complex query with like operator and special characters",
			filter:      "(name like qwen3-1.7 or status eq active) and age gt 25",
			allowFields: allowedFields,
			expectSQL:   []string{"name LIKE ?", "status = ?", "age > ?"},
		},
		{
			name:        "empty filter",
			filter:      "",
			allowFields: allowedFields,
			expectSQL:   []string{},
		},
		{
			name:         "nil allowed fields",
			filter:       "name eq John",
			allowFields:  nil,
			expectError:  true,
			errorMessage: "field 'name' is not allowed in filter query",
		},
		{
			name:         "empty allowed fields",
			filter:       "name eq John",
			allowFields:  []string{},
			expectError:  true,
			errorMessage: "field 'name' is not allowed in filter query",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := QueryToGorm(tt.filter, tt.allowFields, testDB.Model(&Dummy{}))

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}
				if !strings.Contains(err.Error(), tt.errorMessage) {
					t.Errorf("Expected error message '%s', got '%s'", tt.errorMessage, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Execute dummy query to generate SQL
			var d []Dummy
			q.Find(&d)

			sql := q.Statement.SQL.String()

			// For empty filter, verify no WHERE clause
			if tt.filter == "" {
				if strings.Contains(sql, "WHERE") {
					t.Errorf("Empty filter should not generate WHERE clause, got: %s", sql)
				}
				return
			}

			// Verify expected SQL fragments
			for _, expectedSQL := range tt.expectSQL {
				if !strings.Contains(sql, expectedSQL) {
					t.Errorf("Expected SQL to contain '%s', got: %s", expectedSQL, sql)
				}
			}
		})
	}
}

// TestValidateFieldName tests the field validation function
func TestValidateFieldName(t *testing.T) {
	tests := []struct {
		name        string
		fieldName   string
		allowFields []string
		expectValid bool
	}{
		{
			name:        "allowed field",
			fieldName:   "name",
			allowFields: []string{"name", "age", "status"},
			expectValid: true,
		},
		{
			name:        "disallowed field",
			fieldName:   "password",
			allowFields: []string{"name", "age", "status"},
			expectValid: false,
		},
		{
			name:        "empty allowed fields",
			fieldName:   "name",
			allowFields: []string{},
			expectValid: false,
		},
		{
			name:        "nil allowed fields",
			fieldName:   "name",
			allowFields: nil,
			expectValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateFieldName(tt.fieldName, tt.allowFields)
			if result != tt.expectValid {
				t.Errorf("validateFieldName() = %v, want %v", result, tt.expectValid)
			}
		})
	}
}

// TestUnsupportedOperators tests the isUnsupportedOperator function
func TestUnsupportedOperators(t *testing.T) {
	tests := []struct {
		name     string
		operator string
		want     bool
	}{
		{"has operator", "has", true},
		{"in operator", "in", true},
		{"contains operator", "contains", true},
		{"startswith operator", "startswith", true},
		{"endswith operator", "endswith", true},
		{"any operator", "any", true},
		{"all operator", "all", true},
		{"not operator", "not", true},
		{"supported eq operator", "eq", false},
		{"supported like operator", "like", false},
		{"regular word", "hello", false},
		{"mixed case operator", "HaS", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUnsupportedOperator(tt.operator); got != tt.want {
				t.Errorf("isUnsupportedOperator(%q) = %v, want %v", tt.operator, got, tt.want)
			}
		})
	}
}
