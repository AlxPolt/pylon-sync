package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const graphBase = "https://graph.microsoft.com/v1.0"

// Client is an authenticated Microsoft Graph API client.
type Client struct {
	auth       *Authenticator
	httpClient *http.Client
}

// NewClient creates a Graph API client.
func NewClient(auth *Authenticator) *Client {
	return &Client{
		auth:       auth,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) do(ctx context.Context, method, url string, body interface{}, dst interface{}) error {
	token, err := c.auth.Token(ctx)
	if err != nil {
		return fmt.Errorf("get token: %w", err)
	}

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("graph request %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("graph API %s %s: status %d: %s", method, url, resp.StatusCode, respBody)
	}

	if dst != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, dst); err != nil {
			return fmt.Errorf("decode response: %w (body: %s)", err, respBody)
		}
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, dst interface{}) error {
	return c.do(ctx, http.MethodGet, graphBase+path, nil, dst)
}

func (c *Client) post(ctx context.Context, path string, body, dst interface{}) error {
	return c.do(ctx, http.MethodPost, graphBase+path, body, dst)
}

func (c *Client) patch(ctx context.Context, path string, body, dst interface{}) error {
	return c.do(ctx, http.MethodPatch, graphBase+path, body, dst)
}

// SiteResponse is the Graph API response for a site search.
type SiteResponse struct {
	Value []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"value"`
}

// FindSite looks up a SharePoint site by name and returns its ID.
func (c *Client) FindSite(ctx context.Context, siteName string) (string, error) {
	var resp SiteResponse
	if err := c.get(ctx, "/sites?search="+siteName, &resp); err != nil {
		return "", fmt.Errorf("find site %q: %w", siteName, err)
	}
	if len(resp.Value) == 0 {
		return "", fmt.Errorf("site %q not found", siteName)
	}
	return resp.Value[0].ID, nil
}

// DriveItemResponse is the Graph API response for a drive item.
type DriveItemResponse struct {
	ID string `json:"id"`
}

// FindOneDriveFile looks up a file in the user's personal OneDrive and returns its item ID.
func (c *Client) FindOneDriveFile(ctx context.Context, filePath string) (string, error) {
	path := fmt.Sprintf("/me/drive/root:/%s", filePath)
	var resp DriveItemResponse
	if err := c.get(ctx, path, &resp); err != nil {
		return "", fmt.Errorf("find OneDrive file %q: %w", filePath, err)
	}
	return resp.ID, nil
}

// FindFile looks up a file in a SharePoint site drive and returns its item ID.
func (c *Client) FindFile(ctx context.Context, siteID, filePath string) (string, error) {
	path := fmt.Sprintf("/sites/%s/drive/root:/%s", siteID, filePath)
	var resp DriveItemResponse
	if err := c.get(ctx, path, &resp); err != nil {
		return "", fmt.Errorf("find file %q: %w", filePath, err)
	}
	return resp.ID, nil
}

// WorkbookSessionResponse holds a workbook session ID.
type WorkbookSessionResponse struct {
	ID string `json:"id"`
}

// CreateWorkbookSession starts a persistent workbook session (faster for multiple writes).
func (c *Client) CreateWorkbookSession(ctx context.Context, siteID, itemID string) (string, error) {
	path := fmt.Sprintf("/sites/%s/drive/items/%s/workbook/createSession", siteID, itemID)
	body := map[string]bool{"persistChanges": true}
	var resp WorkbookSessionResponse
	if err := c.post(ctx, path, body, &resp); err != nil {
		return "", fmt.Errorf("create workbook session: %w", err)
	}
	return resp.ID, nil
}

// CloseWorkbookSession closes a workbook session.
func (c *Client) CloseWorkbookSession(ctx context.Context, siteID, itemID, sessionID string) error {
	path := fmt.Sprintf("/sites/%s/drive/items/%s/workbook/closeSession", siteID, itemID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, graphBase+path, nil)
	if err != nil {
		return err
	}
	token, _ := c.auth.Token(ctx)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("workbook-session-id", sessionID)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
