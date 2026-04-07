package pylon

import (
	"fmt"
	"strings"
	"time"
)

// JSON:API response wrappers

type ListResponse struct {
	Data  []ProjectResource `json:"data"`
	Meta  *Meta             `json:"meta,omitempty"`
	Links *Links            `json:"links,omitempty"`
}

type SingleProjectResponse struct {
	Data ProjectResource `json:"data"`
}

type SingleDesignResponse struct {
	Data DesignResource `json:"data"`
}

type Meta struct {
	TotalCount int `json:"total_count"`
}

type Links struct {
	Next string `json:"next"`
	Prev string `json:"prev"`
}

// solar_projects resource

type ProjectResource struct {
	ID            string               `json:"id"`
	Type          string               `json:"type"`
	Attributes    ProjectAttributes    `json:"attributes"`
	Relationships ProjectRelationships `json:"relationships"`
}

type ProjectAttributes struct {
	ReferenceNumber string          `json:"reference_number"`
	SiteAddress     SiteAddress     `json:"site_address"`
	CustomerDetails CustomerDetails `json:"customer_details"`
	Acceptance      Acceptance      `json:"acceptance"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	SiteCountryCode string          `json:"site_country_code"`
}

type SiteAddress struct {
	Line1   string  `json:"line1"`
	Line2   *string `json:"line2"`
	City    string  `json:"city"`
	State   string  `json:"state"`
	Zip     string  `json:"zip"`
	Country string  `json:"country"`
}

func (a SiteAddress) Full() string {
	parts := []string{a.Line1}
	if a.Line2 != nil && *a.Line2 != "" {
		parts = append(parts, *a.Line2)
	}
	parts = append(parts, a.City, a.State, a.Zip)
	return strings.Join(parts, ", ")
}

type CustomerDetails struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
	Email string `json:"email"`
}

type Acceptance struct {
	IsAccepted bool `json:"is_accepted"`
}

type ProjectRelationships struct {
	Owner struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	} `json:"owner"`
	PrimaryDesign struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	} `json:"primary_design"`
}

// solar_designs resource

type DesignResource struct {
	ID         string           `json:"id"`
	Type       string           `json:"type"`
	Attributes DesignAttributes `json:"attributes"`
}

type DesignAttributes struct {
	Summary       DesignSummary  `json:"summary"`
	ModuleTypes   []ModuleType   `json:"module_types"`
	InverterTypes []InverterType `json:"inverter_types"`
	StorageTypes  []StorageType  `json:"storage_types"`
	MaterialTypes []MaterialType `json:"material_types"`
	Optimizers    []Optimizer    `json:"solar_optimizers"`
}

type DesignSummary struct {
	DcOutputKW float64 `json:"dc_output_kw"`
	StorageKWH float64 `json:"storage_kwh"`
	QuoteTotal float64 `json:"quote_total"`
}

type ModuleType struct {
	Description string  `json:"description"`
	Quantity    int     `json:"quantity"`
	WattPeak    float64 `json:"watt_peak"`
}

type InverterType struct {
	Description string `json:"description"`
	Quantity    int    `json:"quantity"`
	Brand       string `json:"brand"`
}

type StorageType struct {
	Description string  `json:"description"`
	Quantity    int     `json:"quantity"`
	CapacityKWH float64 `json:"capacity_kwh"`
}

type MaterialType struct {
	Description string `json:"description"`
	Quantity    int    `json:"quantity"`
	Category    string `json:"category"`
}

type Optimizer struct {
	Description string `json:"description"`
	Quantity    int    `json:"quantity"`
}

// Project is the denormalised view used throughout the app.

type Project struct {
	ID             string
	CustomerName   string
	Address        string
	AddressLine1   string
	AddressCity    string
	AddressZip     string
	AddressState   string
	ContactPhone   string
	ContactEmail   string
	Status         string // "accepted" or "pending"
	CreatedAt      time.Time
	AcceptedAt     *time.Time
	UpdatedAt      time.Time
	CreatedBy      string
	WebProposalURL string
	PDFProposalURL string

	// From design
	DcOutputKW     float64
	StorageKWH     float64
	QuoteTotal     float64
	ModuleQty      int
	ModuleDesc     string
	InverterDesc   string
	BatteryDesc    string
	OptimizersDesc string
	MaterialsDesc  string
}

func (p *Project) GetField(field string) string {
	switch field {
	case "customer_name":
		return p.CustomerName
	case "address":
		return p.Address
	case "address_mprn":
		return fmt.Sprintf("%s, %s %s MPRN:", p.AddressLine1, p.AddressCity, p.AddressZip)
	case "address_state":
		return p.AddressState
	case "contact_phone":
		return p.ContactPhone
	case "contact_email":
		return p.ContactEmail
	case "status":
		return p.Status
	case "created_by":
		return p.CreatedBy
	case "web_proposal_url":
		return p.WebProposalURL
	case "pdf_proposal_url":
		return p.PDFProposalURL
	case "module_desc":
		return p.ModuleDesc
	case "inverter_desc":
		return p.InverterDesc
	case "storage_desc", "battery_desc":
		return p.BatteryDesc
	case "optimizers_desc":
		return p.OptimizersDesc
	case "materials_desc":
		return p.MaterialsDesc
	default:
		return ""
	}
}

func (p *Project) GetFloatField(field string) (float64, bool) {
	switch field {
	case "dc_output_kw":
		return p.DcOutputKW, true
	case "storage_kwh":
		return p.StorageKWH, true
	case "quote_total":
		return p.QuoteTotal, true
	}
	return 0, false
}

func (p *Project) GetIntField(field string) (int, bool) {
	switch field {
	case "module_quantity":
		return p.ModuleQty, true
	}
	return 0, false
}

func (p *Project) GetTimeField(field string) (*time.Time, bool) {
	switch field {
	case "created_at":
		return &p.CreatedAt, true
	case "accepted_at":
		return p.AcceptedAt, true
	case "updated_at":
		return &p.UpdatedAt, true
	}
	return nil, false
}

// projectFromResource converts a raw API resource to a Project.
func projectFromResource(r ProjectResource) *Project {
	a := r.Attributes

	status := "pending"
	if a.Acceptance.IsAccepted {
		status = "accepted"
	}

	var acceptedAt *time.Time
	if a.Acceptance.IsAccepted {
		t := a.UpdatedAt
		acceptedAt = &t
	}

	return &Project{
		ID:           r.ID,
		CustomerName: a.CustomerDetails.Name,
		Address:      a.SiteAddress.Full(),
		AddressLine1: a.SiteAddress.Line1,
		AddressCity:  a.SiteAddress.City,
		AddressZip:   a.SiteAddress.Zip,
		AddressState: a.SiteAddress.State,
		ContactPhone: a.CustomerDetails.Phone,
		ContactEmail: a.CustomerDetails.Email,
		Status:       status,
		CreatedAt:    a.CreatedAt,
		UpdatedAt:    a.UpdatedAt,
		AcceptedAt:   acceptedAt,
		CreatedBy:    r.Relationships.Owner.Data.ID,
	}
}

// enrichWithDesign fills design-derived fields on an existing Project.
func enrichWithDesign(p *Project, d *DesignAttributes) {
	p.DcOutputKW = d.Summary.DcOutputKW
	p.StorageKWH = d.Summary.StorageKWH
	p.QuoteTotal = d.Summary.QuoteTotal

	if len(d.ModuleTypes) > 0 {
		m := d.ModuleTypes[0]
		p.ModuleQty = m.Quantity
		p.ModuleDesc = m.Description
	}
	if len(d.InverterTypes) > 0 {
		inv := d.InverterTypes[0]
		p.InverterDesc = inv.Description
		if inv.Quantity > 1 {
			p.InverterDesc = fmt.Sprintf("%s x%d", inv.Description, inv.Quantity)
		}
	}
	if len(d.StorageTypes) > 0 {
		s := d.StorageTypes[0]
		p.BatteryDesc = s.Description
		if s.Quantity > 1 {
			p.BatteryDesc = fmt.Sprintf("%s x%d", s.Description, s.Quantity)
		}
	}
	if len(d.Optimizers) > 0 {
		o := d.Optimizers[0]
		p.OptimizersDesc = fmt.Sprintf("%s x%d", o.Description, o.Quantity)
	}

	parts := []string{}
	if p.ModuleDesc != "" {
		parts = append(parts, p.ModuleDesc)
	}
	if p.InverterDesc != "" {
		parts = append(parts, p.InverterDesc)
	}
	if p.BatteryDesc != "" {
		parts = append(parts, p.BatteryDesc)
	}
	p.MaterialsDesc = strings.Join(parts, " | ")
}
