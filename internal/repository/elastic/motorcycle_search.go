package elastic

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/sortorder"

	"github.com/fajri-rabka/go-elasticsearch-poc/internal/model"
)

// Search executes a search query against Elasticsearch with full-text search,
// exact filtering, price range filter, sorting, and pagination.
func (r *MotorcycleRepository) Search(ctx context.Context, filter model.SearchFilter) (model.SearchResult, error) {
	from := (filter.Page - 1) * filter.Limit
	size := filter.Limit

	query := buildSearchQuery(filter)

	searchReq := r.client.Search().
		Index(r.index).
		Query(query).
		From(from).
		Size(size).
		TrackTotalHits(esdsl.NewTrackHits().Bool(true))

	if sortOption := buildSortOption(filter.Sort); sortOption != nil {
		searchReq.Sort(sortOption)
	}

	res, err := searchReq.Do(ctx)
	if err != nil {
		return model.SearchResult{}, fmt.Errorf("search motorcycles in index %q: %w", r.index, err)
	}

	total := int64(0)
	if res.Hits.Total != nil {
		total = res.Hits.Total.Value
	}

	items := make([]model.MotorcycleWithID, 0, len(res.Hits.Hits))
	for _, hit := range res.Hits.Hits {
		var m model.Motorcycle
		if err := json.Unmarshal(hit.Source_, &m); err != nil {
			return model.SearchResult{}, fmt.Errorf("decode search hit source: %w", err)
		}

		id := ""
		if hit.Id_ != nil {
			id = *hit.Id_
		}

		items = append(items, model.MotorcycleWithID{
			ID:         id,
			Motorcycle: m,
		})
	}

	totalPages := 0
	if total > 0 && filter.Limit > 0 {
		totalPages = int((total + int64(filter.Limit) - 1) / int64(filter.Limit))
	}

	return model.SearchResult{
		Items:      items,
		Total:      total,
		Page:       filter.Page,
		Limit:      filter.Limit,
		TotalPages: totalPages,
	}, nil
}

// buildSearchQuery constructs the QueryVariant combining full-text search and filters.
func buildSearchQuery(filter model.SearchFilter) types.QueryVariant {
	var filterClauses []types.QueryVariant

	if filter.Category != "" {
		filterClauses = append(filterClauses,
			esdsl.NewTermQuery("category", esdsl.NewFieldValue().String(filter.Category)),
		)
	}

	if filter.Transmission != "" {
		filterClauses = append(filterClauses,
			esdsl.NewTermQuery("transmission", esdsl.NewFieldValue().String(filter.Transmission)),
		)
	}

	if filter.Available != nil {
		filterClauses = append(filterClauses,
			esdsl.NewTermQuery("available", esdsl.NewFieldValue().Bool(*filter.Available)),
		)
	}

	if filter.MinPrice != nil || filter.MaxPrice != nil {
		rangeQ := esdsl.NewNumberRangeQuery("price_idr")
		if filter.MinPrice != nil {
			rangeQ.Gte(types.Float64(float64(*filter.MinPrice)))
		}
		if filter.MaxPrice != nil {
			rangeQ.Lte(types.Float64(float64(*filter.MaxPrice)))
		}
		filterClauses = append(filterClauses, rangeQ)
	}

	hasSearch := filter.Search != ""
	hasFilter := len(filterClauses) > 0

	// If neither full-text search nor any filter is requested, return match_all.
	if !hasSearch && !hasFilter {
		return esdsl.NewMatchAllQuery()
	}

	boolQ := esdsl.NewBoolQuery()

	if hasSearch {
		// Boost model_name so title matches rank higher than matching words in description.
		boolQ.Must(
			esdsl.NewMultiMatchQuery(filter.Search).
				Fields("model_name^2", "description"),
		)
	}

	if hasFilter {
		boolQ.Filter(filterClauses...)
	}

	return boolQ
}

// buildSortOption maps domain sort strings to Elasticsearch sort options.
// For "relevance", nil is returned so Elasticsearch sorts by _score desc automatically.
func buildSortOption(sort string) types.SortCombinationsVariant {
	switch sort {
	case "price_asc":
		return esdsl.NewSortOptions().
			AddSortOption("price_idr", esdsl.NewFieldSort(sortorder.Asc))
	case "price_desc":
		return esdsl.NewSortOptions().
			AddSortOption("price_idr", esdsl.NewFieldSort(sortorder.Desc))
	case "name_asc":
		// Sort on the keyword sub-field to sort the exact string rather than analyzed text.
		return esdsl.NewSortOptions().
			AddSortOption("model_name.keyword", esdsl.NewFieldSort(sortorder.Asc))
	case "name_desc":
		return esdsl.NewSortOptions().
			AddSortOption("model_name.keyword", esdsl.NewFieldSort(sortorder.Desc))
	default:
		return nil
	}
}

