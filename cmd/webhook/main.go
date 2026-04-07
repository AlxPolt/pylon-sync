package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"pylon-sharepoint-sync/internal/config"
	"pylon-sharepoint-sync/internal/graph"
	"pylon-sharepoint-sync/internal/mapping"
	"pylon-sharepoint-sync/internal/pylon"
)

// PylonWebhook is the payload Pylon sends when a proposal is signed.
type PylonWebhook struct {
	Data struct {
		Attributes struct {
			Name string `json:"name"`
		} `json:"attributes"`
		Relationships struct {
			SolarProject struct {
				Data struct {
					ID string `json:"id"`
				} `json:"data"`
			} `json:"solar_project"`
		} `json:"relationships"`
	} `json:"data"`
}

func main() {
	port := os.Getenv("FUNCTIONS_CUSTOMHANDLER_PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/api/webhook", handleWebhook)
	http.HandleFunc("/api/health", handleHealth)

	log.Printf("Webhook server listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "ok")
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var webhook PylonWebhook
	if err := json.Unmarshal(body, &webhook); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Only handle signed proposals
	if webhook.Data.Attributes.Name != "web_proposals.signed" {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "event ignored")
		return
	}

	projectID := webhook.Data.Relationships.SolarProject.Data.ID
	if projectID == "" {
		http.Error(w, "missing project ID", http.StatusBadRequest)
		return
	}

	log.Printf("New signed proposal: project ID=%s", projectID)

	// Load config
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Config error: %v", err)
		http.Error(w, "config error", http.StatusInternalServerError)
		return
	}

	// Fetch project + design from Pylon
	pylonClient := pylon.NewClient(cfg.Pylon.BaseURL, cfg.Pylon.APIToken)
	project, err := pylonClient.FetchProject(projectID)
	if err != nil {
		log.Printf("Pylon error: %v", err)
		http.Error(w, "pylon error", http.StatusInternalServerError)
		return
	}

	// Apply column mapping
	cols := toMappingCols(cfg.Columns)
	ordered, err := mapping.ApplyOrdered(cols, project)
	if err != nil {
		log.Printf("Mapping error: %v", err)
		http.Error(w, "mapping error", http.StatusInternalServerError)
		return
	}
	row := make([]interface{}, len(ordered))
	for i, v := range ordered {
		row[i] = v
	}

	// Write to Excel on OneDrive/SharePoint
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	auth := graph.NewAuthenticator(cfg.Microsoft.TenantID, cfg.Microsoft.ClientID, cfg.Microsoft.ClientSecret)
	gc := graph.NewClient(auth)

	excel, err := graph.NewExcelClient(ctx, gc, cfg.Microsoft.SharePointSite, cfg.Microsoft.FilePath)
	if err != nil {
		log.Printf("Excel client error: %v", err)
		http.Error(w, "excel error", http.StatusInternalServerError)
		return
	}
	defer excel.CloseSession(ctx)

	if err := excel.AppendRows(ctx, cfg.Microsoft.SheetName, [][]interface{}{row}); err != nil {
		log.Printf("Excel write error: %v", err)
		http.Error(w, "excel write error", http.StatusInternalServerError)
		return
	}

	log.Printf("✓ Written to Excel: %s — %s", project.CustomerName, project.Address)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "ok: %s added to Excel", project.CustomerName)
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
