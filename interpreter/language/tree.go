package language

// Tree represents a tree of nodes
type Tree struct {
	Root *TreeNode
}

// TreeNode is a node that contents
type TreeNode struct {
	TokenType TokenType
	Children  []TreeNode
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

func (tn *TreeNode) IsAComparator() bool {
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
	}

	return false
}

func (tn *TreeNode) IsAValidCondition() bool {
	if !tn.IsAComparator() {
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
