package language

import (
	"fmt"
	"testing"
)

func TestNewTree(t *testing.T) {
	input := "NOT :b = :false_value AND :false_value"

	le := NewLexer(input)
	pe := NewParser(le)

	ce := pe.ParseConditionalExpression()

	fmt.Println(ce.String())

	tree, err := NewTree(ce.Expression)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("SUCCESS")

	fmt.Println(tree.Root.Walk(nil))

	t.Fatal()
}
