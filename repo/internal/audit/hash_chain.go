// Package audit implements a tamper-evident audit log using a
// hash-chain. Each entry's hash is computed over its payload and the
// previous entry's hash, so any later tampering invalidates the tail.
package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/eaglepoint/harborclass/internal/models"
	"github.com/eaglepoint/harborclass/internal/store"
)

// Chain appends tamper-evident entries and exposes search/export helpers.
type Chain struct {
	Store store.Store
	Clock func() time.Time
}

// New constructs a chain bound to a store.
func New(s store.Store) *Chain {
	return &Chain{Store: s, Clock: time.Now}
}

// Append adds a new entry; the hash is derived from the previous hash
// and the payload fields so the chain is verifiable.
func (c *Chain) Append(ctx context.Context, actor, action, resource, detail string) (models.AuditEntry, error) {
	prev := ""
	if last, err := c.Store.LatestAudit(ctx); err == nil {
		prev = last.Hash
	} else if !errors.Is(err, store.ErrNotFound) {
		return models.AuditEntry{}, err
	}
	e := models.AuditEntry{
		ID:       fmt.Sprintf("evt-%d", c.Clock().UnixNano()),
		At:       c.Clock(),
		Actor:    actor,
		Action:   action,
		Resource: resource,
		Detail:   detail,
		PrevHash: prev,
	}
	e.Hash = Hash(e)
	return c.Store.AppendAudit(ctx, e)
}

// Hash returns the canonical hash over an audit entry's fields.
func Hash(e models.AuditEntry) string {
	h := sha256.New()
	io.WriteString(h, e.ID)
	io.WriteString(h, "|")
	io.WriteString(h, e.At.UTC().Format(time.RFC3339Nano))
	io.WriteString(h, "|")
	io.WriteString(h, e.Actor)
	io.WriteString(h, "|")
	io.WriteString(h, e.Action)
	io.WriteString(h, "|")
	io.WriteString(h, e.Resource)
	io.WriteString(h, "|")
	io.WriteString(h, e.Detail)
	io.WriteString(h, "|")
	io.WriteString(h, e.PrevHash)
	return hex.EncodeToString(h.Sum(nil))
}

// Verify recomputes the chain against a slice of entries and returns
// the index of the first tampered entry or -1 if the chain is intact.
func Verify(entries []models.AuditEntry) int {
	prev := ""
	for i, e := range entries {
		if e.PrevHash != prev {
			return i
		}
		expected := e
		if Hash(expected) != e.Hash {
			return i
		}
		prev = e.Hash
	}
	return -1
}

// Search is a thin wrapper over store.SearchAudit.
func (c *Chain) Search(ctx context.Context, f store.AuditFilter) ([]models.AuditEntry, error) {
	return c.Store.SearchAudit(ctx, f)
}

// ExportTo writes the filtered rows as newline-delimited CSV to w. This
// is the "export to a local file" capability the admin console uses.
func (c *Chain) ExportTo(ctx context.Context, f store.AuditFilter, w io.Writer) error {
	rows, err := c.Search(ctx, f)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "id,at,actor,action,resource,detail,prev_hash,hash"); err != nil {
		return err
	}
	for _, e := range rows {
		line := strings.Join([]string{
			e.ID,
			e.At.UTC().Format(time.RFC3339Nano),
			e.Actor, e.Action, e.Resource,
			csvEscape(e.Detail),
			e.PrevHash, e.Hash,
		}, ",")
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

func csvEscape(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		return "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\""
	}
	return s
}
