package rust

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/rdeusser/cleave/internal/ir"
)

func emitRustExpr(expression *ir.ExprNode, thisName string) (string, error) {
	if expression == nil {
		return "", errors.New("rust codegen: nil expression")
	}

	switch expression.Kind {
	case ir.ExprLiteral:
		return emitRustLiteral(expression.Literal)
	case ir.ExprIdent:
		if expression.Ident == "this" {
			if thisName == "" {
				return "", errors.New("rust codegen: this is not available in this expression")
			}
			return thisName, nil
		}
		return rustIdent(expression.Ident), nil
	case ir.ExprBinary:
		left, err := emitRustExpr(expression.Left, thisName)
		if err != nil {
			return "", err
		}
		right, err := emitRustExpr(expression.Right, thisName)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(%s %s %s)", left, expression.Op, right), nil
	case ir.ExprUnary:
		operand, err := emitRustExpr(expression.Operand, thisName)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(%s%s)", expression.Op, operand), nil
	case ir.ExprSelect:
		operand, err := emitRustExpr(expression.Operand, thisName)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.%s", operand, rustIdent(expression.Field)), nil
	case ir.ExprCall:
		return emitRustCall(expression, thisName)
	default:
		return "", fmt.Errorf("rust codegen: unsupported expression kind %d", expression.Kind)
	}
}

func emitRustLiteral(literal *ir.LiteralValue) (string, error) {
	if literal == nil {
		return "", errors.New("rust codegen: nil literal")
	}
	switch {
	case literal.Int != nil:
		return rustInt(*literal.Int), nil
	case literal.Bool != nil:
		return strconv.FormatBool(*literal.Bool), nil
	case literal.Str != nil:
		return rustString(*literal.Str), nil
	case literal.Float != nil:
		value := strconv.FormatFloat(*literal.Float, 'g', -1, 64)
		if !strings.ContainsAny(value, ".eE") {
			value += ".0"
		}
		return value, nil
	default:
		return "", errors.New("rust codegen: empty literal")
	}
}

func emitRustCall(expression *ir.ExprNode, thisName string) (string, error) {
	switch expression.FuncName {
	case "size":
		var target *ir.ExprNode
		if expression.IsMember {
			target = expression.Operand
		} else if len(expression.Args) == 1 {
			target = expression.Args[0]
		}
		if target == nil {
			return "", errors.New("rust codegen: size expects one value")
		}
		operand, err := emitRustExpr(target, thisName)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(%s).len()", operand), nil
	case "has":
		if expression.IsMember || len(expression.Args) != 1 {
			return "", errors.New("rust codegen: has expects one field")
		}
		operand, err := emitRustExpr(expression.Args[0], thisName)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(%s).is_some()", operand), nil
	case "startsWith", "endsWith", "contains", "matches":
		if !expression.IsMember || len(expression.Args) != 1 {
			return "", fmt.Errorf("rust codegen: %s expects one member argument", expression.FuncName)
		}
		operand, err := emitRustExpr(expression.Operand, thisName)
		if err != nil {
			return "", err
		}
		argument, err := emitRustExpr(expression.Args[0], thisName)
		if err != nil {
			return "", err
		}
		operand = rustStringReference(expression.Operand, operand)
		argument = rustStringReference(expression.Args[0], argument)
		switch expression.FuncName {
		case "startsWith":
			return fmt.Sprintf(
				"self::text_starts_with(%s, %s)",
				operand,
				argument,
			), nil
		case "endsWith":
			return fmt.Sprintf(
				"self::text_ends_with(%s, %s)",
				operand,
				argument,
			), nil
		case "contains":
			return fmt.Sprintf(
				"self::text_contains(%s, %s)",
				operand,
				argument,
			), nil
		default:
			return fmt.Sprintf(
				"self::regex_matches(%s, %s)?",
				operand,
				argument,
			), nil
		}
	default:
		return "", fmt.Errorf("rust codegen: unsupported CEL function %q", expression.FuncName)
	}
}

func rustStringReference(expression *ir.ExprNode, emitted string) string {
	if expression.Kind == ir.ExprLiteral && expression.Literal != nil && expression.Literal.Str != nil {
		return emitted
	}
	return fmt.Sprintf("(%s).as_ref()", emitted)
}
