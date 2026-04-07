package config

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

type PylonConfig struct {
	APIToken     string `json:"api_token"`
	BaseURL      string `json:"base_url"`
	FilterStatus string `json:"filter_status"` // "all", "accepted", "pending"
	DaysLookback int    `json:"days_lookback"`
}

type MicrosoftConfig struct {
	TenantID       string `json:"tenant_id"`
	ClientID       string `json:"client_id"`
	ClientSecret   string `json:"client_secret"`
	SharePointSite string `json:"sharepoint_site"`
	FilePath       string `json:"file_path"`
	SheetName      string `json:"sheet_name"`
	TableName      string `json:"table_name"`
}

type FormatConfig struct {
	Template     string `json:"template,omitempty"`
	RoundTo      *int   `json:"round_to,omitempty"`
	OutputFormat string `json:"output_format,omitempty"`
	Fallback     string `json:"fallback,omitempty"`
	Calculation  string `json:"calculation,omitempty"`
}

type Condition struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
}

type Rule struct {
	Condition  string      `json:"condition,omitempty"`
	Conditions []Condition `json:"conditions,omitempty"`
	Match      string      `json:"match,omitempty"` // "all" or "any"
	Output     string      `json:"output"`
}

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

type SyncConfig struct {
	LastSync        time.Time `json:"last_sync"`
	AddedProjectIDs []string  `json:"added_project_ids"`
}

type Config struct {
	Pylon     PylonConfig     `json:"pylon"`
	Microsoft MicrosoftConfig `json:"microsoft"`
	Columns   []ColumnMapping `json:"columns"`
	Sync      SyncConfig      `json:"sync"`
}

var (
	mu           sync.RWMutex
	configPath   = "config.json"
)

func SetPath(p string) { configPath = p }

func Load() (*Config, error) {
	mu.RLock()
	defer mu.RUnlock()

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Override with environment variables if set (for Azure Functions)
	if v := os.Getenv("PYLON_API_TOKEN"); v != "" {
		cfg.Pylon.APIToken = v
	}
	if v := os.Getenv("MICROSOFT_CLIENT_ID"); v != "" {
		cfg.Microsoft.ClientID = v
	}
	if v := os.Getenv("MICROSOFT_TENANT_ID"); v != "" {
		cfg.Microsoft.TenantID = v
	}
	if v := os.Getenv("MICROSOFT_CLIENT_SECRET"); v != "" {
		cfg.Microsoft.ClientSecret = v
	}
	if v := os.Getenv("ONEDRIVE_FILE_PATH"); v != "" {
		cfg.Microsoft.FilePath = v
	}

	return &cfg, nil
}

func Save(cfg *Config) error {
	mu.Lock()
	defer mu.Unlock()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

func intPtr(i int) *int { return &i }

func DefaultConfig() *Config {
	return &Config{
		Pylon: PylonConfig{
			BaseURL:      "https://api.getpylon.com/v1",
			FilterStatus: "all",
			DaysLookback: 30,
		},
		Microsoft: MicrosoftConfig{
			SheetName: "Sheet1",
		},
		Columns: []ColumnMapping{
			{Type: "date", PylonField: "accepted_at", ExcelColumn: "A", Label: "Date order confirmed",
				Format: &FormatConfig{OutputFormat: "02/01/2006"}},
			{Type: "manual", ExcelColumn: "B", Label: "Complete", Default: ""},
			{Type: "manual", ExcelColumn: "C", Label: "Roofing", Default: ""},
			{Type: "manual", ExcelColumn: "D", Label: "Sparks & other info", Default: ""},
			{Type: "conditional", PylonField: "address", ExcelColumn: "E", Label: "SLD Required",
				Rules: []Rule{
					{Condition: "value contains 'Northern Ireland'", Output: "Yes (code)"},
					{Condition: "value contains 'Co.'", Output: "No"},
				},
				Default: "Check"},
			{Type: "direct", PylonField: "customer_name", ExcelColumn: "F", Label: "Name"},
			{Type: "direct", PylonField: "address", ExcelColumn: "G", Label: "Location & MPRN"},
			{Type: "direct", PylonField: "contact_phone", ExcelColumn: "H", Label: "Contact No"},
			{Type: "direct", PylonField: "contact_email", ExcelColumn: "I", Label: "Email"},
			{Type: "combine", PylonFields: []string{"module_desc", "inverter_desc", "storage_desc"},
				ExcelColumn: "J", Label: "Materials desc",
				Format: &FormatConfig{Template: "{0} | {1} | {2}", Fallback: "{0}"}},
			{Type: "manual", ExcelColumn: "K", Label: "Ordered", Default: ""},
			{Type: "panels", PylonFields: []string{"dc_output_kw", "module_quantity"},
				ExcelColumn: "L", Label: "Panels"},
			{Type: "conditional", PylonField: "address", ExcelColumn: "M", Label: "Shunts",
				Rules: []Rule{
					{Condition: "value contains 'Northern Ireland'", Output: "NIL"},
				},
				Default: "Required"},
			{Type: "manual", ExcelColumn: "N", Label: "Diverter", Default: ""},
			{Type: "direct", PylonField: "inverter_desc", ExcelColumn: "O", Label: "Inverter"},
			{Type: "direct", PylonField: "battery_desc", ExcelColumn: "P", Label: "Battery"},
			{Type: "direct", PylonField: "optimizers_desc", ExcelColumn: "Q", Label: "Opti"},
			{Type: "manual", ExcelColumn: "R", Label: "EV", Default: ""},
			{Type: "manual", ExcelColumn: "S", Label: "Extras", Default: ""},
			{Type: "manual", ExcelColumn: "T", Label: "Roof", Default: ""},
			{Type: "manual", ExcelColumn: "U", Label: "Deposit £", Default: ""},
			{Type: "manual", ExcelColumn: "V", Label: "Deposit €", Default: ""},
			{Type: "format", PylonField: "dc_output_kw", ExcelColumn: "W", Label: "System size",
				Format: &FormatConfig{Template: "{value} kW", RoundTo: intPtr(1)}},
			{Type: "manual", ExcelColumn: "X", Label: "Install", Default: ""},
			{Type: "manual", ExcelColumn: "Y", Label: "Total sale value £", Default: ""},
			{Type: "manual", ExcelColumn: "Z", Label: "Total sale value €", Default: ""},
			{Type: "direct", PylonField: "created_by", ExcelColumn: "AA", Label: "Sales consultant"},
			{Type: "manual", ExcelColumn: "AB", Label: "Source", Default: ""},
			{Type: "manual", ExcelColumn: "AC", Label: "Notes", Default: ""},
		},
		Sync: SyncConfig{
			LastSync:        time.Now().AddDate(0, 0, -30),
			AddedProjectIDs: []string{},
		},
	}
}
