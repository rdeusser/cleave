package ir

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
)

var (
	hexPattern = regexp.MustCompile(`0x[0-9a-fA-F]+`)

	binaryOps = map[string]string{
		"_==_": "==",
		"_!=_": "!=",
		"_<_":  "<",
		"_<=_": "<=",
		"_>_":  ">",
		"_>=_": ">=",
		"_&&_": "&&",
		"_||_": "||",
		"_+_":  "+",
		"_-_":  "-",
		"_*_":  "*",
		"_/_":  "/",
		"_%_":  "%",
	}

	unaryOps = map[string]string{
		"!_": "!",
		"-_": "-",
	}
)

type ExprNodeKind int

const (
	ExprLiteral ExprNodeKind = iota
	ExprIdent
	ExprBinary
	ExprUnary
	ExprSelect
	ExprCall
)

type ExprNode struct {
	Kind     ExprNodeKind
	Literal  *LiteralValue
	Ident    string
	Op       string
	Left     *ExprNode
	Right    *ExprNode
	Operand  *ExprNode
	Field    string
	FuncName string
	Args     []*ExprNode
	IsMember bool
}

type LiteralValue struct {
	Int   *int64
	Bool  *bool
	Str   *string
	Float *float64
}

func ParseCELExpr(source string) (*ExprNode, error) {
	preprocessed := hexPattern.ReplaceAllStringFunc(source, func(s string) string {
		val, err := strconv.ParseInt(s, 0, 64)
		if err != nil {
			return s
		}
		return strconv.FormatInt(val, 10)
	})

	env, err := cel.NewEnv()
	if err != nil {
		return nil, fmt.Errorf("cel env: %w", err)
	}

	ast, iss := env.Parse(preprocessed)
	if iss != nil && iss.Err() != nil {
		return nil, fmt.Errorf("parse: %w", iss.Err())
	}

	return translateExpr(ast.NativeRep().Expr())
}

func containsThis(expr *ExprNode) bool {
	if expr == nil {
		return false
	}
	if expr.Kind == ExprIdent && expr.Ident == "this" {
		return true
	}
	if containsThis(expr.Left) || containsThis(expr.Right) || containsThis(expr.Operand) {
		return true
	}
	for _, a := range expr.Args {
		if containsThis(a) {
			return true
		}
	}
	return false
}

func translateCall(e celast.Expr) (*ExprNode, error) {
	call := e.AsCall()
	fnName := call.FunctionName()

	if op, ok := binaryOps[fnName]; ok {
		args := call.Args()
		if len(args) != 2 {
			return nil, fmt.Errorf("binary op %q expects 2 args, got %d", fnName, len(args))
		}
		left, err := translateExpr(args[0])
		if err != nil {
			return nil, err
		}
		right, err := translateExpr(args[1])
		if err != nil {
			return nil, err
		}
		return &ExprNode{Kind: ExprBinary, Op: op, Left: left, Right: right}, nil
	}

	if op, ok := unaryOps[fnName]; ok {
		args := call.Args()
		if len(args) != 1 {
			return nil, fmt.Errorf("unary op %q expects 1 arg, got %d", fnName, len(args))
		}
		operand, err := translateExpr(args[0])
		if err != nil {
			return nil, err
		}
		return &ExprNode{Kind: ExprUnary, Op: op, Operand: operand}, nil
	}

	// Member function call (e.g., x.startsWith(s)).
	if call.IsMemberFunction() {
		target, err := translateExpr(call.Target())
		if err != nil {
			return nil, err
		}
		var args []*ExprNode
		for _, a := range call.Args() {
			arg, err := translateExpr(a)
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
		}
		return &ExprNode{Kind: ExprCall, FuncName: fnName, Operand: target, Args: args, IsMember: true}, nil
	}

	// Global function call (e.g., size(x), has(x.f)).
	var args []*ExprNode
	for _, a := range call.Args() {
		arg, err := translateExpr(a)
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
	}
	return &ExprNode{Kind: ExprCall, FuncName: fnName, Args: args, IsMember: false}, nil
}

func translateExpr(e celast.Expr) (*ExprNode, error) {
	switch e.Kind() {
	case celast.LiteralKind:
		return translateLiteral(e)
	case celast.IdentKind:
		return &ExprNode{Kind: ExprIdent, Ident: e.AsIdent()}, nil
	case celast.SelectKind:
		sel := e.AsSelect()
		operand, err := translateExpr(sel.Operand())
		if err != nil {
			return nil, err
		}
		selectExpr := &ExprNode{Kind: ExprSelect, Operand: operand, Field: sel.FieldName()}
		if sel.IsTestOnly() {
			return &ExprNode{
				Kind:     ExprCall,
				FuncName: "has",
				Args:     []*ExprNode{selectExpr},
			}, nil
		}
		return selectExpr, nil
	case celast.CallKind:
		return translateCall(e)
	default:
		return nil, fmt.Errorf("unsupported expression kind: %v", e.Kind())
	}
}

func translateLiteral(e celast.Expr) (*ExprNode, error) {
	lit := &LiteralValue{}
	c := e.AsLiteral()

	switch c.Type() {
	case cel.IntType:
		v := c.Value().(int64)
		lit.Int = &v
	case cel.BoolType:
		v := c.Value().(bool)
		lit.Bool = &v
	case cel.StringType:
		v := c.Value().(string)
		lit.Str = &v
	case cel.DoubleType:
		v := c.Value().(float64)
		lit.Float = &v
	default:
		return nil, fmt.Errorf("unsupported literal type: %v", c.Type())
	}

	return &ExprNode{Kind: ExprLiteral, Literal: lit}, nil
}
