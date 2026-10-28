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
