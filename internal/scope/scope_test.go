package scope

import (
	"testing"

	"sentrix/pkg/models"
)

func mustGuard(t *testing.T, domain string, entries ...string) *Guard {
	t.Helper()
	rules, err := ParseRules(domain, entries)
	if err != nil {
		t.Fatalf("ParseRules: %v", err)
	}
	g, err := New(rules)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g
}

func TestCheckHost(t *testing.T) {
	g := mustGuard(t, "Example.com.", "host:partner.test", "exclude:dev.example.com")
	cases := []struct {
		host string
		want bool
	}{
		{"example.com", true},
		{"API.Example.com", true},
		{"a.b.example.com", true},
		{"notexample.com", false},
		{"example.com.evil.net", false},
		{"partner.test", true},
		{"sub.partner.test", false},
		{"dev.example.com", false},
		{"x.dev.example.com", false},
		{"bad host", false},
	}
	for _, c := range cases {
		if got := g.CheckHost(c.host).InScope; got != c.want {
			t.Errorf("CheckHost(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestCheckURLPrefixesAndExclusions(t *testing.T) {
	g := mustGuard(t, "example.com",
		"url:https://thirdparty.test/program/acme",
		"exclude:https://example.com/admin")
	cases := []struct {
		url  string
		want bool
	}{
		{"https://example.com/", true},
		{"https://example.com/admin", false},
		{"https://example.com/admin/users", false},
		{"https://example.com/administrator", true},
		{"https://thirdparty.test/program/acme", true},
		{"https://thirdparty.test/program/acme/x", true},
		{"https://thirdparty.test/program/acmeevil", false},
		{"https://thirdparty.test/other", false},
	}
	for _, c := range cases {
		if got := g.CheckURL(c.url).InScope; got != c.want {
			t.Errorf("CheckURL(%q) = %v (%s), want %v", c.url, got, g.CheckURL(c.url).Reason, c.want)
		}
	}
}

func TestWildcardRootNormalization(t *testing.T) {
	g := mustGuard(t, "example.com", "root:*.corp.example.org")
	if !g.CheckHost("a.corp.example.org").InScope || !g.CheckHost("corp.example.org").InScope {
		t.Fatal("wildcard root should cover the root and its subdomains")
	}
}

func TestInvalidRules(t *testing.T) {
	bad := []struct {
		domain  string
		entries []string
	}{
		{"localhost", nil},
		{"1.2.3.4", nil},
		{"exa mple.com", nil},
		{"example.com", []string{"url:ftp://x.test/"}},
		{"example.com", []string{"exclude:-bad-.com"}},
	}
	for _, c := range bad {
		if _, err := ParseRules(c.domain, c.entries); err == nil {
			t.Errorf("ParseRules(%q, %v) succeeded, want error", c.domain, c.entries)
		}
	}
	if _, err := New(models.ScopeRules{}); err == nil {
		t.Error("empty scope must be rejected")
	}
}
