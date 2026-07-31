// Package typed provides additive generic wrappers around the stable TableTheory
// core interfaces. The wrappers keep the existing runtime behavior while moving
// common result/destination shapes into Go's type system at call sites.
package typed

import (
	"errors"
	"fmt"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/query"
)

// Model is a generic handle for one TableTheory model type.
type Model[T any] struct {
	db core.DB
}

// Query is a generic query builder for a TableTheory model type.
type Query[T any] struct {
	query core.Query
}

// Key is a primary-key value tied to a specific model type.
type Key[T any] struct {
	pair core.KeyPair
}

// ModelOf creates a generic handle for model type T from an existing TableTheory DB.
func ModelOf[T any](db core.DB) Model[T] {
	return Model[T]{db: db}
}

// NewKey creates a model-typed key without requiring a Model handle.
func NewKey[T any](partitionKey any, sortKey ...any) Key[T] {
	return Key[T]{pair: core.NewKeyPair(partitionKey, sortKey...)}
}

// Core returns the underlying untyped TableTheory key pair for interoperability.
func (k Key[T]) Core() core.KeyPair {
	return k.pair
}

// Key creates a model-typed key.
func (m Model[T]) Key(partitionKey any, sortKey ...any) Key[T] {
	return NewKey[T](partitionKey, sortKey...)
}

// Query returns a generic query builder using a zero-value model instance for metadata.
func (m Model[T]) Query() Query[T] {
	var zero T
	return Query[T]{query: m.db.Model(&zero)}
}

// Item returns a generic query builder bound to item for create/update/delete operations.
func (m Model[T]) Item(item *T) Query[T] {
	return Query[T]{query: m.db.Model(item)}
}

// Create inserts item using the existing TableTheory create semantics.
func (m Model[T]) Create(item *T) error {
	return m.Item(item).Create()
}

// CreateOrUpdate upserts item using the existing TableTheory create-or-update semantics.
func (m Model[T]) CreateOrUpdate(item *T) error {
	return m.Item(item).CreateOrUpdate()
}

// Update updates item fields using the existing TableTheory update semantics.
func (m Model[T]) Update(item *T, fields ...string) error {
	return m.Item(item).Update(fields...)
}

// Delete deletes item using the existing TableTheory delete semantics.
func (m Model[T]) Delete(item *T) error {
	return m.Item(item).Delete()
}

// Get fetches one item by typed key. Missing items return ErrItemNotFound.
func (m Model[T]) Get(key Key[T]) (T, error) {
	items, err := m.BatchGet([]Key[T]{key})
	if err != nil {
		var zero T
		return zero, err
	}
	if len(items) == 0 {
		var zero T
		return zero, theorydbErrors.ErrItemNotFound
	}
	return items[0], nil
}

// GetOrNil fetches one item by typed key and returns nil when it is absent.
func (m Model[T]) GetOrNil(key Key[T]) (*T, error) {
	item, err := m.Get(key)
	if err != nil {
		if errors.Is(err, theorydbErrors.ErrItemNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

// BatchGet retrieves multiple items by typed keys.
func (m Model[T]) BatchGet(keys []Key[T]) ([]T, error) {
	var zero T
	untypedKeys := make([]any, 0, len(keys))
	for _, key := range keys {
		untypedKeys = append(untypedKeys, key.Core())
	}
	var dest []T
	if err := m.db.Model(&zero).BatchGet(untypedKeys, &dest); err != nil {
		return nil, err
	}
	return dest, nil
}

// DeleteKey deletes one item by typed key.
func (m Model[T]) DeleteKey(key Key[T]) error {
	var zero T
	return m.db.Model(&zero).BatchDelete([]any{key.Core()})
}

// Where adds a condition and preserves the generic query type.
func (q Query[T]) Where(field string, op query.Operator, value any) Query[T] {
	q.query = q.query.Where(field, string(op), value)
	return q
}

// Between adds a BETWEEN condition with a two-argument value shape.
func (q Query[T]) Between(field string, lo any, hi any) Query[T] {
	return q.Where(field, query.OpBetween, query.Between(lo, hi))
}

// Filter adds a filter condition and preserves the generic query type.
func (q Query[T]) Filter(field string, op query.Operator, value any) Query[T] {
	q.query = q.query.Filter(field, string(op), value)
	return q
}

// Index selects an index and preserves the generic query type.
func (q Query[T]) Index(indexName string) Query[T] {
	q.query = q.query.Index(indexName)
	return q
}

// Limit sets a result limit and preserves the generic query type.
func (q Query[T]) Limit(limit int) Query[T] {
	q.query = q.query.Limit(limit)
	return q
}

// First returns the first matching item as T.
func (q Query[T]) First() (T, error) {
	var dest T
	if err := q.query.First(&dest); err != nil {
		return dest, err
	}
	return dest, nil
}

// FirstOrNil returns the first matching item or nil when it is absent.
func (q Query[T]) FirstOrNil() (*T, error) {
	var dest T
	found, err := query.FirstOrNil(q.query, &dest)
	if err != nil || !found {
		return nil, err
	}
	return &dest, nil
}

// All returns all matching items as []T.
func (q Query[T]) All() ([]T, error) {
	var dest []T
	if err := q.query.All(&dest); err != nil {
		return nil, err
	}
	return dest, nil
}

// Count returns the number of matching items without materializing them.
func (q Query[T]) Count() (int64, error) {
	return q.query.Count()
}

// Create inserts the query's bound item.
func (q Query[T]) Create() error {
	if q.query == nil {
		return fmt.Errorf("typed query is not bound to a model")
	}
	return q.query.Create()
}

// CreateOrUpdate upserts the query's bound item.
func (q Query[T]) CreateOrUpdate() error {
	if q.query == nil {
		return fmt.Errorf("typed query is not bound to a model")
	}
	return q.query.CreateOrUpdate()
}

// Update updates selected fields on the query's bound item.
func (q Query[T]) Update(fields ...string) error {
	if q.query == nil {
		return fmt.Errorf("typed query is not bound to a model")
	}
	return q.query.Update(fields...)
}

// Delete deletes the query's bound item.
func (q Query[T]) Delete() error {
	if q.query == nil {
		return fmt.Errorf("typed query is not bound to a model")
	}
	return q.query.Delete()
}

// Untyped exposes the underlying core.Query for compatibility with APIs not yet
// wrapped by the generic layer.
func (q Query[T]) Untyped() core.Query {
	return q.query
}
