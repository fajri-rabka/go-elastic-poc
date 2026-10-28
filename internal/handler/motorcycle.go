// Package handler implements HTTP REST API endpoints for the motorcycle catalog.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/fajri-rabka/go-elasticsearch-poc/internal/model"
	"github.com/fajri-rabka/go-elasticsearch-poc/internal/repository/elastic"
	"github.com/fajri-rabka/go-elasticsearch-poc/internal/service"
)

const maxRequestBodyBytes = 1 << 20 // 1 MiB

// MotorcycleService represents the business operations required by the handler.
type MotorcycleService interface {
	Create(ctx context.Context, id string, m model.Motorcycle) error
	Get(ctx context.Context, id string) (model.Motorcycle, error)
	Replace(ctx context.Context, id string, m model.Motorcycle) error
	Delete(ctx context.Context, id string) error
	Search(ctx context.Context, filter model.SearchFilter) (model.SearchResult, error)
}

// MotorcycleHandler routes and handles HTTP requests for motorcycles.
type MotorcycleHandler struct {
	svc    MotorcycleService
	logger *slog.Logger
}

func NewMotorcycleHandler(svc MotorcycleService, logger *slog.Logger) *MotorcycleHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &MotorcycleHandler{svc: svc, logger: logger}
}

// RegisterRoutes registers all motorcycle endpoints onto the given ServeMux.
func (h *MotorcycleHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /motorcycles", h.Create)
	mux.HandleFunc("GET /motorcycles", h.List)
	mux.HandleFunc("GET /motorcycles/{id}", h.Get)
	mux.HandleFunc("PUT /motorcycles/{id}", h.Replace)
	mux.HandleFunc("DELETE /motorcycles/{id}", h.Delete)
	mux.HandleFunc("GET /health", h.Health)
}

// createRequest represents the JSON payload for POST /motorcycles.
// Pointers distinguish missing fields from zero values (e.g. available: false).
type createRequest struct {
	ID           *string `json:"id"`
	Brand        *string `json:"brand"`
	ModelName    *string `json:"model_name"`
	Category     *string `json:"category"`
	EngineCC     *int    `json:"engine_cc"`
	Transmission *string `json:"transmission"`
	PriceIDR     *int64  `json:"price_idr"`
	Available    *bool   `json:"available"`
	Description  *string `json:"description"`
}

// replaceRequest represents the JSON payload for PUT /motorcycles/{id}.
// The ID is supplied via the URL path parameter, not in the body.
type replaceRequest struct {
	Brand        *string `json:"brand"`
	ModelName    *string `json:"model_name"`
	Category     *string `json:"category"`
	EngineCC     *int    `json:"engine_cc"`
	Transmission *string `json:"transmission"`
	PriceIDR     *int64  `json:"price_idr"`
	Available    *bool   `json:"available"`
	Description  *string `json:"description"`
}

// motorcycleResponse is the standard response for GET /motorcycles/{id}.
type motorcycleResponse struct {
	ID           string `json:"id"`
	Brand        string `json:"brand"`
	ModelName    string `json:"model_name"`
	Category     string `json:"category"`
	EngineCC     int    `json:"engine_cc"`
	Transmission string `json:"transmission"`
	PriceIDR     int64  `json:"price_idr"`
	Available    bool   `json:"available"`
	Description  string `json:"description"`
}

// listResponse represents the paginated response for GET /motorcycles.
type listResponse struct {
	Data       []motorcycleResponse `json:"data"`
	Pagination paginationResponse   `json:"pagination"`
}

// paginationResponse provides pagination metadata.
type paginationResponse struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// Create handles POST /motorcycles.
func (h *MotorcycleHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.ID == nil || strings.TrimSpace(*req.ID) == "" {
		h.writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	m, err := extractMotorcycle(req.Brand, req.ModelName, req.Category, req.EngineCC, req.Transmission, req.PriceIDR, req.Available, req.Description)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	id := strings.TrimSpace(*req.ID)
	if err := h.svc.Create(r.Context(), id, m); err != nil {
		h.handleDomainError(w, err, fmt.Sprintf("create motorcycle %q", id))
		return
	}

	resp := toResponse(id, m)
	h.writeJSON(w, http.StatusCreated, resp)
}

// List handles GET /motorcycles (full-text search, filtering, sorting, pagination).
func (h *MotorcycleHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	filter := model.SearchFilter{
		Search:       q.Get("search"),
		Category:     q.Get("category"),
		Transmission: q.Get("transmission"),
		Sort:         q.Get("sort"),
		Page:         1,
		Limit:        10,
	}

	if q.Has("available") {
		b, err := strconv.ParseBool(q.Get("available"))
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid available parameter: must be true or false")
			return
		}
		filter.Available = &b
	}

	if q.Has("min_price") {
		v, err := strconv.ParseInt(q.Get("min_price"), 10, 64)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid min_price parameter: must be an integer")
			return
		}
		filter.MinPrice = &v
	}

	if q.Has("max_price") {
		v, err := strconv.ParseInt(q.Get("max_price"), 10, 64)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid max_price parameter: must be an integer")
			return
		}
		filter.MaxPrice = &v
	}

	if q.Has("page") {
		p, err := strconv.Atoi(q.Get("page"))
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid page parameter: must be an integer")
			return
		}
		filter.Page = p
	}

	if q.Has("limit") {
		l, err := strconv.Atoi(q.Get("limit"))
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid limit parameter: must be an integer")
			return
		}
		filter.Limit = l
	}

	res, err := h.svc.Search(r.Context(), filter)
	if err != nil {
		h.handleDomainError(w, err, "search motorcycles")
		return
	}

	data := make([]motorcycleResponse, 0, len(res.Items))
	for _, item := range res.Items {
		data = append(data, toResponse(item.ID, item.Motorcycle))
	}

	resp := listResponse{
		Data: data,
		Pagination: paginationResponse{
			Page:       res.Page,
			Limit:      res.Limit,
			Total:      res.Total,
			TotalPages: res.TotalPages,
		},
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// Get handles GET /motorcycles/{id}.
func (h *MotorcycleHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if strings.TrimSpace(id) == "" {
		h.writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	m, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.handleDomainError(w, err, fmt.Sprintf("get motorcycle %q", id))
		return
	}

	h.writeJSON(w, http.StatusOK, toResponse(id, m))
}

// Replace handles PUT /motorcycles/{id} (full document replacement).
func (h *MotorcycleHandler) Replace(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if strings.TrimSpace(id) == "" {
		h.writeError(w, http.StatusBadRequest, "id is required in url path")
		return
	}

	var req replaceRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	m, err := extractMotorcycle(req.Brand, req.ModelName, req.Category, req.EngineCC, req.Transmission, req.PriceIDR, req.Available, req.Description)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.svc.Replace(r.Context(), id, m); err != nil {
		h.handleDomainError(w, err, fmt.Sprintf("replace motorcycle %q", id))
		return
	}

	h.writeJSON(w, http.StatusOK, toResponse(id, m))
}

// Delete handles DELETE /motorcycles/{id}.
func (h *MotorcycleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if strings.TrimSpace(id) == "" {
		h.writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		h.handleDomainError(w, err, fmt.Sprintf("delete motorcycle %q", id))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Health handles GET /health.
func (h *MotorcycleHandler) Health(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *MotorcycleHandler) handleDomainError(w http.ResponseWriter, err error, action string) {
	var valErr *service.ValidationError
	if errors.As(err, &valErr) {
		h.writeError(w, http.StatusBadRequest, valErr.Error())
		return
	}

	switch {
	case errors.Is(err, elastic.ErrNotFound):
		h.writeError(w, http.StatusNotFound, "motorcycle not found")
	case errors.Is(err, elastic.ErrAlreadyExists):
		h.writeError(w, http.StatusConflict, "motorcycle already exists")
	case errors.Is(err, elastic.ErrConflict):
		h.writeError(w, http.StatusConflict, "document version conflict: modified concurrently")
	case elastic.IsUnavailable(err):
		h.logger.Error("datastore dependency unavailable", "action", action, "error", err)
		h.writeError(w, http.StatusServiceUnavailable, "datastore service unavailable")
	default:
		h.logger.Error("unexpected internal error", "action", action, "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func (h *MotorcycleHandler) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, map[string]string{"error": message})
}

func (h *MotorcycleHandler) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	ct := r.Header.Get("Content-Type")
	if ct != "" && !strings.HasPrefix(strings.ToLower(ct), "application/json") {
		return fmt.Errorf("Content-Type must be application/json")
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(target); err != nil {
		var syntaxErr *json.SyntaxError
		var unmarshalTypeErr *json.UnmarshalTypeError
		switch {
		case errors.As(err, &syntaxErr):
			return fmt.Errorf("malformed JSON at byte offset %d", syntaxErr.Offset)
		case errors.As(err, &unmarshalTypeErr):
			return fmt.Errorf("invalid value for field %q (expected %s)", unmarshalTypeErr.Field, unmarshalTypeErr.Type)
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			return fmt.Errorf("unknown field: %s", strings.TrimPrefix(err.Error(), "json: unknown field "))
		case errors.Is(err, io.EOF):
			return fmt.Errorf("request body must not be empty")
		default:
			return fmt.Errorf("invalid JSON payload: %w", err)
		}
	}

	if dec.More() {
		return fmt.Errorf("request body must contain a single JSON object")
	}

	return nil
}

func extractMotorcycle(brand, modelName, category *string, engineCC *int, transmission *string, priceIDR *int64, available *bool, description *string) (model.Motorcycle, error) {
	if brand == nil {
		return model.Motorcycle{}, fmt.Errorf("brand is required")
	}
	if modelName == nil {
		return model.Motorcycle{}, fmt.Errorf("model_name is required")
	}
	if category == nil {
		return model.Motorcycle{}, fmt.Errorf("category is required")
	}
	if engineCC == nil {
		return model.Motorcycle{}, fmt.Errorf("engine_cc is required")
	}
	if transmission == nil {
		return model.Motorcycle{}, fmt.Errorf("transmission is required")
	}
	if priceIDR == nil {
		return model.Motorcycle{}, fmt.Errorf("price_idr is required")
	}
	if available == nil {
		return model.Motorcycle{}, fmt.Errorf("available is required")
	}
	if description == nil {
		return model.Motorcycle{}, fmt.Errorf("description is required")
	}

	return model.Motorcycle{
		Brand:        *brand,
		ModelName:    *modelName,
		Category:     *category,
		EngineCC:     *engineCC,
		Transmission: *transmission,
		PriceIDR:     *priceIDR,
		Available:    *available,
		Description:  *description,
	}, nil
}

func toResponse(id string, m model.Motorcycle) motorcycleResponse {
	return motorcycleResponse{
		ID:           id,
		Brand:        m.Brand,
		ModelName:    m.ModelName,
		Category:     m.Category,
		EngineCC:     m.EngineCC,
		Transmission: m.Transmission,
		PriceIDR:     m.PriceIDR,
		Available:    m.Available,
		Description:  m.Description,
	}
}
