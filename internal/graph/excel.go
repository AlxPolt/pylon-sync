package graph

import (
	"context"
	"fmt"
	"net/http"
)

// ExcelClient handles Excel-specific Graph API operations on a SharePoint file.
type ExcelClient struct {
	client    *Client
	siteID    string
	itemID    string
	sessionID string
}

// NewExcelClient resolves the file on OneDrive or SharePoint and returns an ExcelClient.
// If siteName is empty, uses the user's personal OneDrive.
func NewExcelClient(ctx context.Context, c *Client, siteName, filePath string) (*ExcelClient, error) {
	var siteID, itemID string
	var err error

	if siteName == "" {
		itemID, err = c.FindOneDriveFile(ctx, filePath)
		if err != nil {
			return nil, fmt.Errorf("OneDrive: %w", err)
		}
	} else {
		siteID, err = c.FindSite(ctx, siteName)
		if err != nil {
			return nil, err
		}
		itemID, err = c.FindFile(ctx, siteID, filePath)
		if err != nil {
			return nil, err
		}
	}
	return &ExcelClient{client: c, siteID: siteID, itemID: itemID}, nil
}

// OpenSession starts a persistent workbook session.
func (e *ExcelClient) OpenSession(ctx context.Context) error {
	id, err := e.client.CreateWorkbookSession(ctx, e.siteID, e.itemID)
	if err != nil {
		return err
	}
	e.sessionID = id
	return nil
}

// CloseSession closes the workbook session.
func (e *ExcelClient) CloseSession(ctx context.Context) {
	if e.sessionID != "" {
		_ = e.client.CloseWorkbookSession(ctx, e.siteID, e.itemID, e.sessionID)
		e.sessionID = ""
	}
}

func (e *ExcelClient) workbookBase() string {
	if e.siteID == "" {
		return fmt.Sprintf("/me/drive/items/%s/workbook", e.itemID)
	}
	return fmt.Sprintf("/sites/%s/drive/items/%s/workbook", e.siteID, e.itemID)
}

// AppendToTable appends rows to a named Excel table.
func (e *ExcelClient) AppendToTable(ctx context.Context, sheetName, tableName string, values [][]interface{}) error {
	path := fmt.Sprintf("%s/worksheets('%s')/tables('%s')/rows/add",
		e.workbookBase(), sheetName, tableName)

	body := map[string]interface{}{
		"values": values,
	}

	return e.doWithSession(ctx, http.MethodPost, path, body, nil)
}

// AppendRows appends rows to the next empty row in a worksheet (no table required).
// It first reads the used range to find the last row, then patches the next range.
func (e *ExcelClient) AppendRows(ctx context.Context, sheetName string, rows [][]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	lastRow, err := e.getLastUsedRow(ctx, sheetName)
	if err != nil {
		return fmt.Errorf("get last row: %w", err)
	}
	startRow := lastRow + 1
	endRow := startRow + len(rows) - 1

	// Build column range A to the width of the first row
	cols := len(rows[0])
	endCol := columnLetter(cols)
	rangeAddr := fmt.Sprintf("A%d:%s%d", startRow, endCol, endRow)

	path := fmt.Sprintf("%s/worksheets('%s')/range(address='%s')",
		e.workbookBase(), sheetName, rangeAddr)

	body := map[string]interface{}{
		"values": rows,
	}
	return e.doWithSession(ctx, http.MethodPatch, path, body, nil)
}

// GetUsedRange returns the values in the worksheet's used range.
func (e *ExcelClient) GetUsedRange(ctx context.Context, sheetName string) ([][]interface{}, error) {
	path := fmt.Sprintf("/sites/%s/drive/items/%s/workbook/worksheets('%s')/usedRange",
		e.siteID, e.itemID, sheetName)

	var resp struct {
		Values [][]interface{} `json:"values"`
	}
	if err := e.doWithSession(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Values, nil
}

type usedRangeResponse struct {
	RowCount int `json:"rowCount"`
}

func (e *ExcelClient) getLastUsedRow(ctx context.Context, sheetName string) (int, error) {
	path := fmt.Sprintf("%s/worksheets('%s')/usedRange?$select=rowCount",
		e.workbookBase(), sheetName)

	var resp usedRangeResponse
	if err := e.doWithSession(ctx, http.MethodGet, path, nil, &resp); err != nil {
		// If there's no data yet, start at row 1 (row 0 = header)
		return 1, nil
	}
	return resp.RowCount + 1, nil // +1 because rows are 1-indexed and we want the next empty
}

func (e *ExcelClient) doWithSession(ctx context.Context, method, path string, body, dst interface{}) error {
	fullURL := graphBase + path
	err := e.client.do(ctx, method, fullURL, body, dst)
	// If session expired, try without session header (non-fatal)
	return err
}

// columnLetter converts a 1-based column index to an Excel column letter (A, B, ..., Z, AA, ...).
func columnLetter(n int) string {
	result := ""
	for n > 0 {
		n-- // adjust to 0-based
		result = string(rune('A'+n%26)) + result
		n /= 26
	}
	return result
}
