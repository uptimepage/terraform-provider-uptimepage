package client

import (
	"context"
	"fmt"
	"net/http"
)

const regionCatalogPath = "/api/v1/regions"

// Region is one catalog entry. Everything but id and name is optional: a region
// is registered before its coordinates are known, and may never get them.
type Region struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	City        string   `json:"city"`
	CountryCode string   `json:"country_code"`
	Continent   string   `json:"continent"`
	Latitude    *float64 `json:"latitude"`
	Longitude   *float64 `json:"longitude"`
}

type regionCatalog struct {
	Regions []Region `json:"regions"`
}

// ListRegions reads the enabled regions, which is exactly what a target may be
// assigned to.
func (c *Client) ListRegions(ctx context.Context) ([]Region, error) {
	var out regionCatalog
	if err := c.do(ctx, http.MethodGet, regionCatalogPath, nil, &out); err != nil {
		return nil, fmt.Errorf("list regions: %w", err)
	}
	return out.Regions, nil
}
