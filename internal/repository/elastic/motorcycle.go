package elastic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"slices"
	"strconv"
	"strings"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/dynamicmapping"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/result"

	"github.com/fajri-rabka/go-elasticsearch-poc/internal/model"
)

var (
	// ErrAlreadyExists means a document with the same _id is already stored.
	ErrAlreadyExists = errors.New("document already exists")
	// ErrNotFound means no document exists for the requested _id.
	ErrNotFound = errors.New("document not found")
	// ErrConflict means a concurrent update changed the document before this operation could finish.
	ErrConflict = errors.New("document version conflict")
)

// MotorcycleRepository stores motorcycles as documents in a single index.
type MotorcycleRepository struct {
	client *elasticsearch.TypedClient
	index  string
}

func NewMotorcycleRepository(client *elasticsearch.TypedClient, index string) *MotorcycleRepository {
	return &MotorcycleRepository{client: client, index: index}
}

// EnsureIndex creates the index with the motorcycle settings and mapping if it
// does not exist yet, and reports whether it was created. An existing index is
// never recreated or changed; its mapping is verified instead, and a mismatch
// is returned as an error.
func (r *MotorcycleRepository) EnsureIndex(ctx context.Context) (bool, error) {
	// Create first and treat "already exists" as a normal outcome. Compared to
	// exists-then-create this is one request and has no race between the two.
	res, err := r.client.Indices.Create(r.index).
		Settings(motorcycleIndexSettings()).
		Mappings(motorcycleMapping()).
		Do(ctx)
	if err != nil {
		if hasErrorType(err, "resource_already_exists_exception") {
			return false, r.verifyMapping(ctx)
		}
		return false, fmt.Errorf("create index %q: %w", r.index, err)
	}
	if !res.Acknowledged {
		return false, fmt.Errorf("create index %q: request was not acknowledged by the cluster", r.index)
	}

	return true, nil
}

// Create stores m under id. It uses the create operation instead of index, so
// a document that already has this _id is never overwritten; ErrAlreadyExists
// is returned instead.
func (r *MotorcycleRepository) Create(ctx context.Context, id string, m model.Motorcycle) error {
	if _, err := r.client.Create(r.index, id).Document(m).Do(ctx); err != nil {
		if hasErrorType(err, "version_conflict_engine_exception") {
			return fmt.Errorf("create motorcycle %q: %w", id, ErrAlreadyExists)
		}
		return fmt.Errorf("create motorcycle %q: %w", id, err)
	}

	return nil
}

// Get reads a motorcycle by _id. GET by ID is real-time: it sees the latest
// write even before the index is refreshed, so callers do not need a refresh
// or a sleep right after Create. Search does not behave this way.
func (r *MotorcycleRepository) Get(ctx context.Context, id string) (model.Motorcycle, error) {
	res, err := r.client.Get(r.index, id).Do(ctx)
	if err != nil {
		return model.Motorcycle{}, fmt.Errorf("get motorcycle %q: %w", id, err)
	}
	if !res.Found {
		return model.Motorcycle{}, fmt.Errorf("get motorcycle %q: %w", id, ErrNotFound)
	}

	var m model.Motorcycle
	if err := json.Unmarshal(res.Source_, &m); err != nil {
		return model.Motorcycle{}, fmt.Errorf("decode motorcycle %q: %w", id, err)
	}

	return m, nil
}

// Replace performs a full document replacement for an existing motorcycle.
// It verifies the document exists first and uses Elasticsearch optimistic
// concurrency control (if_seq_no and if_primary_term) to ensure no accidental
// upsert happens and concurrent writes are rejected with ErrConflict.
func (r *MotorcycleRepository) Replace(ctx context.Context, id string, m model.Motorcycle) error {
	getRes, err := r.client.Get(r.index, id).Do(ctx)
	if err != nil {
		return fmt.Errorf("check existing motorcycle %q for replacement: %w", id, err)
	}
	if !getRes.Found {
		return fmt.Errorf("replace motorcycle %q: %w", id, ErrNotFound)
	}
	if getRes.SeqNo_ == nil || getRes.PrimaryTerm_ == nil {
		return fmt.Errorf("replace motorcycle %q: missing sequence number or primary term", id)
	}

	_, err = r.client.Index(r.index).
		Id(id).
		IfSeqNo(strconv.FormatInt(*getRes.SeqNo_, 10)).
		IfPrimaryTerm(strconv.FormatInt(*getRes.PrimaryTerm_, 10)).
		Document(m).
		Do(ctx)
	if err != nil {
		if hasErrorType(err, "version_conflict_engine_exception") {
			return fmt.Errorf("replace motorcycle %q: %w", id, ErrConflict)
		}
		return fmt.Errorf("replace motorcycle %q: %w", id, err)
	}

	return nil
}

// Delete removes the document with the given id. If the document does not exist,
// it returns ErrNotFound.
func (r *MotorcycleRepository) Delete(ctx context.Context, id string) error {
	res, err := r.client.Delete(r.index, id).Do(ctx)
	if err != nil {
		return fmt.Errorf("delete motorcycle %q: %w", id, err)
	}
	if res.Result == result.Notfound || res.Result.String() == "not_found" {
		return fmt.Errorf("delete motorcycle %q: %w", id, ErrNotFound)
	}

	return nil
}

// IsUnavailable reports whether an error indicates that the datastore cannot
// be reached or timed out, representing a dependency failure (HTTP 503).
func IsUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var esErr *types.ElasticsearchError
	if errors.As(err, &esErr) {
		return esErr.Status == 502 || esErr.Status == 503 || esErr.Status == 504
	}
	return false
}

func (r *MotorcycleRepository) verifyMapping(ctx context.Context) error {
	res, err := r.client.Indices.GetMapping().Index(r.index).Do(ctx)
	if err != nil {
		return fmt.Errorf("get mapping of index %q: %w", r.index, err)
	}
	record, ok := res[r.index]
	if !ok {
		return fmt.Errorf("get mapping of index %q: index not found in response", r.index)
	}

	problems, err := mappingProblems(motorcycleMapping(), &record.Mappings)
	if err != nil {
		return fmt.Errorf("compare mapping of index %q: %w", r.index, err)
	}
	if len(problems) > 0 {
		return fmt.Errorf("index %q already exists with a different mapping and was left unchanged: %s",
			r.index, strings.Join(problems, "; "))
	}

	return nil
}

// motorcycleMapping is the single source of truth for the index mapping. It is
// used both to create the index and to verify an existing one.
func motorcycleMapping() *types.TypeMapping {
	return esdsl.NewTypeMapping().
		// Reject documents with unknown fields instead of letting Elasticsearch
		// guess a type and silently extend the mapping.
		Dynamic(dynamicmapping.Strict).
		AddProperty("brand", esdsl.NewKeywordProperty()).
		// The keyword sub-field prepares exact match, sorting, and aggregations
		// on model names (Phase 5) without having to reindex later.
		AddProperty("model_name", esdsl.NewTextProperty().
			AddField("keyword", esdsl.NewKeywordProperty())).
		AddProperty("category", esdsl.NewKeywordProperty()).
		AddProperty("engine_cc", esdsl.NewIntegerNumberProperty()).
		AddProperty("transmission", esdsl.NewKeywordProperty()).
		AddProperty("price_idr", esdsl.NewLongNumberProperty()).
		AddProperty("available", esdsl.NewBooleanProperty()).
		AddProperty("description", esdsl.NewTextProperty()).
		TypeMappingCaster()
}

func motorcycleIndexSettings() *types.IndexSettings {
	// A single-node cluster can never place a replica, so any replica would stay
	// unassigned and turn cluster health yellow.
	return esdsl.NewIndexSettings().
		NumberOfShards("1").
		NumberOfReplicas("0").
		IndexSettingsCaster()
}

// mappingProblems lists the differences between the wanted and the actual
// mapping that matter for this index: the dynamic setting and every field
// type, including sub-fields. An empty result means the mapping is compatible.
func mappingProblems(want, got *types.TypeMapping) ([]string, error) {
	wantFields, err := fieldTypes(want.Properties)
	if err != nil {
		return nil, err
	}
	gotFields, err := fieldTypes(got.Properties)
	if err != nil {
		return nil, err
	}

	var problems []string
	if w, g := dynamicSetting(want.Dynamic), dynamicSetting(got.Dynamic); w != g {
		problems = append(problems, fmt.Sprintf("dynamic is %q, want %q", g, w))
	}
	for _, name := range slices.Sorted(maps.Keys(wantFields)) {
		gotType, ok := gotFields[name]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("field %q is missing, want %s", name, wantFields[name]))
		case gotType != wantFields[name]:
			problems = append(problems, fmt.Sprintf("field %q is %s, want %s", name, gotType, wantFields[name]))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(gotFields)) {
		if _, ok := wantFields[name]; !ok {
			problems = append(problems, fmt.Sprintf("unexpected field %q (%s)", name, gotFields[name]))
		}
	}

	return problems, nil
}

// fieldTypes flattens properties into "field" / "field.subfield" -> type.
// types.Property is an interface with dozens of implementations, so reading
// the "type" attribute from their JSON form is the simplest generic way to
// compare them.
func fieldTypes(props map[string]types.Property) (map[string]string, error) {
	type field struct {
		Type   string           `json:"type"`
		Fields map[string]field `json:"fields"`
	}

	raw, err := json.Marshal(props)
	if err != nil {
		return nil, fmt.Errorf("encode properties: %w", err)
	}
	var fields map[string]field
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("decode properties: %w", err)
	}

	flat := make(map[string]string)
	for name, f := range fields {
		flat[name] = f.Type
		for subName, sub := range f.Fields {
			flat[name+"."+subName] = sub.Type
		}
	}

	return flat, nil
}

func dynamicSetting(d *dynamicmapping.DynamicMapping) string {
	if d == nil {
		// Elasticsearch's default when the mapping does not set it.
		return dynamicmapping.True.String()
	}
	return d.String()
}

// hasErrorType reports whether err is an Elasticsearch error response of the
// given type, such as "resource_already_exists_exception". It supports both
// pointer and value targets for robustness.
func hasErrorType(err error, errorType string) bool {
	var esErr *types.ElasticsearchError
	if errors.As(err, &esErr) && esErr.ErrorCause.Type == errorType {
		return true
	}
	var esVal types.ElasticsearchError
	if errors.As(err, &esVal) && esVal.ErrorCause.Type == errorType {
		return true
	}
	return false
}
