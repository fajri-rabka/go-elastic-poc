package elastic

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
)

func TestMotorcycleMapping(t *testing.T) {
	want := map[string]string{
		"brand":              "keyword",
		"model_name":         "text",
		"model_name.keyword": "keyword",
		"category":           "keyword",
		"engine_cc":          "integer",
		"transmission":       "keyword",
		"price_idr":          "long",
		"available":          "boolean",
		"description":        "text",
	}

	mapping := motorcycleMapping()
	got, err := fieldTypes(mapping.Properties)
	if err != nil {
		t.Fatalf("fieldTypes: %v", err)
	}

	if !maps.Equal(got, want) {
		t.Errorf("field types = %v, want %v", got, want)
	}
	if d := dynamicSetting(mapping.Dynamic); d != "strict" {
		t.Errorf("dynamic = %q, want %q", d, "strict")
	}
}

// The JSON below has the shape Elasticsearch returns from GET <index>/_mapping
// for an index created with motorcycleMapping. It guards the assumption that a
// decoded server mapping compares equal to the one we build in Go.
func TestMappingProblemsAcceptsServerMapping(t *testing.T) {
	got := decodeMapping(t, `{
		"dynamic": "strict",
		"properties": {
			"available":    {"type": "boolean"},
			"brand":        {"type": "keyword"},
			"category":     {"type": "keyword"},
			"description":  {"type": "text"},
			"engine_cc":    {"type": "integer"},
			"model_name":   {"type": "text", "fields": {"keyword": {"type": "keyword"}}},
			"price_idr":    {"type": "long"},
			"transmission": {"type": "keyword"}
		}
	}`)

	problems, err := mappingProblems(motorcycleMapping(), got)
	if err != nil {
		t.Fatalf("mappingProblems: %v", err)
	}
	if len(problems) != 0 {
		t.Errorf("problems = %q, want none", problems)
	}
}

func TestMappingProblemsReportsDifferences(t *testing.T) {
	// No dynamic setting, price_idr as integer, description missing,
	// model_name without its keyword sub-field, and an extra color field.
	got := decodeMapping(t, `{
		"properties": {
			"available":    {"type": "boolean"},
			"brand":        {"type": "keyword"},
			"category":     {"type": "keyword"},
			"color":        {"type": "keyword"},
			"engine_cc":    {"type": "integer"},
			"model_name":   {"type": "text"},
			"price_idr":    {"type": "integer"},
			"transmission": {"type": "keyword"}
		}
	}`)

	problems, err := mappingProblems(motorcycleMapping(), got)
	if err != nil {
		t.Fatalf("mappingProblems: %v", err)
	}

	want := []string{
		`dynamic is "true", want "strict"`,
		`field "description" is missing, want text`,
		`field "model_name.keyword" is missing, want keyword`,
		`field "price_idr" is integer, want long`,
		`unexpected field "color" (keyword)`,
	}
	if !slices.Equal(problems, want) {
		t.Errorf("problems =\n%q\nwant\n%q", problems, want)
	}
}

func TestHasErrorType(t *testing.T) {
	esErr := types.ElasticsearchError{
		Status:     400,
		ErrorCause: types.ErrorCause{Type: "resource_already_exists_exception"},
	}
	wrapped := fmt.Errorf("create index: %w", esErr)

	tests := []struct {
		name      string
		err       error
		errorType string
		want      bool
	}{
		{name: "wrapped matching type", err: wrapped, errorType: "resource_already_exists_exception", want: true},
		{name: "pointer matching type", err: &esErr, errorType: "resource_already_exists_exception", want: true},
		{name: "different type", err: wrapped, errorType: "version_conflict_engine_exception", want: false},
		{name: "not an elasticsearch error", err: errors.New("connection refused"), errorType: "resource_already_exists_exception", want: false},
		{name: "nil error", err: nil, errorType: "resource_already_exists_exception", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasErrorType(tt.err, tt.errorType); got != tt.want {
				t.Errorf("hasErrorType() = %t, want %t", got, tt.want)
			}
		})
	}
}

func decodeMapping(t *testing.T, raw string) *types.TypeMapping {
	t.Helper()
	var m types.TypeMapping
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("decode mapping: %v", err)
	}
	return &m
}
