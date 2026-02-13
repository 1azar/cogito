package tools

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"

	"github.com/1azar/cogito/tool"
)

type CalcInput struct {
	Expression string `json:"expression"`
}

type CalcOutput struct {
	Result float64 `json:"result"`
}

func calculate(ctx context.Context, in CalcInput) (CalcOutput, error) {
	if in.Expression == "" {
		return CalcOutput{}, errors.New("expression is empty")
	}

	node, err := parser.ParseExpr(in.Expression)
	if err != nil {
		return CalcOutput{}, fmt.Errorf("invalid expression: %w", err)
	}

	result, err := eval(node)
	if err != nil {
		return CalcOutput{}, err
	}

	return CalcOutput{Result: result}, nil
}

var CalculatorTool tool.Tool

func init() {
	var err error

	CalculatorTool, err = tool.Func(
		"calculator",
		"Safely evaluates a numeric mathematical expression. Supports +, -, *, /, %, parentheses, and floating point numbers. Does not allow variables or functions.",
		calculate,
	)
	if err != nil {
		panic(err)
	}
}

func eval(expr ast.Expr) (float64, error) {
	switch e := expr.(type) {

	case *ast.BasicLit:
		if e.Kind != token.INT && e.Kind != token.FLOAT {
			return 0, errors.New("only numeric literals allowed")
		}
		return strconv.ParseFloat(e.Value, 64)

	case *ast.BinaryExpr:
		left, err := eval(e.X)
		if err != nil {
			return 0, err
		}
		right, err := eval(e.Y)
		if err != nil {
			return 0, err
		}

		switch e.Op {
		case token.ADD:
			return left + right, nil
		case token.SUB:
			return left - right, nil
		case token.MUL:
			return left * right, nil
		case token.QUO:
			if right == 0 {
				return 0, errors.New("division by zero")
			}
			return left / right, nil
		case token.REM:
			return float64(int64(left) % int64(right)), nil
		default:
			return 0, fmt.Errorf("unsupported operator: %s", e.Op)
		}

	case *ast.ParenExpr:
		return eval(e.X)

	case *ast.UnaryExpr:
		val, err := eval(e.X)
		if err != nil {
			return 0, err
		}
		switch e.Op {
		case token.ADD:
			return val, nil
		case token.SUB:
			return -val, nil
		default:
			return 0, fmt.Errorf("unsupported unary operator: %s", e.Op)
		}

	default:
		return 0, fmt.Errorf("unsupported expression type: %T", expr)
	}
}
