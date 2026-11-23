package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sentrix/internal/config"
)

func setup(t *testing.T) (cfgPath string) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.DataDir, cfg.OutputDir = filepath.Join(dir, "p"), filepath.Join(dir, "o")
	cfg.Animation = false
	cfgPath = filepath.Join(dir, "config.json")
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatal(err)
	}
	return cfgPath
}

func run(t *testing.T, cfg, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Run(append([]string{"-config", cfg, "-no-color"}, args...), strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

func TestVersionAboutAndUnknown(t *testing.T) {
	cfg := setup(t)
	if code, out, _ := run(t, cfg, "", "version"); code != 0 || !strings.Contains(out, "MONSIF HMOURI") {
		t.Errorf("version: %d %q", code, out)
	}
	if code, out, _ := run(t, cfg, "", "about"); code != 0 || !strings.Contains(out, "github.com/monsifhmouri") || !strings.Contains(out, "MONSTR MIND LABS") {
		t.Errorf("about: %d %q", code, out)
	}
	if code, _, errs := run(t, cfg, "", "frobnicate"); code != 2 || !strings.Contains(errs, "unknown command") {
		t.Errorf("unknown command: %d %q", code, errs)
	}
}

func TestScanRequiresConfirmation(t *testing.T) {
	cfg := setup(t)
	// Answering "n" must abort before any network use and save nothing.
	code, out, _ := run(t, cfg, "n\n", "scan", "-name", "p1", "-domain", "example.com", "-sources", "crtsh")
	if code != 0 || !strings.Contains(out, "cancelled; nothing was saved or sent") {
		t.Fatalf("%d %q", code, out)
	}
	if _, out, _ := run(t, cfg, "", "projects"); !strings.Contains(out, "no projects") {
		t.Errorf("project saved despite cancel: %q", out)
	}
	// EOF on stdin must also abort, not proceed.
	if code, out, _ := run(t, cfg, "", "scan", "-name", "p1", "-domain", "example.com"); strings.Contains(out, "Scanning") {
		t.Errorf("scan started without confirmation (code %d)", code)
	}
}

func TestValidationErrorsAreReported(t *testing.T) {
	cfg := setup(t)
	for _, args := range [][]string{
		{"scan", "-name", "x", "-domain", "bad domain"},
		{"scan", "-name", "x", "-domain", "example.com", "-concurrency", "999"},
		{"show", "-name", "ghost"},
		{"export", "-name", "ghost"},
		{"check"},
	} {
		if code, _, errs := run(t, cfg, "", args...); code != 1 || errs == "" {
			t.Errorf("%v: code %d stderr %q", args, code, errs)
		}
	}
}

func TestBackupRestoreExportShowFlow(t *testing.T) {
	cfg := setup(t)
	// Create the project through the store directly so no network is used.
	dir := filepath.Dir(cfg)
	a := mustApp(t, cfg)
	p, err := a.NewProject(appInput("flow"))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.Save(p); err != nil {
		t.Fatal(err)
	}

	if code, out, _ := run(t, cfg, "", "projects"); code != 0 || !strings.Contains(out, "flow") {
		t.Errorf("projects: %q", out)
	}
	if code, out, _ := run(t, cfg, "", "check", "-name", "flow"); code != 0 || !strings.Contains(out, "integrity OK") {
		t.Errorf("check: %q", out)
	}
	bak := filepath.Join(dir, "flow.bak")
	if code, _, errs := run(t, cfg, "", "backup", "-name", "flow", "-to", bak); code != 0 {
		t.Fatalf("backup: %s", errs)
	}
	if code, _, _ := run(t, cfg, "", "restore", "-from", bak); code != 1 {
		t.Error("restore over an existing project must fail without -overwrite")
	}
	if code, _, errs := run(t, cfg, "", "restore", "-from", bak, "-overwrite"); code != 0 {
		t.Errorf("restore -overwrite: %s", errs)
	}
	outDir := filepath.Join(dir, "exp")
	if code, _, errs := run(t, cfg, "", "export", "-name", "flow", "-out", outDir, "-format", "json,csv,txt,html"); code != 0 {
		t.Fatalf("export: %s", errs)
	}
	for _, ext := range []string{"json", "csv", "txt", "html"} {
		if _, err := os.Stat(filepath.Join(outDir, "flow."+ext)); err != nil {
			t.Errorf("missing export %s", ext)
		}
	}
	if code, out, _ := run(t, cfg, "", "show", "-name", "flow"); code != 0 || !strings.Contains(out, "0 of 0 records") {
		t.Errorf("show: %q", out)
	}
}

func TestConfigCommandMasksSecrets(t *testing.T) {
	cfg := setup(t)
	c, _ := config.Load(cfg)
	sc := c.Sources["otx"]
	sc.APIKey = "super-secret-value"
	c.Sources["otx"] = sc
	if err := c.Save(cfg); err != nil {
		t.Fatal(err)
	}
	_, out, _ := run(t, cfg, "", "config")
	if strings.Contains(out, "super-secret-value") || !strings.Contains(out, "********") {
		t.Errorf("secret handling wrong: %s", out)
	}
}

func TestInteractiveMenuExitAndAbout(t *testing.T) {
	cfg := setup(t)
	code, out, _ := run(t, cfg, "7\n0\n")
	if code != 0 || !strings.Contains(out, "ENGINEERED BY MONSIF HMOURI") || !strings.Contains(out, "About SENTRIX") {
		t.Errorf("menu: %d %q", code, out)
	}
	// Closing stdin must terminate the menu cleanly.
	if code, _, _ := run(t, cfg, ""); code != 0 {
		t.Errorf("EOF should exit 0, got %d", code)
	}
}

func TestInteractiveCreateCancelledAtReview(t *testing.T) {
	cfg := setup(t)
	// Includes one invalid domain to prove re-prompting, then declines the final confirmation.
	in := "1\nmyproj\nnot a domain\nexample.com\nexclude:dev.example.com\nAcme\n\ncrtsh\n\n\n\n\n\nn\n0\n"
	code, out, _ := run(t, cfg, in)
	if code != 0 || !strings.Contains(out, "invalid:") || !strings.Contains(out, "cancelled; nothing was saved or sent.") {
		t.Fatalf("%d\n%s", code, out)
	}
	if _, out, _ := run(t, cfg, "", "projects"); !strings.Contains(out, "no projects") {
		t.Errorf("unexpected project saved: %s", out)
	}
}
