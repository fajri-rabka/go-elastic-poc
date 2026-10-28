package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fajri-rabka/go-elasticsearch-poc/internal/model"
	"github.com/fajri-rabka/go-elasticsearch-poc/internal/repository/elastic"
	"github.com/fajri-rabka/go-elasticsearch-poc/internal/service"
)

type fakeService struct {
	createFunc  func(ctx context.Context, id string, m model.Motorcycle) error
	getFunc     func(ctx context.Context, id string) (model.Motorcycle, error)
	replaceFunc func(ctx context.Context, id string, m model.Motorcycle) error
	deleteFunc  func(ctx context.Context, id string) error
	searchFunc  func(ctx context.Context, filter model.SearchFilter) (model.SearchResult, error)
}

func (f *fakeService) Create(ctx context.Context, id string, m model.Motorcycle) error {
	if f.createFunc != nil {
		return f.createFunc(ctx, id, m)
	}
	return nil
}

func (f *fakeService) Get(ctx context.Context, id string) (model.Motorcycle, error) {
	if f.getFunc != nil {
		return f.getFunc(ctx, id)
	}
	return model.Motorcycle{}, nil
}

func (f *fakeService) Replace(ctx context.Context, id string, m model.Motorcycle) error {
	if f.replaceFunc != nil {
		return f.replaceFunc(ctx, id, m)
	}
	return nil
}

func (f *fakeService) Delete(ctx context.Context, id string) error {
	if f.deleteFunc != nil {
		return f.deleteFunc(ctx, id)
	}
	return nil
}

func (f *fakeService) Search(ctx context.Context, filter model.SearchFilter) (model.SearchResult, error) {
	if f.searchFunc != nil {
		return f.searchFunc(ctx, filter)
	}
	return model.SearchResult{}, nil
}

type fakeRepo struct{}

func (fakeRepo) Create(ctx context.Context, id string, m model.Motorcycle) error { return nil }
func (fakeRepo) Get(ctx context.Context, id string) (model.Motorcycle, error) {
	return model.Motorcycle{}, nil
}
func (fakeRepo) Replace(ctx context.Context, id string, m model.Motorcycle) error { return nil }
func (fakeRepo) Delete(ctx context.Context, id string) error                      { return nil }
func (fakeRepo) Search(ctx context.Context, filter model.SearchFilter) (model.SearchResult, error) {
	return model.SearchResult{}, nil
}

func setupTestServer(svc MotorcycleService) http.Handler {
	mux := http.NewServeMux()
	h := NewMotorcycleHandler(svc, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.RegisterRoutes(mux)
	return mux
}

func validSampleMotorcycle() model.Motorcycle {
	return model.Motorcycle{
		Brand:        "Honda",
		ModelName:    "Scoopy",
		Category:     "matic",
		EngineCC:     110,
		Transmission: "automatic",
		PriceIDR:     22_000_000,
		Available:    true,
		Description:  "Motor matic Honda harian",
	}
}

func TestHandlerHealth(t *testing.T) {
	handler := setupTestServer(&fakeService{})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want %q", body["status"], "ok")
	}
}

func TestHandlerCreate(t *testing.T) {
	t.Run("POST valid returns 201", func(t *testing.T) {
		svc := &fakeService{
			createFunc: func(ctx context.Context, id string, m model.Motorcycle) error {
				if id != "honda-scoopy-001" {
					t.Errorf("got id %q, want %q", id, "honda-scoopy-001")
				}
				return nil
			},
		}
		handler := setupTestServer(svc)

		payload := `{
			"id": "honda-scoopy-001",
			"brand": "Honda",
			"model_name": "Scoopy",
			"category": "matic",
			"engine_cc": 110,
			"transmission": "automatic",
			"price_idr": 22000000,
			"available": true,
			"description": "Motor matic Honda harian"
		}`
		req := httptest.NewRequest(http.MethodPost, "/motorcycles", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body.String())
		}
		var resp motorcycleResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.ID != "honda-scoopy-001" || resp.ModelName != "Scoopy" {
			t.Errorf("unexpected response: %+v", resp)
		}
	})

	t.Run("POST duplicate returns 409", func(t *testing.T) {
		svc := &fakeService{
			createFunc: func(ctx context.Context, id string, m model.Motorcycle) error {
				return elastic.ErrAlreadyExists
			},
		}
		handler := setupTestServer(svc)

		payload := `{
			"id": "honda-scoopy-001",
			"brand": "Honda",
			"model_name": "Scoopy",
			"category": "matic",
			"engine_cc": 110,
			"transmission": "automatic",
			"price_idr": 22000000,
			"available": true,
			"description": "Motor matic Honda harian"
		}`
		req := httptest.NewRequest(http.MethodPost, "/motorcycles", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
		}
	})

	t.Run("POST missing id returns 400", func(t *testing.T) {
		handler := setupTestServer(&fakeService{})

		payload := `{
			"brand": "Honda",
			"model_name": "Scoopy",
			"category": "matic",
			"engine_cc": 110,
			"transmission": "automatic",
			"price_idr": 22000000,
			"available": true,
			"description": "Motor matic Honda harian"
		}`
		req := httptest.NewRequest(http.MethodPost, "/motorcycles", strings.NewReader(payload))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("POST malformed JSON returns 400", func(t *testing.T) {
		handler := setupTestServer(&fakeService{})

		req := httptest.NewRequest(http.MethodPost, "/motorcycles", strings.NewReader(`{invalid-json`))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("POST unknown field returns 400", func(t *testing.T) {
		handler := setupTestServer(&fakeService{})

		payload := `{
			"id": "honda-scoopy-001",
			"brand": "Honda",
			"model_name": "Scoopy",
			"category": "matic",
			"engine_cc": 110,
			"transmission": "automatic",
			"price_idr": 22000000,
			"available": true,
			"description": "Motor matic Honda harian",
			"extra_field": "disallowed"
		}`
		req := httptest.NewRequest(http.MethodPost, "/motorcycles", strings.NewReader(payload))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("POST multiple JSON objects returns 400", func(t *testing.T) {
		handler := setupTestServer(&fakeService{})

		payload := `{"id":"a"}{"id":"b"}`
		req := httptest.NewRequest(http.MethodPost, "/motorcycles", strings.NewReader(payload))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("POST validation error returns 400", func(t *testing.T) {
		svc := &fakeService{
			createFunc: func(ctx context.Context, id string, m model.Motorcycle) error {
				return &service.ValidationError{Message: `brand must be "Honda"`}
			},
		}
		handler := setupTestServer(svc)

		payload := `{
			"id": "honda-scoopy-001",
			"brand": "Yamaha",
			"model_name": "Scoopy",
			"category": "matic",
			"engine_cc": 110,
			"transmission": "automatic",
			"price_idr": 22000000,
			"available": true,
			"description": "Motor matic Honda harian"
		}`
		req := httptest.NewRequest(http.MethodPost, "/motorcycles", strings.NewReader(payload))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
		var errBody map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
			t.Fatalf("unmarshal error body: %v", err)
		}
		if errBody["error"] != `brand must be "Honda"` {
			t.Errorf("got error message %q, want %q", errBody["error"], `brand must be "Honda"`)
		}
	})
}

func TestHandlerGet(t *testing.T) {
	t.Run("GET existing returns 200", func(t *testing.T) {
		svc := &fakeService{
			getFunc: func(ctx context.Context, id string) (model.Motorcycle, error) {
				if id == "honda-scoopy-001" {
					return validSampleMotorcycle(), nil
				}
				return model.Motorcycle{}, elastic.ErrNotFound
			},
		}
		handler := setupTestServer(svc)

		req := httptest.NewRequest(http.MethodGet, "/motorcycles/honda-scoopy-001", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		var resp motorcycleResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.ID != "honda-scoopy-001" || resp.ModelName != "Scoopy" {
			t.Errorf("unexpected data: %+v", resp)
		}
	})

	t.Run("GET missing returns 404", func(t *testing.T) {
		svc := &fakeService{
			getFunc: func(ctx context.Context, id string) (model.Motorcycle, error) {
				return model.Motorcycle{}, elastic.ErrNotFound
			},
		}
		handler := setupTestServer(svc)

		req := httptest.NewRequest(http.MethodGet, "/motorcycles/non-existent", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("GET datastore unavailable returns 503", func(t *testing.T) {
		svc := &fakeService{
			getFunc: func(ctx context.Context, id string) (model.Motorcycle, error) {
				return model.Motorcycle{}, context.DeadlineExceeded
			},
		}
		handler := setupTestServer(svc)

		req := httptest.NewRequest(http.MethodGet, "/motorcycles/honda-scoopy-001", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
		}
	})
}

func TestHandlerReplace(t *testing.T) {
	t.Run("PUT existing returns 200", func(t *testing.T) {
		svc := &fakeService{
			replaceFunc: func(ctx context.Context, id string, m model.Motorcycle) error {
				if id != "honda-scoopy-001" {
					t.Errorf("got id %q, want %q", id, "honda-scoopy-001")
				}
				if m.PriceIDR != 25_000_000 {
					t.Errorf("got price %d, want %d", m.PriceIDR, 25_000_000)
				}
				return nil
			},
		}
		handler := setupTestServer(svc)

		payload := `{
			"brand": "Honda",
			"model_name": "Scoopy",
			"category": "matic",
			"engine_cc": 110,
			"transmission": "automatic",
			"price_idr": 25000000,
			"available": true,
			"description": "Motor matic Honda harian update"
		}`
		req := httptest.NewRequest(http.MethodPut, "/motorcycles/honda-scoopy-001", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
		}
	})

	t.Run("PUT missing returns 404", func(t *testing.T) {
		svc := &fakeService{
			replaceFunc: func(ctx context.Context, id string, m model.Motorcycle) error {
				return elastic.ErrNotFound
			},
		}
		handler := setupTestServer(svc)

		payload := `{
			"brand": "Honda",
			"model_name": "Scoopy",
			"category": "matic",
			"engine_cc": 110,
			"transmission": "automatic",
			"price_idr": 25000000,
			"available": true,
			"description": "Motor matic Honda harian"
		}`
		req := httptest.NewRequest(http.MethodPut, "/motorcycles/non-existent", strings.NewReader(payload))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("PUT concurrent conflict returns 409", func(t *testing.T) {
		svc := &fakeService{
			replaceFunc: func(ctx context.Context, id string, m model.Motorcycle) error {
				return elastic.ErrConflict
			},
		}
		handler := setupTestServer(svc)

		payload := `{
			"brand": "Honda",
			"model_name": "Scoopy",
			"category": "matic",
			"engine_cc": 110,
			"transmission": "automatic",
			"price_idr": 25000000,
			"available": true,
			"description": "Motor matic Honda harian"
		}`
		req := httptest.NewRequest(http.MethodPut, "/motorcycles/honda-scoopy-001", strings.NewReader(payload))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
		}
	})

	t.Run("PUT invalid payload returns 400", func(t *testing.T) {
		handler := setupTestServer(&fakeService{})

		req := httptest.NewRequest(http.MethodPut, "/motorcycles/honda-scoopy-001", strings.NewReader(`{"brand": 123}`))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})
}

func TestHandlerDelete(t *testing.T) {
	t.Run("DELETE existing returns 204", func(t *testing.T) {
		called := false
		svc := &fakeService{
			deleteFunc: func(ctx context.Context, id string) error {
				called = true
				if id != "honda-scoopy-001" {
					t.Errorf("got id %q, want %q", id, "honda-scoopy-001")
				}
				return nil
			},
		}
		handler := setupTestServer(svc)

		req := httptest.NewRequest(http.MethodDelete, "/motorcycles/honda-scoopy-001", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
		}
		if !called {
			t.Errorf("expected deleteFunc to be called")
		}
	})

	t.Run("DELETE missing returns 404", func(t *testing.T) {
		svc := &fakeService{
			deleteFunc: func(ctx context.Context, id string) error {
				return elastic.ErrNotFound
			},
		}
		handler := setupTestServer(svc)

		req := httptest.NewRequest(http.MethodDelete, "/motorcycles/non-existent", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})
}

func TestWriteError(t *testing.T) {
	h := NewMotorcycleHandler(&fakeService{}, nil)
	rec := httptest.NewRecorder()

	h.writeError(rec, http.StatusBadRequest, "something bad happened")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if body["error"] != "something bad happened" {
		t.Errorf("got error message %q", body["error"])
	}
}

func TestHandlerUnexpectedInternalError(t *testing.T) {
	svc := &fakeService{
		getFunc: func(ctx context.Context, id string) (model.Motorcycle, error) {
			return model.Motorcycle{}, errors.New("unexpected database error with internal info")
		},
	}
	var logBuf bytes.Buffer
	testLogger := slog.New(slog.NewTextHandler(&logBuf, nil))
	h := NewMotorcycleHandler(svc, testLogger)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/motorcycles/honda-scoopy-001", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	// Verify that internal error details are NOT leaked to the response body
	if strings.Contains(rec.Body.String(), "unexpected database error") {
		t.Errorf("internal error details leaked to client: %s", rec.Body.String())
	}
	// Verify that internal error was logged
	if !strings.Contains(logBuf.String(), "unexpected database error") {
		t.Errorf("expected internal error to be logged, got: %s", logBuf.String())
	}
}

func TestHandlerListMotorcycles(t *testing.T) {
	sampleMotorcycle := model.Motorcycle{
		Brand:        "Honda",
		ModelName:    "BeAT",
		Category:     "matic",
		EngineCC:     110,
		Transmission: "automatic",
		PriceIDR:     18_000_000,
		Available:    true,
		Description:  "All New Honda BeAT",
	}

	t.Run("default list returns 200 and formatted response", func(t *testing.T) {
		svc := &fakeService{
			searchFunc: func(ctx context.Context, filter model.SearchFilter) (model.SearchResult, error) {
				if filter.Page != 1 || filter.Limit != 10 {
					t.Errorf("expected default page=1 limit=10, got page=%d limit=%d", filter.Page, filter.Limit)
				}
				return model.SearchResult{
					Items: []model.MotorcycleWithID{
						{ID: "honda-beat-001", Motorcycle: sampleMotorcycle},
					},
					Total:      1,
					Page:       1,
					Limit:      10,
					TotalPages: 1,
				}, nil
			},
		}
		server := setupTestServer(svc)

		req := httptest.NewRequest(http.MethodGet, "/motorcycles", nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d, want %d", rec.Code, http.StatusOK)
		}

		var resp struct {
			Data []struct {
				ID        string `json:"id"`
				ModelName string `json:"model_name"`
			} `json:"data"`
			Pagination struct {
				Page       int   `json:"page"`
				Limit      int   `json:"limit"`
				Total      int64 `json:"total"`
				TotalPages int   `json:"total_pages"`
			} `json:"pagination"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if len(resp.Data) != 1 || resp.Data[0].ID != "honda-beat-001" {
			t.Errorf("unexpected data: %+v", resp.Data)
		}
		if resp.Pagination.Total != 1 || resp.Pagination.TotalPages != 1 {
			t.Errorf("unexpected pagination: %+v", resp.Pagination)
		}
	})

	t.Run("parses query parameters correctly", func(t *testing.T) {
		var capturedFilter model.SearchFilter
		svc := &fakeService{
			searchFunc: func(ctx context.Context, filter model.SearchFilter) (model.SearchResult, error) {
				capturedFilter = filter
				return model.SearchResult{Items: []model.MotorcycleWithID{}, Total: 0, Page: filter.Page, Limit: filter.Limit, TotalPages: 0}, nil
			},
		}
		server := setupTestServer(svc)

		req := httptest.NewRequest(http.MethodGet, "/motorcycles?search=beat&category=matic&transmission=automatic&available=true&min_price=15000000&max_price=25000000&sort=price_asc&page=2&limit=5", nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d, want %d", rec.Code, http.StatusOK)
		}
		if capturedFilter.Search != "beat" {
			t.Errorf("search = %q, want 'beat'", capturedFilter.Search)
		}
		if capturedFilter.Category != "matic" {
			t.Errorf("category = %q, want 'matic'", capturedFilter.Category)
		}
		if capturedFilter.Transmission != "automatic" {
			t.Errorf("transmission = %q, want 'automatic'", capturedFilter.Transmission)
		}
		if capturedFilter.Available == nil || !*capturedFilter.Available {
			t.Errorf("available = %v, want true", capturedFilter.Available)
		}
		if capturedFilter.MinPrice == nil || *capturedFilter.MinPrice != 15_000_000 {
			t.Errorf("min_price = %v, want 15000000", capturedFilter.MinPrice)
		}
		if capturedFilter.MaxPrice == nil || *capturedFilter.MaxPrice != 25_000_000 {
			t.Errorf("max_price = %v, want 25000000", capturedFilter.MaxPrice)
		}
		if capturedFilter.Sort != "price_asc" {
			t.Errorf("sort = %q, want 'price_asc'", capturedFilter.Sort)
		}
		if capturedFilter.Page != 2 {
			t.Errorf("page = %d, want 2", capturedFilter.Page)
		}
		if capturedFilter.Limit != 5 {
			t.Errorf("limit = %d, want 5", capturedFilter.Limit)
		}
	})

	t.Run("empty results serializes data as empty array not null", func(t *testing.T) {
		svc := &fakeService{
			searchFunc: func(ctx context.Context, filter model.SearchFilter) (model.SearchResult, error) {
				return model.SearchResult{
					Items:      []model.MotorcycleWithID{},
					Total:      0,
					Page:       1,
					Limit:      10,
					TotalPages: 0,
				}, nil
			},
		}
		server := setupTestServer(svc)

		req := httptest.NewRequest(http.MethodGet, "/motorcycles", nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d, want %d", rec.Code, http.StatusOK)
		}

		rawBody := rec.Body.String()
		if !strings.Contains(rawBody, `"data":[]`) {
			t.Errorf("expected JSON to contain '\"data\":[]', got %s", rawBody)
		}
	})

	invalidParams := []struct {
		name       string
		url        string
		wantSubstr string
	}{
		{name: "invalid available", url: "/motorcycles?available=maybe", wantSubstr: "invalid available"},
		{name: "invalid min_price", url: "/motorcycles?min_price=abc", wantSubstr: "invalid min_price"},
		{name: "invalid max_price", url: "/motorcycles?max_price=abc", wantSubstr: "invalid max_price"},
		{name: "invalid page", url: "/motorcycles?page=abc", wantSubstr: "invalid page"},
		{name: "invalid limit", url: "/motorcycles?limit=abc", wantSubstr: "invalid limit"},
	}

	for _, tc := range invalidParams {
		t.Run(tc.name, func(t *testing.T) {
			server := setupTestServer(&fakeService{})
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			rec := httptest.NewRecorder()
			server.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("code = %d, want %d", rec.Code, http.StatusBadRequest)
			}
			if !strings.Contains(rec.Body.String(), tc.wantSubstr) {
				t.Errorf("response body %q does not contain %q", rec.Body.String(), tc.wantSubstr)
			}
		})
	}

	t.Run("service validation errors return 400", func(t *testing.T) {
		realSvc := service.NewMotorcycleService(&fakeRepo{})
		server := setupTestServer(realSvc)

		cases := []struct {
			name       string
			url        string
			wantSubstr string
		}{
			{name: "page 0", url: "/motorcycles?page=0", wantSubstr: "page must be greater than or equal to 1"},
			{name: "limit 0", url: "/motorcycles?limit=0", wantSubstr: "limit must be between 1 and 50"},
			{name: "limit 51", url: "/motorcycles?limit=51", wantSubstr: "limit must be between 1 and 50"},
			{name: "min_price > max_price", url: "/motorcycles?min_price=20000000&max_price=10000000", wantSubstr: "min_price cannot be greater than max_price"},
			{name: "invalid sort", url: "/motorcycles?sort=random", wantSubstr: "invalid sort option"},
			{name: "relevance without search", url: "/motorcycles?sort=relevance", wantSubstr: "sort=relevance is only valid when a search query is provided"},
			{name: "result window overflow", url: "/motorcycles?page=1001&limit=10", wantSubstr: "exceed maximum result window of 10000"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, tc.url, nil)
				rec := httptest.NewRecorder()
				server.ServeHTTP(rec, req)

				if rec.Code != http.StatusBadRequest {
					t.Fatalf("code = %d, want %d", rec.Code, http.StatusBadRequest)
				}
				if !strings.Contains(rec.Body.String(), tc.wantSubstr) {
					t.Errorf("response body %q does not contain %q", rec.Body.String(), tc.wantSubstr)
				}
			})
		}
	})

	t.Run("datastore unavailable returns 503", func(t *testing.T) {
		svc := &fakeService{
			searchFunc: func(ctx context.Context, filter model.SearchFilter) (model.SearchResult, error) {
				return model.SearchResult{}, context.DeadlineExceeded
			},
		}
		server := setupTestServer(svc)

		req := httptest.NewRequest(http.MethodGet, "/motorcycles", nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("code = %d, want %d", rec.Code, http.StatusServiceUnavailable)
		}
		if !strings.Contains(rec.Body.String(), "datastore service unavailable") {
			t.Errorf("response body %q does not contain expected message", rec.Body.String())
		}
	})

	t.Run("route disambiguation GET /motorcycles vs GET /motorcycles/{id}", func(t *testing.T) {
		listCalled := false
		getCalled := false

		svc := &fakeService{
			searchFunc: func(ctx context.Context, filter model.SearchFilter) (model.SearchResult, error) {
				listCalled = true
				return model.SearchResult{Items: []model.MotorcycleWithID{}, Total: 0, Page: 1, Limit: 10, TotalPages: 0}, nil
			},
			getFunc: func(ctx context.Context, id string) (model.Motorcycle, error) {
				getCalled = true
				if id != "honda-beat-001" {
					t.Errorf("got id %q, want honda-beat-001", id)
				}
				return sampleMotorcycle, nil
			},
		}
		server := setupTestServer(svc)

		// Request 1: GET /motorcycles
		req1 := httptest.NewRequest(http.MethodGet, "/motorcycles", nil)
		rec1 := httptest.NewRecorder()
		server.ServeHTTP(rec1, req1)
		if rec1.Code != http.StatusOK || !listCalled {
			t.Fatalf("expected list endpoint to be called, code=%d, listCalled=%v", rec1.Code, listCalled)
		}

		// Request 2: GET /motorcycles/honda-beat-001
		req2 := httptest.NewRequest(http.MethodGet, "/motorcycles/honda-beat-001", nil)
		rec2 := httptest.NewRecorder()
		server.ServeHTTP(rec2, req2)
		if rec2.Code != http.StatusOK || !getCalled {
			t.Fatalf("expected get endpoint to be called, code=%d, getCalled=%v", rec2.Code, getCalled)
		}
	})
}
