package tui

import (
	"context"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/config"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/provider"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/runtime"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

func testQuestionUI(t *testing.T, request schema.QuestionRequest) *UI {
	t.Helper()
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
	created, err := app.AskQuestion(current.ID, request.Questions, request.Tool)
	if err != nil {
		t.Fatal(err)
	}
	ui := New(app, nil, discardOutput{})
	ui.prepareQuestionState(created)
	return ui
}

func TestQuestionUICollectsMultipleQuestionsAndSelections(t *testing.T) {
	ui := testQuestionUI(t, schema.QuestionRequest{Questions: []schema.QuestionInfo{
		{Question: "Languages", Multiple: true, Options: []schema.QuestionOption{{Label: "Go"}, {Label: "Rust"}}},
		{Question: "Mode", Options: []schema.QuestionOption{{Label: "Build"}, {Label: "Plan"}}},
	}})
	ctx := context.Background()
	if !ui.handleQuestionKey(ctx, Key{Kind: KeyText, Text: "1"}) || !ui.handleQuestionKey(ctx, Key{Kind: KeyText, Text: "2"}) {
		t.Fatal("multiple selection was not handled")
	}
	if !ui.handleQuestionKey(ctx, Key{Kind: KeyEnter}) || ui.questionAt != 1 {
		t.Fatalf("question did not advance: at=%d", ui.questionAt)
	}
	if !ui.handleQuestionKey(ctx, Key{Kind: KeyText, Text: "2"}) || !ui.questionDone {
		t.Fatal("last question was not selected")
	}
	answers, err := ui.questionAnswers(ui.app.PendingQuestions(ui.app.CurrentSession().ID)[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(answers) != 2 || len(answers[0]) != 2 || answers[0][0] != "Go" || answers[0][1] != "Rust" || answers[1][0] != "Plan" {
		t.Fatalf("answers = %#v", answers)
	}
}

func TestQuestionUICustomAnswerAndSubmit(t *testing.T) {
	custom := true
	ui := testQuestionUI(t, schema.QuestionRequest{Questions: []schema.QuestionInfo{{Question: "Name", Custom: &custom}}})
	ctx := context.Background()
	if !ui.handleQuestionKey(ctx, Key{Kind: KeyText, Text: "c"}) || !ui.questionMode {
		t.Fatal("custom mode did not open")
	}
	for _, value := range []string{"Y", "T", "E", "A", "M"} {
		ui.handleQuestionKey(ctx, Key{Kind: KeyText, Text: value})
	}
	ui.handleQuestionKey(ctx, Key{Kind: KeyEnter})
	if ui.questionMode || !ui.questionDone {
		t.Fatal("custom answer did not complete")
	}
	request := ui.app.PendingQuestions(ui.app.CurrentSession().ID)[0]
	answers, err := ui.questionAnswers(request)
	if err != nil || len(answers) != 1 || len(answers[0]) != 1 || answers[0][0] != "YTEAM" {
		t.Fatalf("answers = %#v, err=%v", answers, err)
	}
}

type discardOutput struct{}

func (discardOutput) Write(data []byte) (int, error) { return len(data), nil }
