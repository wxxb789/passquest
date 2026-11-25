package models

import (
	"strings"

	"github.com/wxxb789/passquest/internal/provider"
)

// Selection is the catalog's chosen pair as both adapters show it above the
// provider rows: the trimmed provider id when a selection is configured, or
// the legacy hand-configured marker.
type Selection struct {
	Configured bool
	Provider   string
	Model      string
	Endpoint   string
	Legacy     bool
}

// SelectionOf is the active selection of `model list` and the window's model
// list. Legacy marks a saved configuration whose provider is empty: a
// hand-configured endpoint the registry does not name.
func SelectionOf(listing Listing) Selection {
	providerID := strings.TrimSpace(listing.Active.Provider)
	return Selection{
		Configured: listing.Configured && providerID != "",
		Provider:   providerID,
		Model:      listing.Active.Model,
		Endpoint:   listing.Active.Endpoint,
		Legacy:     listing.Configured && providerID == "",
	}
}

// Provider is one provider row of the catalog both adapters list: a registry
// provider, or the legacy hand-configured selection.
type Provider struct {
	// ID is the row's provider id; "legacy" for the hand-configured row.
	ID string
	// CredentialID is the id its credential variable belongs to; empty for
	// the legacy row, whose key is the legacy variable.
	CredentialID  string
	Name          string
	EndpointShape string
	Custom        bool
	Active        bool
	Models        []Model
}

// Model is one model a provider row shows, with its capability evidence.
type Model struct {
	ID           string
	Label        string
	Default      bool
	Active       bool
	Capabilities []CapabilityState
}

// ProvidersOf is the provider list `model list` and the window's model list
// render: every registry provider in order, with the active custom provider's
// registry models replaced by the one model its selection names, then the
// legacy row when a hand-configured endpoint is in effect. An empty registry
// stays a null list.
func ProvidersOf(listing Listing) []Provider {
	activeID := strings.TrimSpace(listing.Active.Provider)
	var rows []Provider
	for _, p := range listing.Registry.Providers {
		modelRows := modelRowsOf(listing, p.ID, p.Models, activeID == p.ID)
		shape := p.EndpointShape
		if p.Custom && activeID == p.ID {
			selected := provider.Model{ID: listing.Active.Model, Capabilities: []provider.Capability{provider.CapTextChat}}
			modelRows = modelRowsOf(listing, p.ID, []provider.Model{selected}, true)
			shape = shapeOfEndpoint(listing.Active.Endpoint, shape)
		}
		rows = append(rows, Provider{
			ID: p.ID, CredentialID: p.ID, Name: p.Name, EndpointShape: shape,
			Custom: p.Custom, Active: activeID == p.ID, Models: modelRows,
		})
	}
	if listing.Configured && activeID == "" {
		rows = append(rows, Provider{
			ID: "legacy", CredentialID: "", Name: "Manual tutor configuration",
			EndpointShape: shapeOfEndpoint(listing.Active.Endpoint, provider.EndpointShape), Active: true,
			Models: []Model{{
				ID: listing.Active.Model, Label: "hand-configured endpoint", Active: true,
				Capabilities: []CapabilityState{{ID: string(provider.CapTextChat), Declared: true, State: "legacy"}},
			}},
		})
	}
	return rows
}

func shapeOfEndpoint(endpoint, declared string) string {
	if format, ok := provider.FormatOf(endpoint); ok {
		return format.Shape()
	}
	return declared
}

// modelRowsOf renders one provider's models with their capability evidence,
// the way `model list` shows them.
func modelRowsOf(listing Listing, providerID string, models []provider.Model, active bool) []Model {
	rows := make([]Model, 0, len(models))
	for _, m := range models {
		row := Model{ID: m.ID, Label: m.Label, Default: m.Default, Active: active && listing.Active.Model == m.ID}
		verifications := listing.Index.ForModel(providerID, m.ID)
		for _, c := range provider.Capabilities {
			row.Capabilities = append(row.Capabilities, CapabilityStateOf(c, m.Has(c), verifications))
		}
		rows = append(rows, row)
	}
	return rows
}
