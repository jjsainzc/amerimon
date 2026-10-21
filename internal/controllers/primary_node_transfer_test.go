package controllers

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestNodeAccessExportImportRoundTripMergesAndPreservesOtherNodes(t *testing.T) {
	p, db, _ := primaryNodeFixture(t)
	c, rec := nodeTestContext(http.MethodGet, nil)
	if err := p.exportAccess(c); err != nil {
		t.Fatal(err)
	}
	var doc nodeAccessDocument
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Format != "amerimon-node-access" || doc.Version != 1 || doc.Node != "616265" || len(doc.Entries) != 2 {
		t.Fatal("invalid export shape")
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), ".json") {
		t.Fatal("missing download filename")
	}
	doc.Node = "616266"
	doc.Entries[0].Reason = "Updated reason"
	doc.Entries = append(doc.Entries, nodeAccessEntry{"allowlist", "ea4abc/p", "Imported"})
	db["allowlist/616265/448512"] = "Keep existing"
	c, rec = nodeTestContext(http.MethodPost, map[string]any{"primary_node": "616265", "document": doc})
	if err := p.importAccess(c); err != nil {
		t.Fatal(err)
	}
	if db["allowlist/616265/448511"] != "Updated reason" || db["allowlist/616265/EA4ABC/P"] != "Imported" ||
		db["allowlist/616265/448512"] != "Keep existing" || db["denylist/700001/9999"] != "Otro servidor" {
		t.Fatal("merge modified unrelated entries or did not update matching entries")
	}
	if rec.Code != http.StatusOK {
		t.Fatal("import failed")
	}
}

func TestNodeAccessImportValidatesEntireDocumentBeforeWriting(t *testing.T) {
	for _, entries := range [][]nodeAccessEntry{
		{{"allowlist", "448512", "Valid"}, {"denylist", "616265", "Self"}},
		{{"allowlist", "test1", "First"}, {"allowlist", "TEST1", "Duplicate"}},
		{{"allowlist", "448512", "Valid"}, {"denylist", "TEST2", "bad\nreason"}},
	} {
		p, db, _ := primaryNodeFixture(t)
		before := nodeTestDB{}
		for k, v := range db {
			before[k] = v
		}
		c, _ := nodeTestContext(http.MethodPost, map[string]any{
			"primary_node": "616265", "document": nodeAccessDocument{"amerimon-node-access", 1, "616266", entries},
		})
		if err := p.importAccess(c); err == nil {
			t.Fatal("invalid document accepted")
		}
		if !reflect.DeepEqual(db, before) {
			t.Fatal("invalid import modified AstDB")
		}
	}
}

func TestNodeAccessImportRollbackRestoresUpdatedAndNewEntries(t *testing.T) {
	p, db, _ := primaryNodeFixture(t)
	before := nodeTestDB{}
	for k, v := range db {
		before[k] = v
	}
	failed := false
	p.ami = func(command string) (string, error) {
		if !failed && strings.HasPrefix(command, "database put denylist/616265 TEST3 ") {
			failed = true
			return "", errors.New("test AMI failure")
		}
		return db.command(command)
	}
	doc := nodeAccessDocument{"amerimon-node-access", 1, "616265", []nodeAccessEntry{
		{"allowlist", "448511", "Changed"}, {"allowlist", "448512", "New"}, {"denylist", "TEST3", "Fail"},
	}}
	c, _ := nodeTestContext(http.MethodPost, map[string]any{"primary_node": "616265", "document": doc})
	if err := p.importAccess(c); err == nil {
		t.Fatal("failure treated as success")
	}
	if !reflect.DeepEqual(db, before) {
		t.Fatalf("rollback failed: %+v", db)
	}
}

func TestNodeAccessImportRejectsUnknownVersionAndStaleTarget(t *testing.T) {
	for _, tc := range []struct {
		version int
		target  string
	}{{2, "616265"}, {1, "616266"}} {
		p, _, _ := primaryNodeFixture(t)
		p.ami = func(string) (string, error) { t.Fatal("invalid request reached AMI"); return "", nil }
		c, _ := nodeTestContext(http.MethodPost, map[string]any{
			"primary_node": tc.target, "document": nodeAccessDocument{"amerimon-node-access", tc.version, "616265", []nodeAccessEntry{}},
		})
		if err := p.importAccess(c); err == nil {
			t.Fatal("invalid import accepted")
		}
	}
}
