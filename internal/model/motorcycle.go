// Package model contains the domain types stored in Elasticsearch.
package model

// Motorcycle is one entry in the Honda motorcycle catalog.
//
// It has no ID field on purpose: the identifier lives in the Elasticsearch
// document _id, so keeping a copy in _source would let the two drift apart.
//
// No field uses omitempty, so zero values such as Available=false are still
// stored explicitly instead of disappearing from the document.
type Motorcycle struct {
	Brand        string `json:"brand"`
	ModelName    string `json:"model_name"`
	Category     string `json:"category"`
	EngineCC     int    `json:"engine_cc"`
	Transmission string `json:"transmission"`
	PriceIDR     int64  `json:"price_idr"`
	Available    bool   `json:"available"`
	Description  string `json:"description"`
}

// MotorcycleWithID pairs a Motorcycle domain model with its document _id.
type MotorcycleWithID struct {
	ID string
	Motorcycle
}

// SearchFilter specifies criteria for searching, filtering, sorting, and paginating motorcycles.
type SearchFilter struct {
	Search       string
	Category     string
	Transmission string
	Available    *bool
	MinPrice     *int64
	MaxPrice     *int64
	Sort         string
	Page         int
	Limit        int
}

// SearchResult contains the paginated items and hit counts from a search.
type SearchResult struct {
	Items      []MotorcycleWithID
	Total      int64
	Page       int
	Limit      int
	TotalPages int
}
