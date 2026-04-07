package mapping

// MappingType defines how a Pylon field maps to an Excel column.
type MappingType string

const (
	TypeDirect           MappingType = "direct"
	TypeFormat           MappingType = "format"
	TypeCombine          MappingType = "combine"
	TypeConditional      MappingType = "conditional"
	TypeMultiConditional MappingType = "multi_conditional"
	TypeStatic           MappingType = "static"
	TypeDate             MappingType = "date"
	TypeLookup           MappingType = "lookup"
	TypeManual           MappingType = "manual"
	TypePanels           MappingType = "panels" // special: "{qty}x{watt}W"
)

// FormatConfig controls value transformation for format/date/combine types.
type FormatConfig struct {
	Template     string `json:"template,omitempty"`
	RoundTo      *int   `json:"round_to,omitempty"`
	OutputFormat string `json:"output_format,omitempty"` // Go time layout
	Fallback     string `json:"fallback,omitempty"`
	Calculation  string `json:"calculation,omitempty"`
}

// Condition is a single predicate in a multi_conditional rule.
type Condition struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
}

// Rule is one branch in a conditional/multi_conditional mapping.
type Rule struct {
	// For simple conditional: "value > 10"
	Condition string `json:"condition,omitempty"`
	// For multi_conditional: list of field conditions
	Conditions []Condition `json:"conditions,omitempty"`
	Match      string      `json:"match,omitempty"` // "all" | "any"
	Output     string      `json:"output"`
}

// ColumnMapping is the config for one Excel column.
type ColumnMapping struct {
	Type        string            `json:"type"`
	PylonField  string            `json:"pylon_field,omitempty"`
	PylonFields []string          `json:"pylon_fields,omitempty"`
	ExcelColumn string            `json:"excel_column"`
	Label       string            `json:"label"`
	Rules       []Rule            `json:"rules,omitempty"`
	Format      *FormatConfig     `json:"format,omitempty"`
	LookupTable map[string]string `json:"lookup_table,omitempty"`
	Value       string            `json:"value,omitempty"`
	Default     string            `json:"default,omitempty"`
}

// IsAuto returns true if the column is filled automatically (not manually).
func (c *ColumnMapping) IsAuto() bool {
	return MappingType(c.Type) != TypeManual
}
