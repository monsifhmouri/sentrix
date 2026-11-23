package deduplication

import (
	"testing"

	"sentrix/pkg/models"
)

func TestKey(t *testing.T) {
	ep := models.KindEndpoint
	if Key(ep, "https://a.test/x?id=1", Strict) == Key(ep, "https://a.test/x?id=2", Strict) {
		t.Error("strict mode must keep different query values apart")
	}
	if Key(ep, "https://a.test/x?id=1&b=2", ParamNames) != Key(ep, "https://a.test/x?id=9&b=7", ParamNames) {
		t.Error("params mode must ignore query values")
	}
	if Key(ep, "https://a.test/x?id=1", ParamNames) == Key(ep, "https://a.test/x?other=1", ParamNames) {
		t.Error("params mode must still distinguish parameter names")
	}
	if Key(models.KindHost, "a.test", ParamNames) == Key(ep, "a.test", ParamNames) {
		t.Error("kinds must not collide")
	}
}

func TestParseMode(t *testing.T) {
	if m, ok := ParseMode("params"); !ok || m != ParamNames {
		t.Error("params should parse")
	}
	if _, ok := ParseMode("fuzzy"); ok {
		t.Error("fuzzy must be rejected")
	}
}
