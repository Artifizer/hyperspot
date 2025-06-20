package orm

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hypernetix/hyperspot/libs/errorx"
	"gorm.io/gorm"
)

// FilterOperator represents allowed comparison operators
type filterOperator string

const (
	OpEqual    filterOperator = "eq"
	OpLike     filterOperator = "like"
	OpNotEqual filterOperator = "ne"
	OpGreater  filterOperator = "gt"
	OpLess     filterOperator = "lt"
	OpGte      filterOperator = "ge"
	OpLte      filterOperator = "le"
	OpAnd      filterOperator = "and"
	OpOr       filterOperator = "or"
	OpParenL   filterOperator = "("
	OpParenR   filterOperator = ")"
)

// Token represents a single token in the filter query
type Token struct {
	Type  string
	Value string
}

// tokenizeFilter tokenizes the provided $filter string into individual tokens.
func tokenizeFilter(filter string) ([]Token, errorx.Error) {
	// Updated regex to include hyphens and underscores in words
	re := regexp.MustCompile(`\b(eq|ne|gt|lt|ge|le|and|or|like)\b|[\(\)]|[A-Za-z0-9_\-\.]+`)
	matches := re.FindAllString(filter, -1)

	var tokens []Token
	for _, match := range matches {
		switch strings.ToLower(match) {
		case "eq", "ne", "gt", "lt", "ge", "le", "and", "or", "like":
			tokens = append(tokens, Token{"operator", match})
		case "(", ")":
			tokens = append(tokens, Token{"paren", match})
		default:
			// Check if the token looks like an operator but isn't supported
			if isUnsupportedOperator(match) {
				return nil, errorx.NewErrBadRequest(fmt.Sprintf("unsupported operator: '%s', supported operators are: eq, ne, gt, lt, ge, le, and, or, like", match))
			}
			tokens = append(tokens, Token{"value", match})
		}
	}
	return tokens, nil
}

// isUnsupportedOperator checks if a token looks like an operator but isn't in our supported list
func isUnsupportedOperator(token string) bool {
	unsupportedOperators := []string{
		"has", "in", "contains", "startswith", "endswith",
		"any", "all", "not",
	}
	token = strings.ToLower(token)
	for _, op := range unsupportedOperators {
		if token == op {
			return true
		}
	}
	return false
}

// ASTNode represents a node in the abstract syntax tree.
type ASTNode struct {
	Left     *ASTNode
	Operator string
	Right    *ASTNode
	Value    string
}

// parseTokens converts a token list into an AST.
func parseTokens(tokens []Token) (*ASTNode, errorx.Error) {
	var stack []*ASTNode
	var opStack []Token

	precedence := map[string]int{
		"or":  1,
		"and": 2,
		"eq":  3, "ne": 3, "gt": 3, "lt": 3, "ge": 3, "le": 3, "like": 3,
		"(": 0,
	}

	for _, token := range tokens {
		switch token.Type {
		case "value":
			stack = append(stack, &ASTNode{Value: token.Value})
		case "operator":
			// Ensure there are enough operands for the operator
			for len(opStack) > 0 && precedence[opStack[len(opStack)-1].Value] >= precedence[token.Value] {
				if len(stack) < 2 {
					return nil, errorx.NewErrBadRequest(fmt.Sprintf("not enough operands for operator %s", opStack[len(opStack)-1].Value))
				}
				node := &ASTNode{
					Operator: opStack[len(opStack)-1].Value,
					Right:    stack[len(stack)-1],
					Left:     stack[len(stack)-2],
				}
				stack = stack[:len(stack)-2]
				stack = append(stack, node)
				opStack = opStack[:len(opStack)-1]
			}
			opStack = append(opStack, token)
		case "paren":
			if token.Value == "(" {
				opStack = append(opStack, token)
			} else {
				for len(opStack) > 0 && opStack[len(opStack)-1].Value != "(" {
					if len(stack) < 2 {
						return nil, errorx.NewErrBadRequest("not enough operands inside parentheses")
					}
					node := &ASTNode{
						Operator: opStack[len(opStack)-1].Value,
						Right:    stack[len(stack)-1],
						Left:     stack[len(stack)-2],
					}
					stack = stack[:len(stack)-2]
					stack = append(stack, node)
					opStack = opStack[:len(opStack)-1]
				}
				// Remove the "(" from the stack.
				if len(opStack) == 0 {
					return nil, errorx.NewErrBadRequest("mismatched parentheses in filter")
				}
				opStack = opStack[:len(opStack)-1]
			}
		}
	}

	for len(opStack) > 0 {
		if opStack[len(opStack)-1].Value == "(" {
			return nil, errorx.NewErrBadRequest("mismatched parentheses in filter")
		}
		if len(stack) < 2 {
			return nil, errorx.NewErrBadRequest(fmt.Sprintf("not enough operands for operator %s", opStack[len(opStack)-1].Value))
		}
		node := &ASTNode{
			Operator: opStack[len(opStack)-1].Value,
			Right:    stack[len(stack)-1],
			Left:     stack[len(stack)-2],
		}
		stack = stack[:len(stack)-2]
		stack = append(stack, node)
		opStack = opStack[:len(opStack)-1]
	}

	if len(stack) != 1 {
		return nil, errorx.NewErrBadRequest(fmt.Sprintf("invalid expression, %d tokens remain", len(stack)))
	}

	return stack[0], nil
}

// validateFieldName checks if a field name is in the list of allowed fields
func validateFieldName(fieldName string, allowedFields []string) bool {
	for _, allowed := range allowedFields {
		if fieldName == allowed {
			return true
		}
	}
	return false
}

// convertToGorm converts an ASTNode into GORM conditions using chained Where calls.
func convertToGorm(db *gorm.DB, node *ASTNode, allowedFields []string) (*gorm.DB, errorx.Error) {
	if node == nil {
		return db, nil
	}

	switch node.Operator {
	case "and":
		db1, err := convertToGorm(db, node.Left, allowedFields)
		if err != nil {
			return nil, err
		}
		db2, err := convertToGorm(db, node.Right, allowedFields)
		if err != nil {
			return nil, err
		}
		return db1.Where(db2), nil
	case "or":
		db1, err := convertToGorm(db, node.Left, allowedFields)
		if err != nil {
			return nil, err
		}
		db2, err := convertToGorm(db, node.Right, allowedFields)
		if err != nil {
			return nil, err
		}
		return db1.Or(db2), nil
	case "eq", "like", "ne", "gt", "lt", "ge", "le":
		if !validateFieldName(node.Left.Value, allowedFields) {
			return nil, errorx.NewErrBadRequest(fmt.Sprintf("field '%s' is not allowed in filter query, allowed fields are: %s", node.Left.Value, strings.Join(allowedFields, ", ")))
		}

		switch node.Operator {
		case "eq":
			return db.Where(fmt.Sprintf("%s = ?", node.Left.Value), node.Right.Value), nil
		case "like":
			return db.Where(fmt.Sprintf("%s LIKE ?", node.Left.Value), "%"+node.Right.Value+"%"), nil
		case "ne":
			return db.Where(fmt.Sprintf("%s != ?", node.Left.Value), node.Right.Value), nil
		case "gt":
			return db.Where(fmt.Sprintf("%s > ?", node.Left.Value), node.Right.Value), nil
		case "lt":
			return db.Where(fmt.Sprintf("%s < ?", node.Left.Value), node.Right.Value), nil
		case "ge":
			return db.Where(fmt.Sprintf("%s >= ?", node.Left.Value), node.Right.Value), nil
		case "le":
			return db.Where(fmt.Sprintf("%s <= ?", node.Left.Value), node.Right.Value), nil
		}
	}

	return db, nil
}

// QueryToGorm converts an OData-like $filter query into a GORM query.
func QueryToGorm(query string, allowedFields []string, db *gorm.DB) (*gorm.DB, errorx.Error) {
	// Return the original query if the filter is empty.
	if query == "" {
		return db, nil
	}

	tokens, err := tokenizeFilter(query)
	if err != nil {
		return nil, err
	}

	ast, err := parseTokens(tokens)
	if err != nil {
		return nil, err
	}

	dbQuery, err := convertToGorm(db, ast, allowedFields)
	if err != nil {
		return nil, err
	}

	return dbQuery, nil
}
