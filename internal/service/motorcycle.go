// Package service implements the motorcycle catalog domain business logic.
package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/fajri-rabka/go-elasticsearch-poc/internal/model"
)

// slugRegex matches clean lowercase slugs like "honda-beat-001" or "honda-scoopy-api-001".
var slugRegex = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

const (
	minIDLength = 3
	maxIDLength = 64
)

// ValidationError represents a business rule validation failure (HTTP 400).
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

func validationErr(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

// MotorcycleRepository abstracts the underlying data storage for motorcycles.
type MotorcycleRepository interface {
	Create(ctx context.Context, id string, m model.Motorcycle) error
	Get(ctx context.Context, id string) (model.Motorcycle, error)
	Replace(ctx context.Context, id string, m model.Motorcycle) error
	Delete(ctx context.Context, id string) error
	Search(ctx context.Context, filter model.SearchFilter) (model.SearchResult, error)
}

// MotorcycleService coordinates motorcycle catalog business operations.
type MotorcycleService struct {
	repo MotorcycleRepository
}

func NewMotorcycleService(repo MotorcycleRepository) *MotorcycleService {
	return &MotorcycleService{repo: repo}
}

// Create validates business rules and stores a new motorcycle under id.
func (s *MotorcycleService) Create(ctx context.Context, id string, m model.Motorcycle) error {
	if err := s.ValidateID(id); err != nil {
		return err
	}
	if err := s.ValidateMotorcycle(m); err != nil {
		return err
	}

	return s.repo.Create(ctx, id, m)
}

// Get validates the id and retrieves the motorcycle from the repository.
func (s *MotorcycleService) Get(ctx context.Context, id string) (model.Motorcycle, error) {
	if err := s.ValidateID(id); err != nil {
		return model.Motorcycle{}, err
	}

	return s.repo.Get(ctx, id)
}

// Replace validates business rules and performs a full replacement of an existing motorcycle.
func (s *MotorcycleService) Replace(ctx context.Context, id string, m model.Motorcycle) error {
	if err := s.ValidateID(id); err != nil {
		return err
	}
	if err := s.ValidateMotorcycle(m); err != nil {
		return err
	}

	return s.repo.Replace(ctx, id, m)
}

// Delete validates the id and deletes the motorcycle from the repository.
func (s *MotorcycleService) Delete(ctx context.Context, id string) error {
	if err := s.ValidateID(id); err != nil {
		return err
	}

	return s.repo.Delete(ctx, id)
}

// ValidateID checks that the document ID matches the expected lowercase slug format.
func (s *MotorcycleService) ValidateID(id string) error {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return validationErr("id must not be empty")
	}
	if len(trimmed) < minIDLength || len(trimmed) > maxIDLength {
		return validationErr("id length must be between %d and %d characters", minIDLength, maxIDLength)
	}
	if !slugRegex.MatchString(trimmed) {
		return validationErr("id %q must be lowercase alphanumeric with optional hyphens (e.g. honda-scoopy-api-001)", trimmed)
	}

	return nil
}

// ValidateMotorcycle enforces domain rules for motorcycles in this POC.
func (s *MotorcycleService) ValidateMotorcycle(m model.Motorcycle) error {
	if strings.TrimSpace(m.Brand) != "Honda" {
		return validationErr("brand must be %q", "Honda")
	}
	if strings.TrimSpace(m.ModelName) == "" {
		return validationErr("model_name must not be empty")
	}
	if strings.TrimSpace(m.Category) == "" {
		return validationErr("category must not be empty")
	}
	if m.EngineCC <= 0 {
		return validationErr("engine_cc must be greater than 0")
	}
	if strings.TrimSpace(m.Transmission) == "" {
		return validationErr("transmission must not be empty")
	}
	if m.PriceIDR <= 0 {
		return validationErr("price_idr must be greater than 0")
	}
	if strings.TrimSpace(m.Description) == "" {
		return validationErr("description must not be empty")
	}

	return nil
}

const (
	minLimit  = 1
	maxLimit  = 50
	maxWindow = 10000
)

var allowedSortOptions = map[string]struct{}{
	"relevance":  {},
	"price_asc":  {},
	"price_desc": {},
	"name_asc":   {},
	"name_desc":  {},
}

// Search coordinates search, filter, sort, and pagination across motorcycles.
func (s *MotorcycleService) Search(ctx context.Context, filter model.SearchFilter) (model.SearchResult, error) {
	if err := s.ValidateSearchFilter(&filter); err != nil {
		return model.SearchResult{}, err
	}

	return s.repo.Search(ctx, filter)
}

// ValidateSearchFilter enforces search parameter rules and sets sort defaults.
func (s *MotorcycleService) ValidateSearchFilter(f *model.SearchFilter) error {
	f.Search = strings.TrimSpace(f.Search)
	f.Category = strings.TrimSpace(f.Category)
	f.Transmission = strings.TrimSpace(f.Transmission)
	f.Sort = strings.TrimSpace(f.Sort)

	if f.MinPrice != nil && *f.MinPrice < 0 {
		return validationErr("min_price must be greater than or equal to 0")
	}
	if f.MaxPrice != nil && *f.MaxPrice < 0 {
		return validationErr("max_price must be greater than or equal to 0")
	}
	if f.MinPrice != nil && f.MaxPrice != nil && *f.MinPrice > *f.MaxPrice {
		return validationErr("min_price cannot be greater than max_price")
	}

	if f.Page < 1 {
		return validationErr("page must be greater than or equal to 1")
	}

	if f.Limit < minLimit || f.Limit > maxLimit {
		return validationErr("limit must be between %d and %d", minLimit, maxLimit)
	}

	// Guard against integer overflow and ensure we stay within Elasticsearch max_result_window.
	from := int64(f.Page-1) * int64(f.Limit)
	if from < 0 || from+int64(f.Limit) > maxWindow {
		return validationErr("page offset and limit exceed maximum result window of %d", maxWindow)
	}

	if f.Sort == "" {
		if f.Search != "" {
			f.Sort = "relevance"
		} else {
			f.Sort = "name_asc"
		}
	} else {
		if _, ok := allowedSortOptions[f.Sort]; !ok {
			return validationErr("invalid sort option %q", f.Sort)
		}
		if f.Sort == "relevance" && f.Search == "" {
			return validationErr("sort=relevance is only valid when a search query is provided")
		}
	}

	return nil
}
