// Package elastic contains the Elasticsearch-backed data access code.
package elastic

import (
	"fmt"

	"github.com/elastic/go-elasticsearch/v9"
)

// NewClient creates a typed Elasticsearch client for the given address.
// It does not contact the cluster; callers verify connectivity with a request.
func NewClient(address string) (*elasticsearch.TypedClient, error) {
	client, err := elasticsearch.NewTyped(
		elasticsearch.WithAddresses(address),
	)
	if err != nil {
		return nil, fmt.Errorf("new typed client for %s: %w", address, err)
	}

	return client, nil
}
