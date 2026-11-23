package cli

import (
	"testing"

	"sentrix/internal/app"
)

func mustApp(t *testing.T, cfg string) *app.App {
	t.Helper()
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func appInput(name string) app.ProjectInput {
	return app.ProjectInput{Name: name, Domain: "example.com", Sources: []string{"crtsh"}}
}
