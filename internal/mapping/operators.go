package mapping

import (
	"fmt"
	"strconv"
	"strings"
)

// EvalStringCondition evaluates a simple condition string like "value > 10"
// against a string value. The string is parsed to float64 for numeric operators.
func EvalStringCondition(condition, value string) (bool, error) {
	condition = strings.TrimSpace(condition)

	for _, op := range []string{">=", "<=", "!=", ">", "<", "=="} {
		parts := strings.SplitN(condition, " "+op+" ", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) == "value" {
			rhs := strings.TrimSpace(parts[1])
			return compareNumeric(value, op, rhs)
		}
	}

	if strings.HasPrefix(condition, "value contains ") {
		needle := strings.Trim(strings.TrimPrefix(condition, "value contains "), "'\"")
		return strings.Contains(value, needle), nil
	}
	if strings.HasPrefix(condition, "value starts_with ") {
		needle := strings.Trim(strings.TrimPrefix(condition, "value starts_with "), "'\"")
		return strings.HasPrefix(value, needle), nil
	}
	if condition == "value is_empty" {
		return value == "", nil
	}
	if condition == "value is_not_empty" {
		return value != "", nil
	}

	return false, fmt.Errorf("unknown condition: %q", condition)
}

// EvalCondition evaluates a structured Condition against a field value (string or numeric).
func EvalCondition(cond Condition, strVal string, numVal float64, hasNum bool) (bool, error) {
	op := cond.Operator
	rhsRaw := fmt.Sprintf("%v", cond.Value)

	switch op {
	case "contains":
		needle := strings.Trim(rhsRaw, "'\"")
		return strings.Contains(strVal, needle), nil
	case "starts_with":
		needle := strings.Trim(rhsRaw, "'\"")
		return strings.HasPrefix(strVal, needle), nil
	case "is_empty":
		return strVal == "", nil
	case "is_not_empty":
		return strVal != "", nil
	}

	// Numeric operators
	if hasNum {
		rhs, err := strconv.ParseFloat(rhsRaw, 64)
		if err != nil {
			return false, fmt.Errorf("cannot parse rhs %q as float: %w", rhsRaw, err)
		}
		switch op {
		case ">":
			return numVal > rhs, nil
		case ">=":
			return numVal >= rhs, nil
		case "<":
			return numVal < rhs, nil
		case "<=":
			return numVal <= rhs, nil
		case "==":
			return numVal == rhs, nil
		case "!=":
			return numVal != rhs, nil
		}
	}

	// String equality fallback
	switch op {
	case "==":
		return strVal == strings.Trim(rhsRaw, "'\""), nil
	case "!=":
		return strVal != strings.Trim(rhsRaw, "'\""), nil
	}

	return false, fmt.Errorf("unknown operator: %q", op)
}

func compareNumeric(lhsStr, op, rhsStr string) (bool, error) {
	lhs, err := strconv.ParseFloat(lhsStr, 64)
	if err != nil {
		// Fall back to string comparison for == and !=
		switch op {
		case "==":
			return lhsStr == rhsStr, nil
		case "!=":
			return lhsStr != rhsStr, nil
		default:
			return false, fmt.Errorf("cannot parse %q as number for operator %s", lhsStr, op)
		}
	}
	rhs, err := strconv.ParseFloat(rhsStr, 64)
	if err != nil {
		return false, fmt.Errorf("cannot parse rhs %q as number", rhsStr)
	}
	switch op {
	case ">":
		return lhs > rhs, nil
	case ">=":
		return lhs >= rhs, nil
	case "<":
		return lhs < rhs, nil
	case "<=":
		return lhs <= rhs, nil
	case "==":
		return lhs == rhs, nil
	case "!=":
		return lhs != rhs, nil
	}
	return false, fmt.Errorf("unknown operator: %s", op)
}
