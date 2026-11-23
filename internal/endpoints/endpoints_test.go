package endpoints

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct {
		in, want string
		params   []string
	}{
		{"HTTPS://Example.COM:443/a/../b/?z=1&a=2#frag", "https://example.com/b?a=2&z=1", []string{"a", "z"}},
		{"example.com/path", "http://example.com/path", nil},
		{"http://example.com:80", "http://example.com/", nil},
		{"http://example.com:8080/x//y", "http://example.com:8080/x/y", nil},
		{"https://example.com/a%2Fb/", "https://example.com/a%2Fb", nil},
		{"https://example.com/caf%C3%A9", "https://example.com/caf%C3%A9", nil},
		{"https://example.com/p?b=2&a=1&a=0&flag", "https://example.com/p?a=0&a=1&b=2&flag", []string{"a", "b", "flag"}},
		{"https://example.com./", "https://example.com/", nil},
	}
	for _, c := range cases {
		got, err := Normalize(c.in)
		if err != nil {
			t.Errorf("Normalize(%q): %v", c.in, err)
			continue
		}
		if got.Normalized != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got.Normalized, c.want)
		}
		if got.Original != c.in {
			t.Errorf("original not preserved: %q", got.Original)
		}
		if len(got.Params) != len(c.params) {
			t.Errorf("Normalize(%q) params = %v, want %v", c.in, got.Params, c.params)
		}
	}
}

func TestNormalizeRejects(t *testing.T) {
	for _, in := range []string{"", "   ", "ftp://example.com/x", "http://exa mple.com", "http://", "javascript:alert(1)"} {
		if _, err := Normalize(in); err == nil {
			t.Errorf("Normalize(%q) succeeded, want error", in)
		}
	}
}

func TestClassify(t *testing.T) {
	cases := map[string]string{
		"/swagger.json":         "openapi-docs",
		"/v3/api-docs":          "openapi-docs",
		"/graphql":              "graphql",
		"/static/app.min.js":    "javascript",
		"/sitemap.xml":          "sitemap",
		"/robots.txt":           "public-config",
		"/.well-known/security": "public-config",
		"/img/logo.png":         "static",
		"/api/users":            "rest-api",
		"/v2/orders":            "rest-api",
		"/docs/intro":           "documentation",
		"/about":                "route",
	}
	for p, want := range cases {
		if got := Classify(p); got.Class != want || got.Rule == "" {
			t.Errorf("Classify(%q) = %+v, want class %q", p, got, want)
		}
	}
}
