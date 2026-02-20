package ir

import "testing"

func TestParseCELSimpleComparison(t *testing.T) {
	expr, err := ParseCELExpr("this >= 1")
	if err != nil {
		t.Fatal(err)
	}
	if expr.Kind != ExprBinary {
		t.Fatalf("expected ExprBinary, got %v", expr.Kind)
	}
	if expr.Op != ">=" {
		t.Errorf("op: got %q, want %q", expr.Op, ">=")
	}
	if expr.Left.Kind != ExprIdent || expr.Left.Ident != "this" {
		t.Errorf("left: expected ident 'this', got %+v", expr.Left)
	}
	if expr.Right.Kind != ExprLiteral || expr.Right.Literal.Int == nil || *expr.Right.Literal.Int != 1 {
		t.Errorf("right: expected literal 1, got %+v", expr.Right)
	}
}

func TestParseCELCompound(t *testing.T) {
	expr, err := ParseCELExpr("this >= 0 && this <= 7")
	if err != nil {
		t.Fatal(err)
	}
	if expr.Kind != ExprBinary {
		t.Fatalf("expected ExprBinary, got %v", expr.Kind)
	}
	if expr.Op != "&&" {
		t.Errorf("op: got %q, want %q", expr.Op, "&&")
	}
	if expr.Left.Kind != ExprBinary || expr.Left.Op != ">=" {
		t.Errorf("left: expected >= comparison, got %+v", expr.Left)
	}
	if expr.Right.Kind != ExprBinary || expr.Right.Op != "<=" {
		t.Errorf("right: expected <= comparison, got %+v", expr.Right)
	}
}

func TestParseCELFieldRef(t *testing.T) {
	expr, err := ParseCELExpr("this == version")
	if err != nil {
		t.Fatal(err)
	}
	if expr.Kind != ExprBinary {
		t.Fatalf("expected ExprBinary, got %v", expr.Kind)
	}
	if expr.Op != "==" {
		t.Errorf("op: got %q, want %q", expr.Op, "==")
	}
	if expr.Left.Kind != ExprIdent || expr.Left.Ident != "this" {
		t.Errorf("left: expected ident 'this', got %+v", expr.Left)
	}
	if expr.Right.Kind != ExprIdent || expr.Right.Ident != "version" {
		t.Errorf("right: expected ident 'version', got %+v", expr.Right)
	}
}

func TestParseCELHexLiteral(t *testing.T) {
	expr, err := ParseCELExpr("this == 0x89504E47")
	if err != nil {
		t.Fatal(err)
	}
	if expr.Kind != ExprBinary {
		t.Fatalf("expected ExprBinary, got %v", expr.Kind)
	}
	if expr.Right.Kind != ExprLiteral || expr.Right.Literal.Int == nil {
		t.Fatalf("right: expected int literal, got %+v", expr.Right)
	}
	if *expr.Right.Literal.Int != 2303741511 {
		t.Errorf("right value: got %d, want %d", *expr.Right.Literal.Int, 2303741511)
	}
}

func TestParseCELInvalid(t *testing.T) {
	_, err := ParseCELExpr("!!!")
	if err == nil {
		t.Fatal("expected error for invalid CEL expression")
	}
}

func TestParseCELSize(t *testing.T) {
	expr, err := ParseCELExpr("size(this) > 0")
	if err != nil {
		t.Fatal(err)
	}
	if expr.Kind != ExprBinary {
		t.Fatalf("expected ExprBinary, got %v", expr.Kind)
	}
	if expr.Op != ">" {
		t.Errorf("op: got %q, want %q", expr.Op, ">")
	}
	call := expr.Left
	if call.Kind != ExprCall {
		t.Fatalf("left: expected ExprCall, got %v", call.Kind)
	}
	if call.FuncName != "size" {
		t.Errorf("func name: got %q, want %q", call.FuncName, "size")
	}
	if len(call.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(call.Args))
	}
	if call.Args[0].Kind != ExprIdent || call.Args[0].Ident != "this" {
		t.Errorf("arg: expected ident 'this', got %+v", call.Args[0])
	}
}

func TestContainsThis(t *testing.T) {
	exprWithThis, err := ParseCELExpr("this >= 1")
	if err != nil {
		t.Fatal(err)
	}
	if !containsThis(exprWithThis) {
		t.Error("expected containsThis to return true for 'this >= 1'")
	}

	exprWithoutThis, err := ParseCELExpr("x >= 1")
	if err != nil {
		t.Fatal(err)
	}
	if containsThis(exprWithoutThis) {
		t.Error("expected containsThis to return false for 'x >= 1'")
	}
}
