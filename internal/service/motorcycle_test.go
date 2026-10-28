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
	searchFunc  func(ctx context.Context, filter model.SearchFilter) (model.SearchResult, error)
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

func (f *fakeRepo) Search(ctx context.Context, filter model.SearchFilter) (model.SearchResult, error) {
	if f.searchFunc != nil {
		return f.searchFunc(ctx, filter)
	}
	return model.SearchResult{}, nil
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

func TestValidateSearchFilter(t *testing.T) {
	svc := NewMotorcycleService(&fakeRepo{})

	int64Ptr := func(v int64) *int64 { return &v }

	tests := []struct {
		name     string
		filter   model.SearchFilter
		wantErr  string
		wantSort string
	}{
		{
			name: "valid default without search defaults to name_asc",
			filter: model.SearchFilter{
				Page:  1,
				Limit: 10,
			},
			wantSort: "name_asc",
		},
		{
			name: "valid with search defaults to relevance",
			filter: model.SearchFilter{
				Search: "beat",
				Page:   1,
				Limit:  10,
			},
			wantSort: "relevance",
		},
		{
			name: "valid explicit sort",
			filter: model.SearchFilter{
				Sort:  "price_desc",
				Page:  1,
				Limit: 10,
			},
			wantSort: "price_desc",
		},
		{
			name: "relevance sort without search query fails",
			filter: model.SearchFilter{
				Sort:  "relevance",
				Page:  1,
				Limit: 10,
			},
			wantErr: "sort=relevance is only valid when a search query is provided",
		},
		{
			name: "invalid sort option fails",
			filter: model.SearchFilter{
				Sort:  "speed_asc",
				Page:  1,
				Limit: 10,
			},
			wantErr: "invalid sort option",
		},
		{
			name: "negative min_price fails",
			filter: model.SearchFilter{
				MinPrice: int64Ptr(-1),
				Page:     1,
				Limit:    10,
			},
			wantErr: "min_price must be greater than or equal to 0",
		},
		{
			name: "negative max_price fails",
			filter: model.SearchFilter{
				MaxPrice: int64Ptr(-100),
				Page:     1,
				Limit:    10,
			},
			wantErr: "max_price must be greater than or equal to 0",
		},
		{
			name: "min_price greater than max_price fails",
			filter: model.SearchFilter{
				MinPrice: int64Ptr(30_000_000),
				MaxPrice: int64Ptr(20_000_000),
				Page:     1,
				Limit:    10,
			},
			wantErr: "min_price cannot be greater than max_price",
		},
		{
			name: "page 0 fails",
			filter: model.SearchFilter{
				Page:  0,
				Limit: 10,
			},
			wantErr: "page must be greater than or equal to 1",
		},
		{
			name: "negative page fails",
			filter: model.SearchFilter{
				Page:  -5,
				Limit: 10,
			},
			wantErr: "page must be greater than or equal to 1",
		},
		{
			name: "limit 0 fails",
			filter: model.SearchFilter{
				Page:  1,
				Limit: 0,
			},
			wantErr: "limit must be between 1 and 50",
		},
		{
			name: "limit greater than 50 fails",
			filter: model.SearchFilter{
				Page:  1,
				Limit: 51,
			},
			wantErr: "limit must be between 1 and 50",
		},
		{
			name: "result window exceeds 10000 fails",
			filter: model.SearchFilter{
				Page:  1001,
				Limit: 10,
			},
			wantErr: "page offset and limit exceed maximum result window of 10000",
		},
		{
			name: "result window boundary 10000 succeeds",
			filter: model.SearchFilter{
				Page:  200,
				Limit: 50,
			},
			wantSort: "name_asc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.filter
			err := svc.ValidateSearchFilter(&f)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ValidateSearchFilter() error = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantSort != "" && f.Sort != tt.wantSort {
				t.Errorf("got sort %q, want %q", f.Sort, tt.wantSort)
			}
		})
	}
}

func TestServiceSearchOperation(t *testing.T) {
	ctx := context.Background()

	t.Run("Search calls repo on valid filter", func(t *testing.T) {
		called := false
		svc := NewMotorcycleService(&fakeRepo{
			searchFunc: func(ctx context.Context, filter model.SearchFilter) (model.SearchResult, error) {
				called = true
				if filter.Sort != "price_desc" {
					t.Errorf("got sort %q, want %q", filter.Sort, "price_desc")
				}
				return model.SearchResult{
					Items: []model.MotorcycleWithID{
						{ID: "honda-beat-001", Motorcycle: validMotorcycle()},
					},
					Total:      1,
					Page:       1,
					Limit:      10,
					TotalPages: 1,
				}, nil
			},
		})

		filter := model.SearchFilter{
			Sort:  "price_desc",
			Page:  1,
			Limit: 10,
		}
		res, err := svc.Search(ctx, filter)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !called {
			t.Fatalf("expected repo.Search to be called")
		}
		if res.Total != 1 || len(res.Items) != 1 {
			t.Errorf("unexpected result: %+v", res)
		}
	})

	t.Run("Search rejects invalid filter before calling repo", func(t *testing.T) {
		called := false
		svc := NewMotorcycleService(&fakeRepo{
			searchFunc: func(ctx context.Context, filter model.SearchFilter) (model.SearchResult, error) {
				called = true
				return model.SearchResult{}, nil
			},
		})

		filter := model.SearchFilter{
			Page:  0, // Invalid
			Limit: 10,
		}
		_, err := svc.Search(ctx, filter)
		if err == nil {
			t.Fatalf("expected error for invalid page")
		}
		if called {
			t.Errorf("repo should not be called when validation fails")
		}
	})
}
