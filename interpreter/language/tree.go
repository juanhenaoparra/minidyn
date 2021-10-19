package language

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strings"
)

var (
	// ErrMissingExpression is a tree error
	ErrMissingExpression = errors.New("missing expression")
	// ErrMissingTokenLiteral is a tree error
	ErrMissingTokenLiteral = errors.New("missing token literal")
	// ErrMissingParentID is a tree error
	ErrMissingParentID = errors.New("missing parent id")
)

func init() {
	rand.Seed(100)
}

// Tree represents a tree of nodes
type Tree struct {
	Root       *TreeNode
	Expression Expression
}

// NewTree creates and fill a new tree based in a expression
func NewTree(expression Expression) (*Tree, error) {
	if expression.String() == "" {
		return nil, ErrMissingExpression
	}

	tree := &Tree{}

	tree.Root = &TreeNode{
		ID:         "ROOT",
		Expression: expression,
		TokenType:  TokenType(expression.TokenLiteral()),
	}

	err := tree.Root.Fill()
	if err != nil {
		return nil, err
	}

	return tree, nil
}

// Walk returns the whole tree in a string format
func (tn *TreeNode) Walk(s *strings.Builder) string {
	if s == nil {
		s = &strings.Builder{}
	}

	s.WriteString("CHILDOF: ")
	s.WriteString(tn.ParentID)
	s.WriteString(" ID: ")
	s.WriteString(tn.ID)
	s.WriteString(" --- ")
	s.WriteString(tn.Expression.String())
	s.WriteString(" --- ")
	s.WriteString(string(tn.TokenType))
	s.WriteString(",\n")

	for _, c := range tn.Children {
		c.Walk(s)
	}

	return s.String()
}

// Fill iterates over the expression and fill the node children
func (tn *TreeNode) Fill() error {
	if tn.Expression.String() == "" {
		return nil
	}

	tl := tn.Expression.TokenLiteral()
	if tl == "" {
		return ErrMissingTokenLiteral
	}

	childStrings := strings.Split(tn.Expression.String(), tl) // FIXME: first group by () and the split
	childStrings = clearChildrenList(childStrings)

	if len(tn.Children) == 0 {
		tn.Children = make([]*TreeNode, 0, len(childStrings))
	}

	for _, cs := range childStrings {
		le := NewLexer(cs)
		par := NewParser(le)

		if par.curToken.Type == IDENT && par.peekToken.Type == EOF {
			childExpression := &Identifier{
				Token: par.curToken,
				Value: par.curToken.Literal,
			}

			newChild, err := NewTreeNode(tn.ID, childExpression, childExpression.Token.Type)
			if err != nil {
				return err
			}

			tn.Children = append(tn.Children, newChild)
			continue
		}

		childExpression := par.ParseConditionalExpression()
		childExpressionTokenType := TokenType(childExpression.Expression.TokenLiteral())

		newChild, err := NewTreeNode(tn.ID, childExpression.Expression, childExpressionTokenType)
		if err != nil {
			return err
		}

		// TODO: use goroutines
		err = newChild.Fill()
		if err != nil {
			return err
		}

		tn.Children = append(tn.Children, newChild)
	}

	return nil
}

func clearChildrenList(childrenRaw []string) []string {
	cleanedChildren := make([]string, 0, len(childrenRaw))

	for _, child := range childrenRaw {
		cleanedChild := deleteUnclosedParenthesis(child)
		cleanedChild = strings.ReplaceAll(cleanedChild, " ", "")

		if cleanedChild == "" {
			continue
		}

		cleanedChildren = append(cleanedChildren, cleanedChild)
	}

	return cleanedChildren
}

func deleteUnclosedParenthesis(input string) string {
	openParenthesisCount := strings.Count(input, "(")
	closeParenthesisCount := strings.Count(input, ")")

	if openParenthesisCount == closeParenthesisCount {
		return input
	}

	toTrim := "("
	count := int(math.Abs(float64(openParenthesisCount) - float64(closeParenthesisCount)))

	if openParenthesisCount < closeParenthesisCount {
		toTrim = ")"
	}

	output := strings.Replace(input, toTrim, "", count)

	return output
}

// TreeNode is a node that contents
type TreeNode struct {
	ID         string
	TokenType  TokenType
	ParentID   string
	Children   []*TreeNode
	Expression Expression
}

// condition-expression ::=
// 		operand comparator operand
// 	| operand BETWEEN operand AND operand
// 	| operand IN ( operand (',' operand (, ...) ))
// 	| function
// 	| condition AND condition
// 	| condition OR condition
// 	| NOT condition
// 	| ( condition )

// comparator ::=
// 	=
// 	| <>
// 	| <
// 	| <=
// 	| >
// 	| >=

// function ::=
// 	attribute_exists (path)
// 	| attribute_not_exists (path)
// 	| attribute_type (path, type)
// 	| begins_with (path, substr)
// 	| contains (path, operand)
// 	| size (path)

func randString() string {
	num := rand.Intn(1000) // #nosec

	return fmt.Sprintf("TN%d", num)
}

// NewTreeNode creates a new tree node with an expression
func NewTreeNode(parentID string, expr Expression, tokenType TokenType) (*TreeNode, error) {
	if parentID == "" {
		return nil, ErrMissingParentID
	}

	if expr == nil {
		return nil, ErrMissingExpression
	}

	return &TreeNode{
		ID:         randString(),
		ParentID:   parentID,
		Expression: expr,
		TokenType:  tokenType,
	}, nil
}

// IsAComparatorOrFunction checks if a node is a comparator or a function
func (tn *TreeNode) IsAComparatorOrFunction() bool {
	precedence, ok := precedences[tn.TokenType]
	if !ok {
		return false
	}

	switch precedence {
	case precedenceValueEqualComparators:
		return true
	case precedenceValueComparators:
		return true
	case precedenceValueBetweenComparator:
		return true
	case precedenceValueCall:
		return true
	}

	return false
}

// IsAValidCondition checks if a node is a valid condition
func (tn *TreeNode) IsAValidCondition() bool {
	if !tn.IsAComparatorOrFunction() {
		return false
	}

	if len(tn.Children) != 2 {
		return false
	}

	for _, child := range tn.Children {
		if child.TokenType != IDENT {
			return false
		}
	}

	return true
}
