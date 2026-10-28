package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fajri-rabka/go-elasticsearch-poc/internal/model"
)

type fakeRepo struct {
	createFunc  func(ctx context.Context, id string, m model.Motorcycle) error
	getFunc     func(ctx context.Context, id string) (model.Motorcycle, error)
	replaceFunc func(ctx context.Context, id string, m model.Motorcycle) error
	deleteFunc  func(ctx context.Context, id string) error
}

func (f *fakeRepo) Create(ctx context.Context, id string, m model.Motorcycle) error {
	if f.createFunc != nil {
		return f.createFunc(ctx, id, m)
	}
	return nil
}

func (f *fakeRepo) Get(ctx context.Context, id string) (model.Motorcycle, error) {
	if f.getFunc != nil {
		return f.getFunc(ctx, id)
	}
	return model.Motorcycle{}, nil
}

func (f *fakeRepo) Replace(ctx context.Context, id string, m model.Motorcycle) error {
	if f.replaceFunc != nil {
		return f.replaceFunc(ctx, id, m)
	}
	return nil
}

func (f *fakeRepo) Delete(ctx context.Context, id string) error {
	if f.deleteFunc != nil {
		return f.deleteFunc(ctx, id)
	}
	return nil
}

func validMotorcycle() model.Motorcycle {
	return model.Motorcycle{
		Brand:        "Honda",
		ModelName:    "Scoopy",
		Category:     "matic",
		EngineCC:     110,
		Transmission: "automatic",
		PriceIDR:     22_000_000,
		Available:    true,
		Description:  "Motor matic harian",
	}
}

func TestValidateID(t *testing.T) {
	svc := NewMotorcycleService(&fakeRepo{})

	tests := []struct {
		name    string
		id      string
		wantErr string
	}{
		{name: "valid id", id: "honda-beat-001"},
		{name: "valid id with digits", id: "cbr-250rr"},
		{name: "empty id", id: "", wantErr: "id must not be empty"},
		{name: "id with spaces", id: "honda beat", wantErr: "must be lowercase alphanumeric"},
		{name: "uppercase id", id: "Honda-Beat", wantErr: "must be lowercase alphanumeric"},
		{name: "special characters", id: "honda_beat!", wantErr: "must be lowercase alphanumeric"},
		{name: "too short", id: "ab", wantErr: "between 3 and 64 characters"},
		{name: "too long", id: strings.Repeat("a", 65), wantErr: "between 3 and 64 characters"},
		{name: "leading hyphen", id: "-honda-beat", wantErr: "must be lowercase alphanumeric"},
		{name: "trailing hyphen", id: "honda-beat-", wantErr: "must be lowercase alphanumeric"},
		{name: "double hyphen", id: "honda--beat", wantErr: "must be lowercase alphanumeric"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.ValidateID(tt.id)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateID(%q) error = %v, want error containing %q", tt.id, err, tt.wantErr)
			}
		})
	}
}

func TestValidateMotorcycle(t *testing.T) {
	svc := NewMotorcycleService(&fakeRepo{})

	tests := []struct {
		name    string
		mutate  func(m *model.Motorcycle)
		wantErr string
	}{
		{
			name:   "valid motorcycle",
			mutate: func(m *model.Motorcycle) {},
		},
		{
			name:    "brand not Honda",
			mutate:  func(m *model.Motorcycle) { m.Brand = "Yamaha" },
			wantErr: `brand must be "Honda"`,
		},
		{
			name:    "brand empty",
			mutate:  func(m *model.Motorcycle) { m.Brand = "" },
			wantErr: `brand must be "Honda"`,
		},
		{
			name:    "empty model_name",
			mutate:  func(m *model.Motorcycle) { m.ModelName = "   " },
			wantErr: "model_name must not be empty",
		},
		{
			name:    "empty category",
			mutate:  func(m *model.Motorcycle) { m.Category = "" },
			wantErr: "category must not be empty",
		},
		{
			name:    "zero engine cc",
			mutate:  func(m *model.Motorcycle) { m.EngineCC = 0 },
			wantErr: "engine_cc must be greater than 0",
		},
		{
			name:    "negative engine cc",
			mutate:  func(m *model.Motorcycle) { m.EngineCC = -10 },
			wantErr: "engine_cc must be greater than 0",
		},
		{
			name:    "empty transmission",
			mutate:  func(m *model.Motorcycle) { m.Transmission = "" },
			wantErr: "transmission must not be empty",
		},
		{
			name:    "zero price",
			mutate:  func(m *model.Motorcycle) { m.PriceIDR = 0 },
			wantErr: "price_idr must be greater than 0",
		},
		{
			name:    "negative price",
			mutate:  func(m *model.Motorcycle) { m.PriceIDR = -5000 },
			wantErr: "price_idr must be greater than 0",
		},
		{
			name:    "empty description",
			mutate:  func(m *model.Motorcycle) { m.Description = " " },
			wantErr: "description must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := validMotorcycle()
			tt.mutate(&m)

			err := svc.ValidateMotorcycle(m)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateMotorcycle() error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestServiceCRUDOperations(t *testing.T) {
	ctx := context.Background()

	t.Run("Create calls repo on valid input", func(t *testing.T) {
		called := false
		svc := NewMotorcycleService(&fakeRepo{
			createFunc: func(ctx context.Context, id string, m model.Motorcycle) error {
				called = true
				if id != "honda-beat-001" {
					t.Errorf("got id %q, want %q", id, "honda-beat-001")
				}
				return nil
			},
		})

		err := svc.Create(ctx, "honda-beat-001", validMotorcycle())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !called {
			t.Errorf("expected repo.Create to be called")
		}
	})

	t.Run("Create fails on invalid id before repo", func(t *testing.T) {
		called := false
		svc := NewMotorcycleService(&fakeRepo{
			createFunc: func(ctx context.Context, id string, m model.Motorcycle) error {
				called = true
				return nil
			},
		})

		err := svc.Create(ctx, "INVALID_ID", validMotorcycle())
		if err == nil {
			t.Fatalf("expected error for invalid id")
		}
		if called {
			t.Errorf("repo should not be called when validation fails")
		}
	})

	t.Run("Get propagates repo error", func(t *testing.T) {
		wantErr := errors.New("not found")
		svc := NewMotorcycleService(&fakeRepo{
			getFunc: func(ctx context.Context, id string) (model.Motorcycle, error) {
				return model.Motorcycle{}, wantErr
			},
		})

		_, err := svc.Get(ctx, "honda-beat-001")
		if !errors.Is(err, wantErr) {
			t.Fatalf("got %v, want %v", err, wantErr)
		}
	})

	t.Run("Replace validates before calling repo", func(t *testing.T) {
		called := false
		svc := NewMotorcycleService(&fakeRepo{
			replaceFunc: func(ctx context.Context, id string, m model.Motorcycle) error {
				called = true
				return nil
			},
		})

		err := svc.Replace(ctx, "honda-beat-001", validMotorcycle())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !called {
			t.Errorf("expected repo.Replace to be called")
		}
	})

	t.Run("Delete calls repo", func(t *testing.T) {
		called := false
		svc := NewMotorcycleService(&fakeRepo{
			deleteFunc: func(ctx context.Context, id string) error {
				called = true
				return nil
			},
		})

		err := svc.Delete(ctx, "honda-beat-001")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !called {
			t.Errorf("expected repo.Delete to be called")
		}
	})
}
