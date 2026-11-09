// Package search provides an embedded Bleve full-text index over captured
// messages. The index is a rebuildable projection of the authoritative store.
package search

import (
	"context"
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/gnrfan/mailvoid/internal/models"
	"github.com/gnrfan/mailvoid/internal/storage"
)

// Query describes a full-text and metadata search.
type Query struct {
	Text      string
	Mailbox   string
	Domain    string
	Recipient string
	Sender    string
	Action    string
	Port      int
	Since     time.Time
	Until     time.Time
	Limit     int
	Offset    int
	Sort      string // "relevance" (default) or "received_at"
}

// Hit is one matched message.
type Hit struct {
	ID         string
	Score      float64
	Highlights map[string][]string
}

// Results is a page of search hits.
type Results struct {
	Hits  []Hit
	Total int
}

// Index is the search projection used by the web and MCP layers.
type Index interface {
	IndexMessage(ctx context.Context, message *models.Message) error
	DeleteMessage(ctx context.Context, id string) error
	Search(ctx context.Context, query Query) (Results, error)
	Close() error
}

const rebuildPageSize = 500

var tagRegexp = regexp.MustCompile(`(?s)<[^>]*>`)

// Bleve is a disk-backed Bleve index.
type Bleve struct {
	mu   sync.RWMutex
	idx  bleve.Index
	path string
}

// Open opens or creates a Bleve index at path. The bool reports whether a new
// empty index was created.
func Open(path string) (*Bleve, bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false, err
	}
	created := false
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		created = true
		idx, err := bleve.New(path, indexMapping())
		if err != nil {
			return nil, false, err
		}
		return &Bleve{idx: idx, path: path}, true, nil
	}
	idx, err := bleve.Open(path)
	if err != nil {
		return nil, false, err
	}
	return &Bleve{idx: idx, path: path}, created, nil
}

// Ensure rebuilds the index if its document count differs from the store.
func (b *Bleve) Ensure(ctx context.Context, store storage.Store) error {
	b.mu.RLock()
	count, err := b.idx.DocCount()
	b.mu.RUnlock()
	if err != nil {
		return err
	}
	total, err := store.CountMessages(ctx, models.MessageFilter{})
	if err != nil {
		return err
	}
	if uint64(total) == count {
		return nil
	}
	return b.Rebuild(ctx, store)
}

// Rebuild recreates the index from the authoritative message store.
func (b *Bleve) Rebuild(ctx context.Context, store storage.Store) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.idx != nil {
		_ = b.idx.Close()
		b.idx = nil
	}
	if err := os.RemoveAll(b.path); err != nil {
		return err
	}
	idx, err := bleve.New(b.path, indexMapping())
	if err != nil {
		return err
	}
	b.idx = idx

	for offset := 0; ; offset += rebuildPageSize {
		messages, err := store.ListMessages(ctx, models.MessageFilter{Limit: rebuildPageSize, Offset: offset})
		if err != nil {
			return err
		}
		if len(messages) == 0 {
			break
		}
		batch := idx.NewBatch()
		for _, message := range messages {
			if err := batch.Index(message.ID, messageDocument(message)); err != nil {
				return err
			}
		}
		if err := idx.Batch(batch); err != nil {
			return err
		}
		if len(messages) < rebuildPageSize {
			break
		}
	}
	return nil
}

// IndexMessage indexes or removes a message.
func (b *Bleve) IndexMessage(_ context.Context, message *models.Message) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.idx == nil {
		return errors.New("search index is closed")
	}
	if message.Deleted {
		return b.idx.Delete(message.ID)
	}
	return b.idx.Index(message.ID, messageDocument(message))
}

// DeleteMessage removes a message from the index.
func (b *Bleve) DeleteMessage(_ context.Context, id string) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.idx == nil {
		return errors.New("search index is closed")
	}
	return b.idx.Delete(id)
}

// Search executes a full-text query with metadata filters.
func (b *Bleve) Search(ctx context.Context, request Query) (Results, error) {
	limit := request.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := request.Offset
	if offset < 0 {
		offset = 0
	}

	q := buildQuery(request)
	searchRequest := bleve.NewSearchRequestOptions(q, limit, offset, false)
	if request.Sort == "received_at" {
		searchRequest.SortBy([]string{"-received_at_unix"})
	} else {
		searchRequest.SortBy([]string{"-_score"})
	}
	highlight := bleve.NewHighlight()
	for _, field := range []string{"subject", "body", "sender", "recipient"} {
		highlight.AddField(field)
	}
	searchRequest.Highlight = highlight

	b.mu.RLock()
	if b.idx == nil {
		b.mu.RUnlock()
		return Results{}, errors.New("search index is closed")
	}
	result, err := b.idx.SearchInContext(ctx, searchRequest)
	b.mu.RUnlock()
	if err != nil {
		return Results{}, err
	}

	hits := make([]Hit, 0, len(result.Hits))
	for _, match := range result.Hits {
		highlights := make(map[string][]string, len(match.Fragments))
		for field, fragments := range match.Fragments {
			highlights[field] = fragments
		}
		hits = append(hits, Hit{ID: match.ID, Score: match.Score, Highlights: highlights})
	}
	return Results{Hits: hits, Total: int(result.Total)}, nil
}

// Close closes the Bleve index.
func (b *Bleve) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.idx == nil {
		return nil
	}
	err := b.idx.Close()
	b.idx = nil
	return err
}

func buildQuery(request Query) query.Query {
	var clauses []query.Query
	if strings.TrimSpace(request.Text) != "" {
		clauses = append(clauses, bleve.NewMatchQuery(request.Text))
	}
	addTerm := func(field, value string) {
		if value == "" {
			return
		}
		term := bleve.NewTermQuery(value)
		term.SetField(field)
		clauses = append(clauses, term)
	}
	addTerm("mailbox_kw", strings.ToLower(request.Mailbox))
	addTerm("domain_kw", strings.ToLower(request.Domain))
	addTerm("recipient_kw", strings.ToLower(request.Recipient))
	addTerm("sender_kw", strings.ToLower(request.Sender))
	addTerm("action_kw", request.Action)

	if request.Port > 0 {
		min := float64(request.Port)
		max := float64(request.Port + 1)
		rangeQuery := bleve.NewNumericRangeQuery(&min, &max)
		rangeQuery.SetField("local_port")
		clauses = append(clauses, rangeQuery)
	}
	if !request.Since.IsZero() {
		min := float64(request.Since.Unix())
		rangeQuery := bleve.NewNumericRangeQuery(&min, nil)
		rangeQuery.SetField("received_at_unix")
		clauses = append(clauses, rangeQuery)
	}
	if !request.Until.IsZero() {
		max := float64(request.Until.Unix() + 1)
		rangeQuery := bleve.NewNumericRangeQuery(nil, &max)
		rangeQuery.SetField("received_at_unix")
		clauses = append(clauses, rangeQuery)
	}

	switch len(clauses) {
	case 0:
		return bleve.NewMatchAllQuery()
	case 1:
		return clauses[0]
	default:
		return bleve.NewConjunctionQuery(clauses...)
	}
}

type messageDoc struct {
	Subject        string  `json:"subject"`
	Body           string  `json:"body"`
	Sender         string  `json:"sender"`
	Recipient      string  `json:"recipient"`
	Mailbox        string  `json:"mailbox"`
	Domain         string  `json:"domain"`
	RemoteIP       string  `json:"remote_ip"`
	Action         string  `json:"action"`
	Headers        string  `json:"headers"`
	SenderKW       string  `json:"sender_kw"`
	RecipientKW    string  `json:"recipient_kw"`
	MailboxKW      string  `json:"mailbox_kw"`
	DomainKW       string  `json:"domain_kw"`
	RemoteIPKW     string  `json:"remote_ip_kw"`
	ActionKW       string  `json:"action_kw"`
	LocalPort      float64 `json:"local_port"`
	Size           float64 `json:"size"`
	ReceivedAtUnix float64 `json:"received_at_unix"`
}

func messageDocument(message *models.Message) messageDoc {
	body := message.BodyText
	if message.BodyHTML != "" {
		body += "\n" + stripHTML(message.BodyHTML)
	}
	return messageDoc{
		Subject:        message.Subject,
		Body:           body,
		Sender:         message.Sender,
		Recipient:      message.Recipient,
		Mailbox:        message.Mailbox,
		Domain:         message.Domain,
		RemoteIP:       message.RemoteIP,
		Action:         message.Action,
		Headers:        message.RawHeaders,
		SenderKW:       strings.ToLower(message.Sender),
		RecipientKW:    strings.ToLower(message.Recipient),
		MailboxKW:      strings.ToLower(message.Mailbox),
		DomainKW:       strings.ToLower(message.Domain),
		RemoteIPKW:     message.RemoteIP,
		ActionKW:       message.Action,
		LocalPort:      float64(message.LocalPort),
		Size:           float64(message.Size),
		ReceivedAtUnix: float64(message.ReceivedAt.Unix()),
	}
}

func stripHTML(value string) string {
	return strings.TrimSpace(html.UnescapeString(tagRegexp.ReplaceAllString(value, " ")))
}

func indexMapping() mapping.IndexMapping {
	im := bleve.NewIndexMapping()
	im.DefaultAnalyzer = "standard"

	dm := bleve.NewDocumentMapping()
	dm.Dynamic = false

	text := bleve.NewTextFieldMapping()
	text.Store = true
	text.IncludeTermVectors = true
	for _, field := range []string{"subject", "body", "sender", "recipient", "mailbox", "domain", "remote_ip"} {
		fm := *text
		dm.AddFieldMappingsAt(field, &fm)
	}
	headers := bleve.NewTextFieldMapping()
	headers.Store = false
	headers.IncludeTermVectors = false
	dm.AddFieldMappingsAt("headers", headers)

	keyword := bleve.NewKeywordFieldMapping()
	for _, field := range []string{"sender_kw", "recipient_kw", "mailbox_kw", "domain_kw", "remote_ip_kw", "action_kw"} {
		fm := *keyword
		dm.AddFieldMappingsAt(field, &fm)
	}

	for _, field := range []string{"local_port", "size", "received_at_unix"} {
		fm := bleve.NewNumericFieldMapping()
		fm.DocValues = true
		dm.AddFieldMappingsAt(field, fm)
	}

	im.DefaultMapping = dm
	return im
}

// IndexedStore decorates a storage.Store and keeps the search index in sync.
// Index failures are reported through onError but never fail the mutation.
type IndexedStore struct {
	storage.Store
	index   Index
	onError func(error)
}

// NewIndexedStore wraps store with an index.
func NewIndexedStore(store storage.Store, index Index, onError func(error)) storage.Store {
	return &IndexedStore{Store: store, index: index, onError: onError}
}

func (s *IndexedStore) report(err error) {
	if err != nil && s.onError != nil {
		s.onError(err)
	}
}

// SaveMessage delegates and indexes the stored message.
func (s *IndexedStore) SaveMessage(ctx context.Context, message *models.Message) error {
	if err := s.Store.SaveMessage(ctx, message); err != nil {
		return err
	}
	s.report(s.index.IndexMessage(ctx, message))
	return nil
}

// DeleteMessage delegates and removes the message from the index.
func (s *IndexedStore) DeleteMessage(ctx context.Context, id string) error {
	if err := s.Store.DeleteMessage(ctx, id); err != nil {
		return err
	}
	s.report(s.index.DeleteMessage(ctx, id))
	return nil
}

// DeleteExpired delegates and removes expired messages from the index.
func (s *IndexedStore) DeleteExpired(ctx context.Context, now time.Time) ([]*models.Message, error) {
	expired, err := s.Store.DeleteExpired(ctx, now)
	if err != nil {
		return nil, err
	}
	for _, message := range expired {
		s.report(s.index.DeleteMessage(ctx, message.ID))
	}
	return expired, nil
}

// String implements fmt.Stringer for diagnostics.
func (b *Bleve) String() string {
	return fmt.Sprintf("bleve(%s)", b.path)
}
