package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"pylon-sharepoint-sync/internal/config"
	"pylon-sharepoint-sync/internal/graph"
	"pylon-sharepoint-sync/internal/mapping"
	"pylon-sharepoint-sync/internal/pylon"
)

func main() {
	fmt.Println("=== Pylon → SharePoint Sync ===")

	testMode := len(os.Args) > 1 && os.Args[1] == "--test"

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Config error: ", err)
	}

	if cfg.Microsoft.TenantID == "" {
		log.Fatal("Microsoft credentials not set. Add them to config.json")
	}

	listMode := len(os.Args) > 1 && os.Args[1] == "--list"
	projectMode := len(os.Args) > 2 && os.Args[1] == "--project"

	if testMode {
		runTest(cfg)
		return
	}

	if projectMode {
		runProject(cfg, os.Args[2])
		return
	}

	if cfg.Pylon.APIToken == "" {
		log.Fatal("Pylon API token not set. Add it to config.json")
	}

	if listMode {
		runList(cfg)
		return
	}

	// ── Fetch proposals from Pylon ──────────────────────────────────────────
	fmt.Printf("Fetching proposals from Pylon (last %d days)...\n", cfg.Pylon.DaysLookback)

	pylonClient := pylon.NewClient(cfg.Pylon.BaseURL, cfg.Pylon.APIToken)
	since := time.Now().AddDate(0, 0, -cfg.Pylon.DaysLookback)
	projects, err := pylonClient.ListProjects(pylon.ListProjectsFilter{
		Status:      cfg.Pylon.FilterStatus,
		Since:       since,
		WithDesigns: true,
	})
	if err != nil {
		log.Fatal("Pylon error: ", err)
	}
	fmt.Printf("Found %d proposals\n", len(projects))

	// Filter already exported
	added := make(map[string]bool)
	for _, id := range cfg.Sync.AddedProjectIDs {
		added[id] = true
	}
	var newProjects []*pylon.Project
	for _, p := range projects {
		if !added[p.ID] {
			newProjects = append(newProjects, p)
		}
	}

	if len(newProjects) == 0 {
		fmt.Println("Nothing new to export. All proposals already in Excel.")
		pause()
		return
	}
	fmt.Printf("%d new proposals to export\n", len(newProjects))

	// ── Apply mapping ───────────────────────────────────────────────────────
	cols := toMappingCols(cfg.Columns)
	var excelRows [][]interface{}
	var exportedIDs []string

	for _, p := range newProjects {
		ordered, err := mapping.ApplyOrdered(cols, p)
		if err != nil {
			fmt.Printf("  WARNING: mapping error for %s (%s): %v\n", p.ID, p.CustomerName, err)
			continue
		}
		row := make([]interface{}, len(ordered))
		for i, v := range ordered {
			row[i] = v
		}
		excelRows = append(excelRows, row)
		exportedIDs = append(exportedIDs, p.ID)
		fmt.Printf("  ✓ %s — %s\n", p.CustomerName, p.Address)
	}

	// ── Write to SharePoint / OneDrive ──────────────────────────────────────
	fmt.Println("Writing to Excel...")

	ctx := context.Background()
	auth := graph.NewAuthenticator(cfg.Microsoft.TenantID, cfg.Microsoft.ClientID, cfg.Microsoft.ClientSecret)
	gc := graph.NewClient(auth)

	excel, err := graph.NewExcelClient(ctx, gc, cfg.Microsoft.SharePointSite, cfg.Microsoft.FilePath)
	if err != nil {
		log.Fatal("SharePoint error: ", err)
	}

	if err := excel.OpenSession(ctx); err != nil {
		fmt.Printf("WARNING: could not open workbook session: %v\n", err)
	}
	defer excel.CloseSession(ctx)

	if cfg.Microsoft.TableName != "" {
		err = excel.AppendToTable(ctx, cfg.Microsoft.SheetName, cfg.Microsoft.TableName, excelRows)
	} else {
		err = excel.AppendRows(ctx, cfg.Microsoft.SheetName, excelRows)
	}
	if err != nil {
		log.Fatal("Excel write error: ", err)
	}

	// ── Save sync state ─────────────────────────────────────────────────────
	cfg.Sync.AddedProjectIDs = append(cfg.Sync.AddedProjectIDs, exportedIDs...)
	cfg.Sync.LastSync = time.Now()
	if err := config.Save(cfg); err != nil {
		fmt.Printf("WARNING: could not save config: %v\n", err)
	}

	fmt.Printf("\n✓ Done! Added %d rows to Excel.\n", len(excelRows))
	pause()
}

func runProject(cfg *config.Config, id string) {
	fmt.Printf("Fetching project %s from Pylon...\n\n", id)

	pylonClient := pylon.NewClient(cfg.Pylon.BaseURL, cfg.Pylon.APIToken)
	p, err := pylonClient.FetchProject(id)
	if err != nil {
		log.Fatal("Pylon error: ", err)
	}

	// Fetch design
	cols := toMappingCols(cfg.Columns)
	ordered, err := mapping.ApplyOrdered(cols, p)
	if err != nil {
		log.Fatal("Mapping error: ", err)
	}
	row := make([]interface{}, len(ordered))
	for i, v := range ordered {
		row[i] = v
	}

	fmt.Printf("Name:    %s\n", p.CustomerName)
	fmt.Printf("Address: %s\n", p.AddressLine1+", "+p.AddressCity+" "+p.AddressZip)
	fmt.Printf("System:  %.1f kW\n", p.DcOutputKW)
	fmt.Printf("Panels:  %s\n", mapping.FormatPanels(p.DcOutputKW, p.ModuleQty))
	fmt.Println("\nWriting to Excel...")

	ctx := context.Background()
	auth := graph.NewAuthenticator(cfg.Microsoft.TenantID, cfg.Microsoft.ClientID, cfg.Microsoft.ClientSecret)
	gc := graph.NewClient(auth)

	excel, err := graph.NewExcelClient(ctx, gc, cfg.Microsoft.SharePointSite, cfg.Microsoft.FilePath)
	if err != nil {
		log.Fatal("SharePoint error: ", err)
	}
	defer excel.CloseSession(ctx)

	if err := excel.AppendRows(ctx, cfg.Microsoft.SheetName, [][]interface{}{row}); err != nil {
		log.Fatal("Excel write error: ", err)
	}

	fmt.Println("✓ Done!")
	pause()
}

func runList(cfg *config.Config) {
	fmt.Printf("Fetching last 1 proposal from Pylon...\n\n")

	pylonClient := pylon.NewClient(cfg.Pylon.BaseURL, cfg.Pylon.APIToken)
	projects, err := pylonClient.ListProjects(pylon.ListProjectsFilter{
		Status:      cfg.Pylon.FilterStatus,
		Since:       time.Now().AddDate(0, 0, -cfg.Pylon.DaysLookback),
		WithDesigns: true,
	})
	if err != nil {
		log.Fatal("Pylon error: ", err)
	}
	if len(projects) == 0 {
		fmt.Println("No proposals found.")
		pause()
		return
	}

	p := projects[0]
	fmt.Printf("ID:       %s\n", p.ID)
	fmt.Printf("Name:     %s\n", p.CustomerName)
	fmt.Printf("Address:  %s\n", p.Address)
	fmt.Printf("Phone:    %s\n", p.ContactPhone)
	fmt.Printf("Email:    %s\n", p.ContactEmail)
	fmt.Printf("Status:   %s\n", p.Status)
	fmt.Printf("System:   %.1f kW\n", p.DcOutputKW)
	fmt.Printf("Panels:   %s\n", mapping.FormatPanels(p.DcOutputKW, p.ModuleQty))
	fmt.Printf("Inverter: %s\n", p.InverterDesc)
	fmt.Printf("Battery:  %s\n", p.BatteryDesc)
	fmt.Printf("By:       %s\n", p.CreatedBy)
	fmt.Println("\n✓ Read successfully. Nothing was written to Excel.")
	pause()
}

func runTest(cfg *config.Config) {
	fmt.Println("TEST MODE — writing one test row to Excel...")

	excelRows := [][]interface{}{
		{"31/03/2026", "", "", "", "Yes (code)", "Test Customer", "10 Main St, Dromore, Northern Ireland",
			"07777123456", "test@example.com", "Trina 475W | Solis 10kW | Fox ESS H10",
			"", "20x475W", "NIL", "", "Solis 10kW S6", "Fox ESS H10 x1", "",
			"", "", "", "", "", "9.5 kW", "", "", "", "Test Consultant", "", ""},
	}

	ctx := context.Background()
	auth := graph.NewAuthenticator(cfg.Microsoft.TenantID, cfg.Microsoft.ClientID, cfg.Microsoft.ClientSecret)
	gc := graph.NewClient(auth)

	excel, err := graph.NewExcelClient(ctx, gc, cfg.Microsoft.SharePointSite, cfg.Microsoft.FilePath)
	if err != nil {
		log.Fatal("SharePoint error: ", err)
	}
	defer excel.CloseSession(ctx)

	if err := excel.AppendRows(ctx, cfg.Microsoft.SheetName, excelRows); err != nil {
		log.Fatal("Excel write error: ", err)
	}

	fmt.Println("✓ Test row written to Excel successfully!")
	pause()
}

// pause keeps the window open on Windows so user can read the output.
func pause() {
	if isWindows() {
		fmt.Println("\nPress Enter to exit...")
		fmt.Scanln()
	}
}

func isWindows() bool {
	return os.PathSeparator == '\\'
}

func toMappingCols(cfgCols []config.ColumnMapping) []mapping.ColumnMapping {
	out := make([]mapping.ColumnMapping, len(cfgCols))
	for i, c := range cfgCols {
		out[i] = mapping.ColumnMapping{
			Type:        c.Type,
			PylonField:  c.PylonField,
			PylonFields: c.PylonFields,
			ExcelColumn: c.ExcelColumn,
			Label:       c.Label,
			Value:       c.Value,
			Default:     c.Default,
		}
		if c.Format != nil {
			out[i].Format = &mapping.FormatConfig{
				Template:     c.Format.Template,
				RoundTo:      c.Format.RoundTo,
				OutputFormat: c.Format.OutputFormat,
				Fallback:     c.Format.Fallback,
			}
		}
		if c.LookupTable != nil {
			out[i].LookupTable = c.LookupTable
		}
		for _, r := range c.Rules {
			mr := mapping.Rule{
				Condition: r.Condition,
				Match:     r.Match,
				Output:    r.Output,
			}
			for _, cond := range r.Conditions {
				mr.Conditions = append(mr.Conditions, mapping.Condition{
					Field:    cond.Field,
					Operator: cond.Operator,
					Value:    cond.Value,
				})
			}
			out[i].Rules = append(out[i].Rules, mr)
		}
	}
	return out
}
