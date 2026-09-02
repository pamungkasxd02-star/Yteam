package tui

import (
	"bytes"
	"context"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/config"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/permission"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/runtime"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

func TestHomeAndPickerRenderContainsUserFacingState(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	store, err := session.Open(home, root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	app := runtime.New(config.Config{Home: home, Model: "test-model"}, root, store, current, provider.New("http://127.0.0.1:1", ""))
	var output bytes.Buffer
	ui := New(app, bytes.NewBuffer(nil), &output)
	ui.picker = NewPicker("Select agent", []PickerItem{{ID: "build", Label: "build", Description: "Implement changes"}})
	ui.draw()
	text := output.String()
	for _, expected := range []string{"YTEAM", "Home", "Select agent", "build", "test-model"} {
		if !bytes.Contains([]byte(text), []byte(expected)) {
			t.Fatalf("missing %q in %s", expected, text)
		}
	}
}

func TestPermissionPromptIsRendered(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	store, err := session.Open(home, root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	app := runtime.New(config.Config{Home: home, Model: "test"}, root, store, current, provider.New("http://127.0.0.1:1", ""))
	request, err := app.Permissions.Assert(current.ID, "edit", "note.txt")
	if err != nil {
		t.Fatal(err)
	}
	if request.ID == "" {
		t.Fatal("missing permission ID")
	}
	var output bytes.Buffer
	ui := New(app, bytes.NewBuffer(nil), &output)
	ui.draw()
	for _, expected := range []string{"Permission required", "edit", "note.txt", "y=once", "a=always", "n=reject"} {
		if !bytes.Contains(output.Bytes(), []byte(expected)) {
			t.Fatalf("missing %q in %s", expected, output.String())
		}
	}
	_ = permission.Once
}

func TestQuestionPromptIsRendered(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	store, err := session.Open(home, root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	app := runtime.New(config.Config{Home: home, Model: "test"}, root, store, current, provider.New("http://127.0.0.1:1", ""))
	request, err := app.AskQuestion(current.ID, []schema.QuestionInfo{{Question: "Use Go?", Header: "Choice", Options: []schema.QuestionOption{{Label: "Yes", Description: "Continue"}}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if request.ID == "" {
		t.Fatal("missing question ID")
	}
	var output bytes.Buffer
	ui := New(app, bytes.NewBuffer(nil), &output)
	ui.draw()
	for _, expected := range []string{"Question: Use Go?", "1. Yes", "Type an answer number"} {
		if !bytes.Contains(output.Bytes(), []byte(expected)) {
			t.Fatalf("missing %q in %s", expected, output.String())
		}
	}
}

func TestTUIHandlesCoreLocalCommandsWithoutProvider(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	store, err := session.Open(home, root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	app := runtime.New(config.Config{Home: home, Model: "test"}, root, store, current, provider.New("http://127.0.0.1:1", ""))
	var output bytes.Buffer
	ui := New(app, bytes.NewBuffer(nil), &output)
	handled, err := ui.command(context.Background(), "/help")
	if !handled || err != nil || !bytes.Contains(output.Bytes(), []byte("OpenCode commands")) {
		t.Fatalf("help handled=%v err=%v output=%q", handled, err, output.String())
	}
}

func TestTUIPaletteUsesCanonicalCommandItems(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	store, err := session.Open(home, root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.New()
	if err != nil {
		t.Fatal(err)
	}
	app := runtime.New(config.Config{Home: home, Model: "test"}, root, store, current, provider.New("http://127.0.0.1:1", ""))
	ui := New(app, bytes.NewBuffer(nil), &bytes.Buffer{})
	handled, err := ui.command(context.Background(), "/palette")
	if !handled || err != nil || ui.picker == nil || ui.pickerKind != "command" {
		t.Fatalf("palette handled=%v err=%v picker=%#v kind=%q", handled, err, ui.picker, ui.pickerKind)
	}
	ui.picker.SetQuery("/status")
	item, ok := ui.picker.Selected()
	if !ok || item.ID != "/status" {
		t.Fatalf("palette selection = %#v, %v", item, ok)
	}
	if err := ui.selectPicker(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ui.picker != nil {
		t.Fatal("palette picker was not closed")
	}
}
