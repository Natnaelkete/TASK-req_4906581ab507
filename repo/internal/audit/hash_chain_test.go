package audit_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eaglepoint/harborclass/internal/audit"
	"github.com/eaglepoint/harborclass/internal/models"
	"github.com/eaglepoint/harborclass/internal/store"
)

func newChain() (*audit.Chain, *store.Memory) {
	s := store.NewMemory()
	c := audit.New(s)
	c.Clock = func() time.Time { return time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC) }
	return c, s
}

func TestAppendChainsByPrevHash(t *testing.T) {
	c, _ := newChain()
	ctx := context.Background()
	e1, err := c.Append(ctx, "org-main", "admin", "login", "auth", "")
	if err != nil {
		t.Fatal(err)
	}
	e2, err := c.Append(ctx, "org-main", "admin", "booking.create", "o1", "HC-03282026-000742")
	if err != nil {
		t.Fatal(err)
	}
	if e1.PrevHash != "" {
		t.Fatalf("first entry should have empty prev hash, got %q", e1.PrevHash)
	}
	if e2.PrevHash != e1.Hash {
		t.Fatalf("prev hash mismatch: %q vs %q", e2.PrevHash, e1.Hash)
	}
	if e1.Hash == e2.Hash {
		t.Fatal("hashes must differ across entries")
	}
}

func TestVerifyDetectsTampering(t *testing.T) {
	c, _ := newChain()
	ctx := context.Background()
	_, _ = c.Append(ctx, "org-main", "admin", "login", "auth", "")
	_, _ = c.Append(ctx, "org-main", "admin", "booking.create", "o1", "")
	_, _ = c.Append(ctx, "org-main", "admin", "booking.confirm", "o1", "")
	rows, _ := c.Search(ctx, store.AuditFilter{})
	if bad := audit.Verify(rows); bad != -1 {
		t.Fatalf("expected intact chain, tamper at %d", bad)
	}
	// Tamper the middle entry.
	rows[1].Detail = "tampered"
	if bad := audit.Verify(rows); bad == -1 {
		t.Fatal("expected tampering to be detected")
	}
}

func TestHashDeterministic(t *testing.T) {
	e := models.AuditEntry{
		ID: "evt-1", At: time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
		Actor: "admin", Action: "login", Resource: "auth", Detail: "",
	}
	if audit.Hash(e) != audit.Hash(e) {
		t.Fatal("hash must be deterministic")
	}
}

func TestSearchFilters(t *testing.T) {
	c, _ := newChain()
	ctx := context.Background()
	_, _ = c.Append(ctx, "org-main", "admin", "login", "auth", "")
	_, _ = c.Append(ctx, "org-main", "student", "booking.create", "o1", "")
	rows, _ := c.Search(ctx, store.AuditFilter{Actor: "student"})
	if len(rows) != 1 || rows[0].Actor != "student" {
		t.Fatalf("search by actor failed: %+v", rows)
	}
}

func TestExportCSV(t *testing.T) {
	c, _ := newChain()
	ctx := context.Background()
	_, _ = c.Append(ctx, "org-main", "admin", "login", "auth", "")
	_, _ = c.Append(ctx, "org-main", "admin", "audit.export", "audit_log", "detail with, comma")
	var buf bytes.Buffer
	if err := c.ExportTo(ctx, store.AuditFilter{}, &buf); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if !strings.Contains(s, "id,at,actor,action,resource,detail,prev_hash,hash") {
		t.Fatalf("missing CSV header: %s", s)
	}
	if !strings.Contains(s, "\"detail with, comma\"") {
		t.Fatalf("CSV comma escaping missing: %s", s)
	}
}
