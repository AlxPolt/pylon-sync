package mapping

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"pylon-sharepoint-sync/internal/pylon"
)

// Apply computes the Excel cell value for one column from one project.
func Apply(col ColumnMapping, p *pylon.Project) (string, error) {
	switch MappingType(col.Type) {
	case TypeDirect:
		return p.GetField(col.PylonField), nil

	case TypeFormat:
		return applyFormat(col, p)

	case TypeCombine:
		return applyCombine(col, p)

	case TypeConditional:
		return applyConditional(col, p)

	case TypeMultiConditional:
		return applyMultiConditional(col, p)

	case TypeStatic:
		return col.Value, nil

	case TypeDate:
		return applyDate(col, p)

	case TypeLookup:
		return applyLookup(col, p)

	case TypeManual:
		return col.Default, nil

	case TypePanels:
		return applyPanels(col, p)

	default:
		return "", fmt.Errorf("unknown mapping type: %q", col.Type)
	}
}

// ApplyAll applies all column mappings to a project and returns a map of
// ExcelColumn → value.
func ApplyAll(cols []ColumnMapping, p *pylon.Project) (map[string]string, error) {
	result := make(map[string]string, len(cols))
	for _, col := range cols {
		val, err := Apply(col, p)
		if err != nil {
			return nil, fmt.Errorf("column %s (%s): %w", col.ExcelColumn, col.Label, err)
		}
		result[col.ExcelColumn] = val
	}
	return result, nil
}

// ApplyOrdered returns values in the same order as cols.
func ApplyOrdered(cols []ColumnMapping, p *pylon.Project) ([]string, error) {
	result := make([]string, len(cols))
	for i, col := range cols {
		val, err := Apply(col, p)
		if err != nil {
			return nil, fmt.Errorf("column %s (%s): %w", col.ExcelColumn, col.Label, err)
		}
		result[i] = val
	}
	return result, nil
}

// --- private helpers ---

func applyFormat(col ColumnMapping, p *pylon.Project) (string, error) {
	if col.Format == nil {
		return p.GetField(col.PylonField), nil
	}
	numVal, hasNum := p.GetFloatField(col.PylonField)
	strVal := p.GetField(col.PylonField)

	var display string
	if hasNum {
		v := numVal
		if col.Format.RoundTo != nil {
			factor := math.Pow(10, float64(*col.Format.RoundTo))
			v = math.Round(v*factor) / factor
		}
		display = strconv.FormatFloat(v, 'f', -1, 64)
		if col.Format.RoundTo != nil {
			display = strconv.FormatFloat(v, 'f', *col.Format.RoundTo, 64)
		}
	} else {
		display = strVal
	}

	if col.Format.Template != "" {
		return strings.ReplaceAll(col.Format.Template, "{value}", display), nil
	}
	return display, nil
}

func applyCombine(col ColumnMapping, p *pylon.Project) (string, error) {
	if len(col.PylonFields) == 0 {
		return col.Default, nil
	}
	values := make([]string, len(col.PylonFields))
	for i, f := range col.PylonFields {
		values[i] = p.GetField(f)
	}

	if col.Format == nil || col.Format.Template == "" {
		return strings.Join(filterEmpty(values), " | "), nil
	}

	result := col.Format.Template
	for i, v := range values {
		result = strings.ReplaceAll(result, fmt.Sprintf("{%d}", i), v)
	}

	// If all values empty, use fallback
	if allEmpty(values) && col.Format.Fallback != "" {
		result = col.Format.Fallback
		for i, v := range values {
			result = strings.ReplaceAll(result, fmt.Sprintf("{%d}", i), v)
		}
	}
	return result, nil
}

func applyConditional(col ColumnMapping, p *pylon.Project) (string, error) {
	strVal := p.GetField(col.PylonField)
	numVal, hasNum := p.GetFloatField(col.PylonField)
	_ = hasNum

	for _, rule := range col.Rules {
		if rule.Condition == "" {
			continue
		}
		// Replace "value" token with actual string value for string conditions,
		// but also pass numeric for numeric operators.
		evalStr := strVal
		if hasNum {
			evalStr = strconv.FormatFloat(numVal, 'f', -1, 64)
		}
		matched, err := EvalStringCondition(rule.Condition, evalStr)
		if err != nil {
			return "", err
		}
		if matched {
			return rule.Output, nil
		}
	}
	return col.Default, nil
}

func applyMultiConditional(col ColumnMapping, p *pylon.Project) (string, error) {
	for _, rule := range col.Rules {
		if len(rule.Conditions) == 0 {
			continue
		}
		matched := evalMultiConditions(rule.Conditions, rule.Match, p)
		if matched {
			return rule.Output, nil
		}
	}
	return col.Default, nil
}

func evalMultiConditions(conds []Condition, match string, p *pylon.Project) bool {
	for _, c := range conds {
		strVal := p.GetField(c.Field)
		numVal, hasNum := p.GetFloatField(c.Field)

		ok, err := EvalCondition(c, strVal, numVal, hasNum)
		if err != nil {
			ok = false
		}

		if match == "any" && ok {
			return true
		}
		if match != "any" && !ok {
			return false
		}
	}
	return match != "any"
}

func applyDate(col ColumnMapping, p *pylon.Project) (string, error) {
	t, ok := p.GetTimeField(col.PylonField)
	if !ok || t == nil || t.IsZero() {
		return "", nil
	}

	layout := "02/01/2006" // default DD/MM/YYYY
	if col.Format != nil && col.Format.OutputFormat != "" {
		layout = col.Format.OutputFormat
	}
	return t.Format(layout), nil
}

func applyLookup(col ColumnMapping, p *pylon.Project) (string, error) {
	key := p.GetField(col.PylonField)
	if v, ok := col.LookupTable[key]; ok {
		return v, nil
	}
	return col.Default, nil
}

func applyPanels(col ColumnMapping, p *pylon.Project) (string, error) {
	dcKW, _ := p.GetFloatField("dc_output_kw")
	qty, _ := p.GetIntField("module_quantity")
	if qty == 0 || dcKW == 0 {
		return "", nil
	}
	return FormatPanels(dcKW, qty), nil
}

// --- util ---

func filterEmpty(ss []string) []string {
	out := ss[:0]
	for _, s := range ss {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func allEmpty(ss []string) bool {
	for _, s := range ss {
		if s != "" {
			return false
		}
	}
	return true
}

// FormatDate is exported for use in templates.
func FormatDate(t *time.Time, layout string) string {
	if t == nil || t.IsZero() {
		return ""
	}
	if layout == "" {
		layout = "02/01/2006"
	}
	return t.Format(layout)
}
