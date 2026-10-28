package elastic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/fajri-rabka/go-elasticsearch-poc/internal/model"
)

func TestBuildSearchQuery(t *testing.T) {
	t.Run("empty filter produces match_all", func(t *testing.T) {
		q := buildSearchQuery(model.SearchFilter{})
		raw, err := json.Marshal(q.QueryCaster())
		if err != nil {
			t.Fatalf("marshal query: %v", err)
		}
		if !strings.Contains(string(raw), "match_all") {
			t.Errorf("expected match_all query, got: %s", string(raw))
		}
	})

	t.Run("search query produces multi_match with boosted model_name", func(t *testing.T) {
		q := buildSearchQuery(model.SearchFilter{Search: "BeAT"})
		raw, err := json.Marshal(q.QueryCaster())
		if err != nil {
			t.Fatalf("marshal query: %v", err)
		}
		str := string(raw)
		if !strings.Contains(str, "multi_match") {
			t.Errorf("expected multi_match, got: %s", str)
		}
		if !strings.Contains(str, "model_name^2") {
			t.Errorf("expected model_name^2 boost, got: %s", str)
		}
		if !strings.Contains(str, "description") {
			t.Errorf("expected description field, got: %s", str)
		}
	})

	t.Run("filter terms produce bool filter", func(t *testing.T) {
		avail := true
		minP := int64(15000000)
		maxP := int64(25000000)

		q := buildSearchQuery(model.SearchFilter{
			Category:     "matic",
			Transmission: "automatic",
			Available:    &avail,
			MinPrice:     &minP,
			MaxPrice:     &maxP,
		})
		raw, err := json.Marshal(q.QueryCaster())
		if err != nil {
			t.Fatalf("marshal query: %v", err)
		}
		str := string(raw)
		if !strings.Contains(str, "bool") || !strings.Contains(str, "filter") {
			t.Errorf("expected bool query with filter clauses, got: %s", str)
		}
		if !strings.Contains(str, "matic") {
			t.Errorf("expected category matic in query, got: %s", str)
		}
		if !strings.Contains(str, "automatic") {
			t.Errorf("expected transmission automatic in query, got: %s", str)
		}
		if !strings.Contains(str, "price_idr") {
			t.Errorf("expected price_idr range, got: %s", str)
		}
	})

	t.Run("combined search and filter produces must and filter", func(t *testing.T) {
		q := buildSearchQuery(model.SearchFilter{
			Search:   "Scoopy",
			Category: "matic",
		})
		raw, err := json.Marshal(q.QueryCaster())
		if err != nil {
			t.Fatalf("marshal query: %v", err)
		}
		str := string(raw)
		if !strings.Contains(str, "must") || !strings.Contains(str, "multi_match") {
			t.Errorf("expected must clause with multi_match, got: %s", str)
		}
		if !strings.Contains(str, "filter") || !strings.Contains(str, "matic") {
			t.Errorf("expected filter clause with category, got: %s", str)
		}
	})
}

func TestBuildSortOption(t *testing.T) {
	tests := []struct {
		sort     string
		wantNil  bool
		wantJSON string
	}{
		{sort: "relevance", wantNil: true},
		{sort: "unknown", wantNil: true},
		{sort: "price_asc", wantNil: false, wantJSON: "price_idr"},
		{sort: "price_desc", wantNil: false, wantJSON: "price_idr"},
		{sort: "name_asc", wantNil: false, wantJSON: "model_name.keyword"},
		{sort: "name_desc", wantNil: false, wantJSON: "model_name.keyword"},
	}

	for _, tt := range tests {
		t.Run(tt.sort, func(t *testing.T) {
			opt := buildSortOption(tt.sort)
			if tt.wantNil {
				if opt != nil {
					t.Fatalf("expected nil sort option for %q", tt.sort)
				}
				return
			}
			if opt == nil {
				t.Fatalf("expected non-nil sort option for %q", tt.sort)
			}
			raw, err := json.Marshal(opt.SortCombinationsCaster())
			if err != nil {
				t.Fatalf("marshal sort: %v", err)
			}
			if !strings.Contains(string(raw), tt.wantJSON) {
				t.Errorf("sort %q JSON = %s, want containing %q", tt.sort, string(raw), tt.wantJSON)
			}
		})
	}
}
