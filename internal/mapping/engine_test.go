package mapping_test

import (
	"testing"
	"time"

	"pylon-sharepoint-sync/internal/mapping"
	"pylon-sharepoint-sync/internal/pylon"
)

func makeProject() *pylon.Project {
	accepted := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	return &pylon.Project{
		ID:             "proj-1",
		CustomerName:   "John Smith",
		Address:        "10 Main St, Dromore, Northern Ireland, BT25 1AA",
		ContactPhone:   "07777123456",
		ContactEmail:   "john@example.com",
		Status:         "accepted",
		CreatedAt:      time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC),
		AcceptedAt:     &accepted,
		CreatedBy:      "Sarah Jones",
		WebProposalURL: "https://app.getpylon.com/proposal/abc123",
		DcOutputKW:     9.5,
		StorageKWH:     10.0,
		ModuleQty:      20,
		InverterDesc:   "Solis 10kW S6",
		BatteryDesc:    "Fox ESS H10 x1",
		OptimizersDesc: "",
		ModuleDesc:     "Trina Vertex 475W",
	}
}

func TestDirect(t *testing.T) {
	p := makeProject()
	col := mapping.ColumnMapping{Type: "direct", PylonField: "customer_name", ExcelColumn: "F", Label: "Name"}
	got, err := mapping.Apply(col, p)
	if err != nil {
		t.Fatal(err)
	}
	if got != "John Smith" {
		t.Errorf("want 'John Smith', got %q", got)
	}
}

func TestFormatKW(t *testing.T) {
	p := makeProject()
	roundTo := 1
	col := mapping.ColumnMapping{
		Type: "format", PylonField: "dc_output_kw", ExcelColumn: "W", Label: "System size",
		Format: &mapping.FormatConfig{Template: "{value} kW", RoundTo: &roundTo},
	}
	got, err := mapping.Apply(col, p)
	if err != nil {
		t.Fatal(err)
	}
	if got != "9.5 kW" {
		t.Errorf("want '9.5 kW', got %q", got)
	}
}

func TestPanels(t *testing.T) {
	p := makeProject() // 9.5 kW / 20 panels = 475 W
	col := mapping.ColumnMapping{
		Type: "panels", PylonFields: []string{"dc_output_kw", "module_quantity"},
		ExcelColumn: "L", Label: "Panels",
	}
	got, err := mapping.Apply(col, p)
	if err != nil {
		t.Fatal(err)
	}
	if got != "20x475W" {
		t.Errorf("want '20x475W', got %q", got)
	}
}

func TestConditionalNI(t *testing.T) {
	p := makeProject()
	col := mapping.ColumnMapping{
		Type: "conditional", PylonField: "address", ExcelColumn: "M", Label: "Shunts",
		Rules: []mapping.Rule{
			{Condition: "value contains 'Northern Ireland'", Output: "NIL"},
		},
		Default: "Required",
	}
	got, err := mapping.Apply(col, p)
	if err != nil {
		t.Fatal(err)
	}
	if got != "NIL" {
		t.Errorf("want 'NIL' for NI address, got %q", got)
	}
}

func TestConditionalROI(t *testing.T) {
	p := makeProject()
	p.Address = "5 Church St, Galway, Co. Galway, Ireland"
	col := mapping.ColumnMapping{
		Type: "conditional", PylonField: "address", ExcelColumn: "M", Label: "Shunts",
		Rules: []mapping.Rule{
			{Condition: "value contains 'Northern Ireland'", Output: "NIL"},
		},
		Default: "Required",
	}
	got, err := mapping.Apply(col, p)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Required" {
		t.Errorf("want 'Required' for ROI address, got %q", got)
	}
}

func TestSLDRequired(t *testing.T) {
	p := makeProject() // NI address
	col := mapping.ColumnMapping{
		Type: "conditional", PylonField: "address", ExcelColumn: "E", Label: "SLD Required",
		Rules: []mapping.Rule{
			{Condition: "value contains 'Northern Ireland'", Output: "Yes (code)"},
		},
		Default: "No",
	}
	got, err := mapping.Apply(col, p)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Yes (code)" {
		t.Errorf("want 'Yes (code)', got %q", got)
	}
}

func TestDateFormat(t *testing.T) {
	p := makeProject()
	col := mapping.ColumnMapping{
		Type: "date", PylonField: "accepted_at", ExcelColumn: "A", Label: "Date order confirmed",
		Format: &mapping.FormatConfig{OutputFormat: "02/01/2006"},
	}
	got, err := mapping.Apply(col, p)
	if err != nil {
		t.Fatal(err)
	}
	if got != "15/03/2026" {
		t.Errorf("want '15/03/2026', got %q", got)
	}
}

func TestLookup(t *testing.T) {
	p := makeProject()
	col := mapping.ColumnMapping{
		Type: "lookup", PylonField: "status", ExcelColumn: "G", Label: "Status",
		LookupTable: map[string]string{
			"accepted": "Sold",
			"pending":  "Awaiting Response",
		},
		Default: "Unknown",
	}
	got, err := mapping.Apply(col, p)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Sold" {
		t.Errorf("want 'Sold', got %q", got)
	}
}

func TestStatic(t *testing.T) {
	p := makeProject()
	col := mapping.ColumnMapping{Type: "static", ExcelColumn: "AB", Label: "Source", Value: "Pylon Import"}
	got, err := mapping.Apply(col, p)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Pylon Import" {
		t.Errorf("want 'Pylon Import', got %q", got)
	}
}

func TestManual(t *testing.T) {
	p := makeProject()
	col := mapping.ColumnMapping{Type: "manual", ExcelColumn: "B", Label: "Complete", Default: ""}
	got, err := mapping.Apply(col, p)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("want empty string for manual, got %q", got)
	}
}

func TestMultiConditional(t *testing.T) {
	p := makeProject() // dcKW=9.5, storage=10.0
	col := mapping.ColumnMapping{
		Type: "multi_conditional", ExcelColumn: "E", Label: "Package",
		Rules: []mapping.Rule{
			{
				Conditions: []mapping.Condition{
					{Field: "dc_output_kw", Operator: ">", Value: 4},
					{Field: "storage_kwh", Operator: ">", Value: 0},
				},
				Match:  "all",
				Output: "Premium (PV + Battery)",
			},
			{
				Conditions: []mapping.Condition{
					{Field: "storage_kwh", Operator: ">", Value: 0},
				},
				Match:  "all",
				Output: "Standard + Battery",
			},
		},
		Default: "PV Only",
	}
	got, err := mapping.Apply(col, p)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Premium (PV + Battery)" {
		t.Errorf("want 'Premium (PV + Battery)', got %q", got)
	}
}

func TestFormatPanels(t *testing.T) {
	tests := []struct {
		kw  float64
		qty int
		exp string
	}{
		{9.5, 20, "20x475W"},
		{4.0, 8, "8x500W"},
		{11.4, 24, "24x475W"},
	}
	for _, tt := range tests {
		got := mapping.FormatPanels(tt.kw, tt.qty)
		if got != tt.exp {
			t.Errorf("FormatPanels(%v, %d) = %q, want %q", tt.kw, tt.qty, got, tt.exp)
		}
	}
}
