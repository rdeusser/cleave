package rust

import (
	"strings"

	"github.com/rdeusser/cleave/internal/ir"
)

type requirements struct {
	hasText           bool
	hasLegacyEncoding bool
	hasRegex          bool
	hasStartsWith     bool
	hasEndsWith       bool
	hasContains       bool
}

func analyzeRequirements(pkg *ir.Package) requirements {
	var result requirements
	for _, structure := range pkg.Structs {
		for _, field := range structure.Fields {
			if isTextField(field) {
				result.hasText = true
			}
			if field.Encoding != "" && !isUTF8(field.Encoding) {
				result.hasLegacyEncoding = true
			}
			inspectExpression(field.Condition, &result)
			for _, validation := range field.Validations {
				inspectExpression(validation.Expression, &result)
			}
		}
	}
	return result
}

func inspectExpression(expression *ir.ExprNode, result *requirements) {
	if expression == nil {
		return
	}
	if expression.Kind == ir.ExprCall {
		switch expression.FuncName {
		case "matches":
			result.hasRegex = true
		case "startsWith":
			result.hasStartsWith = true
		case "endsWith":
			result.hasEndsWith = true
		case "contains":
			result.hasContains = true
		}
	}
	inspectExpression(expression.Left, result)
	inspectExpression(expression.Right, result)
	inspectExpression(expression.Operand, result)
	for _, argument := range expression.Args {
		inspectExpression(argument, result)
	}
}

func isUTF8(encoding string) bool {
	return strings.EqualFold(encoding, "utf-8") || strings.EqualFold(encoding, "utf8")
}
