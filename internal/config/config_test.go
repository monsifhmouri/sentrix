package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingFileGivesDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "none.json"))
	if err != nil || c.Concurrency != 3 || !c.Animation || !c.Sources["crtsh"].Enabled {
		t.Fatalf("%+v, %v", c, err)
	}
}

func TestPartialFileKeepsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.json")
	os.WriteFile(path, []byte(`{"concurrency":5,"sources":{"urlscan":{"api_key":"k","enabled":true}}}`), 0o600)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	u := c.Sources["urlscan"]
	if c.Concurrency != 5 || !c.Animation || c.Theme != "auto" || u.RatePerSecond != 0.5 || u.APIKey != "k" {
		t.Errorf("defaults lost: %+v / %+v", c, u)
	}
	if !c.Sources["otx"].Enabled {
		t.Error("untouched sources must keep defaults")
	}
}

func TestInvalidValuesRejected(t *testing.T) {
	for _, body := range []string{`{"concurrency":0}`, `{"concurrency":99}`, `{"log_level":"loud"}`, `{"export_formats":["pdf"]}`, `{"proxy":"nonsense"}`, `{bad`} {
		path := filepath.Join(t.TempDir(), "c.json")
		os.WriteFile(path, []byte(body), 0o600)
		if _, err := Load(path); err == nil {
			t.Errorf("%s should be rejected", body)
		}
	}
}

func TestEnvOverridesFileAndRedaction(t *testing.T) {
	c := Default()
	c.Sources["otx"] = SourceConfig{Enabled: true, APIKey: "from-file"}
	if c.APIKey("otx") != "from-file" {
		t.Fatal("file key not used")
	}
	t.Setenv("SENTRIX_OTX_API_KEY", "from-env")
	if c.APIKey("otx") != "from-env" {
		t.Error("environment must win")
	}
	if r := c.Redacted(); strings.Contains(r.Sources["otx"].APIKey, "from") {
		t.Error("secret leaked through Redacted")
	}
}

func TestSavePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "c.json")
	if err := Default().Save(path); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("config mode %v, want 0600", fi.Mode().Perm())
	}
}

func TestExpandHome(t *testing.T) {
	t.Setenv("HOME", "/home/u")
	t.Setenv("USERPROFILE", "/home/u")
	if got := ExpandHome("~/x/y"); got != filepath.Join("/home/u", "x/y") {
		t.Errorf("got %q", got)
	}
	if got := ExpandHome("/abs/~/x"); got != "/abs/~/x" {
		t.Errorf("only a leading ~ may expand, got %q", got)
	}
}
