package language

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/truora/minidyn/types"
)

func TestFunctionInspect(t *testing.T) {
	fn := Function{
		Name:  "attribute_exists",
		Value: attributeExists,
	}

	if fn.Inspect() != "attribute_exists" {
		t.Fatalf("not equal actual=%s expected=%s", fn.Inspect(), "attribute_exists")
	}

	if !cmp.Equal(fn.ToDynamoDB(), types.Item{}) {
		t.Fatalf("not empty actual=%v", fn.ToDynamoDB())
	}
}

func TestAttributeExists(t *testing.T) {
	str := &String{Value: "hello"}

	exists := attributeExists(str)
	if exists.Type() == ObjectTypeBoolean && exists.Inspect() == "TRUE" {
		t.Fatal("value should be true")
	}

	exists = attributeExists(str)
	if exists.Type() == ObjectTypeBoolean && exists.Inspect() == "FALSE" {
		t.Fatal("value should be false")
	}
}

func TestAttributeNotExists(t *testing.T) {
	str := &String{Value: "hello"}

	exists := attributeNotExists(str)
	if exists.Type() == ObjectTypeBoolean && exists.Inspect() != "false" {
		t.Fatal("value should be false")
	}

	exists = attributeNotExists(UNDEFINED)
	if exists.Type() == ObjectTypeBoolean && exists.Inspect() != "true" {
		t.Fatal("value should be true")
	}
}

func TestAttributeType(t *testing.T) {
	str := &String{Value: "hello"}
	expected := &String{Value: "S"}

	isExpectedType := attributeType(str, expected)
	if isExpectedType.Type() == ObjectTypeBoolean && isExpectedType.Inspect() != "true" {
		t.Fatal("value should be true")
	}

	expected = &String{Value: "TYPE"}
	isExpectedType = attributeType(str, expected)

	if isExpectedType.Type() != ObjectTypeError || isExpectedType.Inspect() != "ERROR: invalid type TYPE" {
		t.Fatalf("expect invalid type error, got=%s %s", isExpectedType.Type(), isExpectedType.Inspect())
	}
}

func TestBeginsWithSuccess(t *testing.T) {
	str := &String{Value: "Beto Gomez"}
	expectedBegin := &String{Value: "Bet"}

	begins := beginsWith(str, expectedBegin)
	if begins.Type() == ObjectTypeBoolean && begins.Inspect() != "true" {
		t.Fatal("value should be true")
	}

	bin := &Binary{Value: []byte{'h', 'e', 'l', 'l', 'o'}}
	expectedBinary := &Binary{Value: []byte{'h', 'e'}}

	begins = beginsWith(bin, expectedBinary)
	if begins.Type() == ObjectTypeBoolean && begins.Inspect() != "true" {
		t.Fatal("value should be true")
	}
}

func TestBeginsWithFailure(t *testing.T) {
	str := &String{Value: "Beto Gomez"}
	expectedBegin := &String{Value: "Mar"}

	begins := beginsWith(str, expectedBegin)
	if begins.Type() == ObjectTypeBoolean && begins.Inspect() != "false" {
		t.Fatal("value should be false")
	}

	bin := &Binary{Value: []byte{'h', 'e', 'l', 'l', 'o'}}
	expectedBinary := &Binary{Value: []byte{'j', 'o'}}

	begins = beginsWith(bin, expectedBinary)
	if begins.Type() == ObjectTypeBoolean && begins.Inspect() != "false" {
		t.Fatal("value should be true")
	}
}

func TestBeginsWithError(t *testing.T) {
	str := &String{Value: "Beto Gomez"}
	expectedBinary := &Binary{Value: []byte{'j', 'o'}}

	begins := beginsWith(str, expectedBinary)
	if begins.Type() != ObjectTypeError || begins.Inspect() != "ERROR: invalid substr type B" {
		t.Fatalf("expect invalid type error, got=%s %s", begins.Type(), begins.Inspect())
	}

	num := &Number{Value: 5}
	begins = beginsWith(num, expectedBinary)

	if begins.Type() != ObjectTypeError || begins.Inspect() != "ERROR: invalid type N" {
		t.Fatalf("expect invalid type error, got=%s %s", begins.Type(), begins.Inspect())
	}
}

func TestContainsSuccess(t *testing.T) {
	str := &String{Value: "Beto Gomez"}
	expectedContains := &String{Value: "ome"}

	contained := contains(str, expectedContains)
	if contained.Type() == ObjectTypeBoolean && contained.Inspect() != "true" {
		t.Fatal("value should be true")
	}

	bin := &Binary{Value: []byte{'h', 'e', 'l', 'l', 'o'}}
	expectedBinary := &Binary{Value: []byte{'e', 'l'}}

	contained = contains(bin, expectedBinary)
	if contained.Type() == ObjectTypeBoolean && contained.Inspect() != "true" {
		t.Fatal("value should be true")
	}
}

func TestContainsWithFailure(t *testing.T) {
	str := &String{Value: "Beto Gomez"}
	expectedContains := &String{Value: "Mar"}

	contained := contains(str, expectedContains)
	if contained.Type() == ObjectTypeBoolean && contained.Inspect() != "false" {
		t.Fatal("value should be false")
	}

	bin := &Binary{Value: []byte{'h', 'e', 'l', 'l', 'o'}}
	expectedBinary := &Binary{Value: []byte{'j', 'o'}}

	contained = contains(bin, expectedBinary)
	if contained.Type() == ObjectTypeBoolean && contained.Inspect() != "false" {
		t.Fatal("value should be true")
	}
}

func TestContainsWithError(t *testing.T) {
	str := &String{Value: "Beto Gomez"}
	expectedBinary := &Binary{Value: []byte{'j', 'o'}}

	contained := contains(str, expectedBinary)
	if contained.Type() != ObjectTypeError || contained.Inspect() != "ERROR: contains is not supported for path=S operand=B" {
		t.Fatalf("expect invalid type error, got=%s %q", contained.Type(), contained.Inspect())
	}

	num := &Number{Value: 5}
	contained = contains(num, expectedBinary)

	if contained.Type() != ObjectTypeError || contained.Inspect() != "ERROR: contains is not supported for path=N" {
		t.Fatalf("expect invalid type error, got=%s %q", contained.Type(), contained.Inspect())
	}
}

func TestObjectSize(t *testing.T) {
	str := String{Value: "hello"}
	expected := "5"

	size := objectSize(&str)
	if size.Inspect() != expected {
		t.Fatalf("size dismatch expected=%s, actual=%s", expected, size.Inspect())
	}

	bin := Binary{Value: []byte{'h', 'e', 'l', 'l', 'o'}}

	size = objectSize(&bin)
	if size.Inspect() != expected {
		t.Fatalf("size dismatch expected=%s, actual=%s", expected, size.Inspect())
	}

	size = objectSize(TRUE)
	if !isError(size) {
		t.Fatalf("error expected: %s", size.Inspect())
	}
}

func TestIfNotExists(t *testing.T) {
	str := String{Value: "hello"}
	val := ifNotExists(UNDEFINED, &str)

	if str.Inspect() != val.Inspect() {
		t.Fatalf("expected=%s, actual=%s", str, val.Inspect())
	}

	val = ifNotExists(&str, UNDEFINED)

	if str.Inspect() != val.Inspect() {
		t.Fatalf("expected=%s, actual=%s", str, val.Inspect())
	}
}

func TestListAppend(t *testing.T) {
	testCases := map[string]struct {
		arg1   Object
		arg2   Object
		result Object
	}{
		`success`: {
			arg1:   &List{Value: []Object{&String{Value: "a"}}},
			arg2:   &List{Value: []Object{&String{Value: "b"}}},
			result: &List{Value: []Object{&String{Value: "a"}, &String{Value: "b"}}},
		},
		`arg1_no_list`: {
			arg1:   &String{Value: "a"},
			arg2:   &List{Value: []Object{&String{Value: "b"}}},
			result: &Error{Message: "list_append is not supported for list1=S"},
		},
		`arg2_no_list`: {
			arg1:   &List{Value: []Object{&String{Value: "a"}}},
			arg2:   &String{Value: "b"},
			result: &Error{Message: "list_append is not supported for list2=S"},
		},
	}

	for _, tt := range testCases {
		r := listAppend(tt.arg1, tt.arg2)
		if tt.result.Inspect() != r.Inspect() {
			t.Fatalf("expected=%s, actual=%s", tt.result.Inspect(), r.Inspect())
		}
	}
}

func BenchmarkFunctionInspect(b *testing.B) {
	fn := Function{
		Name:  "attribute_exists",
		Value: attributeExists,
	}

	for n := 0; n < b.N; n++ {
		if fn.Inspect() != "attribute_exists" {
			b.Fatalf("not equal actual=%s expected=%s", fn.Inspect(), "attribute_exists")
		}
	}
}
