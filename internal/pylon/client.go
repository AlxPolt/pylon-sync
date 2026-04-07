package pylon

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is an HTTP client for the Pylon REST API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a Pylon API client.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ListProjectsFilter controls which projects are fetched.
type ListProjectsFilter struct {
	Status      string    // "accepted", "pending", "rejected", "expired", "all"
	Since       time.Time // only projects created/accepted after this date
	WithDesigns bool      // fetch design details for each project (extra API calls)
}

// ListProjects fetches solar projects from the API, fetches their design data,
// and returns a slice of denormalised Project structs.
func (c *Client) ListProjects(f ListProjectsFilter) ([]*Project, error) {
	raw, err := c.fetchAllProjects(f)
	if err != nil {
		return nil, err
	}

	projects := make([]*Project, 0, len(raw))
	for _, r := range raw {
		// Filter by status on our side
		if f.Status == "accepted" && !r.Attributes.Acceptance.IsAccepted {
			continue
		}
		if f.Status == "pending" && r.Attributes.Acceptance.IsAccepted {
			continue
		}
		// Filter by date on our side
		if !f.Since.IsZero() && r.Attributes.CreatedAt.Before(f.Since) {
			continue
		}

		p := projectFromResource(r)

		if f.WithDesigns && r.Relationships.PrimaryDesign.Data.ID != "" {
			design, err := c.FetchDesign(r.Relationships.PrimaryDesign.Data.ID)
			if err == nil {
				enrichWithDesign(p, design)
			}
			// non-fatal: continue without design data
		}
		projects = append(projects, p)
	}
	return projects, nil
}

// FetchProject fetches a single project by ID including its design.
func (c *Client) FetchProject(id string) (*Project, error) {
	var resp SingleProjectResponse
	if err := c.get("/solar_projects/"+id, nil, &resp); err != nil {
		return nil, err
	}
	p := projectFromResource(resp.Data)
	if resp.Data.Relationships.PrimaryDesign.Data.ID != "" {
		if design, err := c.FetchDesign(resp.Data.Relationships.PrimaryDesign.Data.ID); err == nil {
			enrichWithDesign(p, design)
		}
	}
	return p, nil
}

// FetchDesign fetches design details for a given design ID.
func (c *Client) FetchDesign(id string) (*DesignAttributes, error) {
	params := url.Values{}
	params.Set("fields[solar_designs]", "summary,module_types,inverter_types,storage_types,material_types")

	var resp SingleDesignResponse
	if err := c.get("/solar_designs/"+id, params, &resp); err != nil {
		return nil, err
	}
	return &resp.Data.Attributes, nil
}

// fetchAllProjects handles pagination and returns raw project resources.
func (c *Client) fetchAllProjects(f ListProjectsFilter) ([]ProjectResource, error) {
	params := url.Values{}
	params.Set("page[size]", "100")

	var all []ProjectResource
	path := "/solar_projects"

	for {
		var resp ListResponse
		if err := c.get(path, params, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Data...)

		if resp.Links == nil || resp.Links.Next == "" {
			break
		}
		// Use the full next URL directly
		nextURL, err := url.Parse(resp.Links.Next)
		if err != nil {
			break
		}
		params = nextURL.Query()
		// If next link is absolute, extract just the path without base
		path = strings.TrimPrefix(nextURL.Path, "/v1")
	}

	return all, nil
}

func (c *Client) get(path string, params url.Values, dst interface{}) error {
	reqURL := c.baseURL + path
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.api+json")
	req.Header.Set("Content-Type", "application/vnd.api+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("pylon request %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("pylon API %s: status %d: %s", path, resp.StatusCode, body)
	}

	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("decode response from %s: %w", path, err)
	}
	return nil
}

