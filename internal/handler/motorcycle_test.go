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
