package endpoints

import (
	"path"
	"regexp"
	"strings"
)

// Classification is a heuristic label derived from the URL alone. It says what
// a resource looks like, never what it is or whether it is vulnerable.
type Classification struct {
	Class      string
	Rule       string
	Confidence string // confidence in the label itself: "medium" or "low"
}

var (
	versionSeg = regexp.MustCompile(`^v[0-9]+$`)

	openAPIPaths = []string{"/swagger", "/swagger.json", "/swagger.yaml", "/swagger-ui", "/swagger-ui.html",
		"/openapi.json", "/openapi.yaml", "/openapi.yml", "/api-docs", "/v2/api-docs", "/v3/api-docs"}

	staticExt = map[string]bool{".css": true, ".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
		".svg": true, ".ico": true, ".woff": true, ".woff2": true, ".ttf": true, ".eot": true,
		".webp": true, ".mp4": true, ".mp3": true, ".pdf": true}

	configNames = map[string]bool{"robots.txt": true, "security.txt": true, "crossdomain.xml": true,
		"manifest.json": true, "humans.txt": true, "web.config": true, "package.json": true}
)

// Classify labels a normalized URL path. Rules are ordered from most to least
// specific; the first match wins.
func Classify(normalizedPath string) Classification {
	p := strings.ToLower(normalizedPath)
	base := path.Base(p)
	ext := path.Ext(base)

	for _, s := range openAPIPaths {
		if p == s || strings.HasPrefix(p, s+"/") || strings.HasSuffix(p, s) {
			return Classification{"openapi-docs", "path matches known OpenAPI/Swagger location " + s, "medium"}
		}
	}
	if strings.HasSuffix(p, "/graphql") || strings.HasSuffix(p, "/graphiql") || strings.Contains(p, "/graphql/") {
		return Classification{"graphql", "path contains a graphql segment", "medium"}
	}
	if ext == ".js" || ext == ".mjs" {
		return Classification{"javascript", "file extension " + ext, "medium"}
	}
	if strings.HasPrefix(base, "sitemap") && (ext == ".xml" || ext == ".gz" || ext == ".txt") {
		return Classification{"sitemap", "file name starts with sitemap", "medium"}
	}
	if configNames[base] || strings.HasPrefix(p, "/.well-known/") {
		return Classification{"public-config", "well-known public configuration file", "medium"}
	}
	if staticExt[ext] {
		return Classification{"static", "static file extension " + ext, "medium"}
	}
	segs := strings.Split(strings.Trim(p, "/"), "/")
	for _, s := range segs {
		if s == "api" || s == "rest" || versionSeg.MatchString(s) {
			return Classification{"rest-api", "path segment " + `"` + s + `"`, "low"}
		}
	}
	for _, s := range segs {
		if s == "docs" || s == "documentation" || s == "developers" || s == "developer" {
			return Classification{"documentation", "path segment " + `"` + s + `"`, "low"}
		}
	}
	return Classification{"route", "no specific pattern matched", "low"}
}
