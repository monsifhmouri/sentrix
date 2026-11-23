package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"sentrix/pkg/models"
)

func sample(name string) *models.Project {
	return &models.Project{
		Name: name, Domain: "example.com", UpdatedAt: time.Now().UTC(),
		Records: []models.Record{
			{ID: "1", Kind: models.KindEndpoint, NormalizedURL: "https://example.com/api/<x>&y", Class: "rest-api", Scope: models.InScope,
				Observations: []models.Observation{{Source: "wayback", Evidence: "cap"}}},
			{ID: "2", Kind: models.KindHost, NormalizedURL: "evil.test", Class: "hostname", Scope: models.OutOfScope,
				Observations: []models.Observation{{Source: "otx", Evidence: "dns"}}},
		},
	}
}

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRoundTripAndList(t *testing.T) {
	s := open(t)
	if err := s.Save(sample("My Project")); err != nil {
		t.Fatal(err)
	}
	p, err := s.Load("my project")
	if err != nil || len(p.Records) != 2 || p.Records[0].NormalizedURL != "https://example.com/api/<x>&y" {
		t.Fatalf("round trip failed: %+v, %v", p, err)
	}
	list, err := s.List()
	if err != nil || len(list) != 1 || list[0].Records != 2 {
		t.Fatalf("list: %+v, %v", list, err)
	}
	if _, err := s.Load("missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestIntegrityCheckDetectsTampering(t *testing.T) {
	s := open(t)
	if err := s.Save(sample("p")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.dir, "p"+fileSuffix)
	data, _ := os.ReadFile(path)
	if err := os.WriteFile(path, []byte(strings.Replace(string(data), "rest-api", "graphql", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Check("p"); err == nil || !strings.Contains(err.Error(), "integrity") {
		t.Fatalf("tampering not detected: %v", err)
	}
	if err := s.Backup("p", filepath.Join(t.TempDir(), "b.json")); err == nil {
		t.Error("backup of a damaged project must fail")
	}
	list, _ := s.List()
	if len(list) != 1 || !strings.Contains(list[0].Name, "unreadable") {
		t.Errorf("damaged project should be listed as unreadable: %+v", list)
	}
}

func TestBackupRestore(t *testing.T) {
	s := open(t)
	if err := s.Save(sample("p")); err != nil {
		t.Fatal(err)
	}
	bak := filepath.Join(t.TempDir(), "p.bak")
	if err := s.Backup("p", bak); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Restore(bak, false); err == nil {
		t.Error("restore must not overwrite silently")
	}
	other := open(t)
	name, err := other.Restore(bak, false)
	if err != nil || name != "p" {
		t.Fatalf("restore: %q, %v", name, err)
	}
	if _, err := other.Load("p"); err != nil {
		t.Error(err)
	}
}

func TestConcurrentSavesNeverCorrupt(t *testing.T) {
	s := open(t)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p := sample("p")
			p.Program = fmt.Sprint(i)
			if err := s.Save(p); err != nil {
				t.Error(err)
			}
			if _, err := s.Load("p"); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if err := s.Check("p"); err != nil {
		t.Fatal(err)
	}
}

func TestSlugRejectsGarbage(t *testing.T) {
	for _, n := range []string{"", "   ", "!!!", strings.Repeat("a", 65)} {
		if _, err := Slug(n); err == nil {
			t.Errorf("Slug(%q) should fail", n)
		}
	}
	if s, _ := Slug("../Evil Name"); strings.ContainsAny(s, "./ ") {
		t.Errorf("unsafe slug %q", s)
	}
}

func TestFilter(t *testing.T) {
	recs := sample("p").Records
	cases := []struct {
		q    Query
		want int
	}{
		{Query{}, 2}, {Query{Kind: models.KindHost}, 1}, {Query{Scope: models.InScope}, 1},
		{Query{Class: "rest-api"}, 1}, {Query{Source: "otx"}, 1}, {Query{Text: "DNS"}, 1}, {Query{Text: "nothing"}, 0},
	}
	for _, c := range cases {
		if got := len(Filter(recs, c.q)); got != c.want {
			t.Errorf("%+v: got %d want %d", c.q, got, c.want)
		}
	}
}
