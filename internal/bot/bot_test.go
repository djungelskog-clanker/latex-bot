package bot

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"

	"latex-bot/internal/mathextract"
	"latex-bot/internal/render"
)

type fakeUploader struct {
	calls []slack.UploadFileParameters
	err   error
}

func (f *fakeUploader) UploadFileContext(_ context.Context, params slack.UploadFileParameters) (*slack.FileSummary, error) {
	f.calls = append(f.calls, params)
	return &slack.FileSummary{ID: "F1"}, f.err
}

type fakeResponder struct {
	messages []*slack.WebhookMessage
	failures int
}

func (f *fakeResponder) Respond(_ context.Context, _ string, message *slack.WebhookMessage) error {
	f.messages = append(f.messages, message)
	if f.failures > 0 {
		f.failures--
		return errors.New("invalid_blocks")
	}
	return nil
}

type fakeRenderer struct {
	rendered       []mathextract.Formula
	messages       [][]mathextract.Part
	failMessageErr error
}

func (f *fakeRenderer) RenderMessage(_ context.Context, parts []mathextract.Part) ([]byte, error) {
	f.messages = append(f.messages, parts)
	if f.failMessageErr != nil {
		return nil, f.failMessageErr
	}
	return onePixelPNG, nil
}

func (f *fakeRenderer) Render(_ context.Context, formula mathextract.Formula) ([]byte, error) {
	f.rendered = append(f.rendered, formula)
	switch {
	case strings.Contains(formula.Tex, "broken"):
		return nil, &render.InvalidFormulaError{Reason: "broken"}
	case strings.Contains(formula.Tex, "crash"):
		return nil, errors.New("latex missing")
	}
	return onePixelPNG, nil
}

var onePixelPNG = func() []byte {
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		panic(err)
	}
	return buffer.Bytes()
}()

func newTestBot() (*Bot, *fakeUploader, *fakeRenderer) {
	uploader := &fakeUploader{}
	renderer := &fakeRenderer{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Bot{Uploader: uploader, Renderer: renderer, Responder: &fakeResponder{}, Logger: logger, ReplyMode: ReplyInThread}, uploader, renderer
}

func message(text string) *slackevents.MessageEvent {
	return &slackevents.MessageEvent{Text: text, Channel: "C1", TimeStamp: "1.0"}
}

func TestSingleFormulaRendersAlone(t *testing.T) {
	bot, uploader, renderer := newTestBot()
	bot.HandleMessage(context.Background(), message("see $x^2$ ok"))
	if len(renderer.rendered) != 1 || len(renderer.messages) != 0 {
		t.Fatalf("expected a single formula render, got %d formulas and %d messages", len(renderer.rendered), len(renderer.messages))
	}
	call := uploader.calls[0]
	if call.Channel != "C1" || call.ThreadTimestamp != "1.0" || call.AltTxt != "x^2" || call.FileSize == 0 {
		t.Errorf("unexpected upload %+v", call)
	}
}

func TestSeveralFormulasRenderWithContext(t *testing.T) {
	bot, uploader, renderer := newTestBot()
	bot.HandleMessage(context.Background(), message("see $x^2$ and $$\\int x$$"))
	if len(renderer.messages) != 1 || len(renderer.rendered) != 0 {
		t.Fatalf("expected one whole-message render, got %d messages and %d formulas", len(renderer.messages), len(renderer.rendered))
	}
	if len(uploader.calls) != 1 || uploader.calls[0].AltTxt != "see $x^2$ and $\\int x$" {
		t.Errorf("unexpected uploads %+v", uploader.calls)
	}
}

func TestFallsBackToStackedFormulasWhenContextFails(t *testing.T) {
	bot, uploader, renderer := newTestBot()
	renderer.failMessageErr = &render.InvalidFormulaError{Reason: "mixed"}
	bot.HandleMessage(context.Background(), message("see $x^2$ and $$\\int x$$"))
	if len(renderer.rendered) != 2 {
		t.Fatalf("rendered %d formulas, want 2", len(renderer.rendered))
	}
	if len(uploader.calls) != 1 || uploader.calls[0].AltTxt != "x^2\n\\int x" {
		t.Errorf("unexpected uploads %+v", uploader.calls)
	}
}

func TestRepliesInExistingThread(t *testing.T) {
	bot, uploader, _ := newTestBot()
	msg := message("$x$")
	msg.ThreadTimeStamp = "0.5"
	bot.HandleMessage(context.Background(), msg)
	if uploader.calls[0].ThreadTimestamp != "0.5" {
		t.Errorf("thread_ts = %q, want 0.5", uploader.calls[0].ThreadTimestamp)
	}
}

func TestSkipsBrokenFormulasButKeepsGoodOnes(t *testing.T) {
	bot, uploader, renderer := newTestBot()
	renderer.failMessageErr = &render.InvalidFormulaError{Reason: "broken"}
	bot.HandleMessage(context.Background(), message("$broken$ and $y$ and $crash$"))
	if len(uploader.calls) != 1 || uploader.calls[0].AltTxt != "y" {
		t.Errorf("unexpected uploads %+v", uploader.calls)
	}
}

func TestNoUploadWhenNothingRenders(t *testing.T) {
	bot, uploader, _ := newTestBot()
	bot.HandleMessage(context.Background(), message("$broken$"))
	bot.HandleMessage(context.Background(), message("it costs $5 and $10"))
	if len(uploader.calls) != 0 {
		t.Errorf("expected no uploads, got %d", len(uploader.calls))
	}
}

func TestIgnoresBotsEditsAndDeletes(t *testing.T) {
	bot, uploader, renderer := newTestBot()
	fromBot := message("$x$")
	fromBot.BotID = "B1"
	edited := message("$x$")
	edited.SubType = "message_changed"
	deleted := message("$x$")
	deleted.SubType = "message_deleted"
	for _, msg := range []*slackevents.MessageEvent{fromBot, edited, deleted} {
		bot.HandleMessage(context.Background(), msg)
	}
	if len(uploader.calls) != 0 || len(renderer.rendered) != 0 {
		t.Errorf("expected messages to be ignored")
	}
}

func TestHandlesFileShareAndBroadcasts(t *testing.T) {
	bot, uploader, _ := newTestBot()
	for _, subtype := range []string{"file_share", "thread_broadcast"} {
		msg := message("$x$")
		msg.SubType = subtype
		bot.HandleMessage(context.Background(), msg)
	}
	if len(uploader.calls) != 2 {
		t.Errorf("uploaded %d times, want 2", len(uploader.calls))
	}
}

func TestUploadFailureDoesNotPanic(t *testing.T) {
	bot, uploader, _ := newTestBot()
	uploader.err = errors.New("boom")
	bot.HandleMessage(context.Background(), message("$x$"))
	if len(uploader.calls) != 1 {
		t.Errorf("expected one upload attempt")
	}
}

func TestAltTextIsTruncated(t *testing.T) {
	text := altText(strings.Repeat("é", 800))
	if len(text) > maxAltTextLen+len("…") || !strings.HasSuffix(text, "…") {
		t.Errorf("alt text not truncated properly: len %d", len(text))
	}
}

func TestChannelModePostsToChannel(t *testing.T) {
	bot, uploader, _ := newTestBot()
	bot.ReplyMode = ReplyInChannel
	bot.HandleMessage(context.Background(), message("$x$"))
	if uploader.calls[0].ThreadTimestamp != "" || uploader.calls[0].Channel != "C1" {
		t.Errorf("expected a top-level channel post, got %+v", uploader.calls[0])
	}
}

func TestChannelModeStillRepliesInsideThreads(t *testing.T) {
	bot, uploader, _ := newTestBot()
	bot.ReplyMode = ReplyInChannel
	msg := message("$x$")
	msg.ThreadTimeStamp = "0.5"
	bot.HandleMessage(context.Background(), msg)
	if uploader.calls[0].ThreadTimestamp != "0.5" {
		t.Errorf("thread_ts = %q, want 0.5", uploader.calls[0].ThreadTimestamp)
	}
}

func TestParseReplyMode(t *testing.T) {
	cases := map[string]ReplyMode{"": ReplyInChannel, "channel": ReplyInChannel, " Thread ": ReplyInThread}
	for input, want := range cases {
		if got, err := ParseReplyMode(input); err != nil || got != want {
			t.Errorf("ParseReplyMode(%q) = %q, %v, want %q", input, got, err, want)
		}
	}
	if _, err := ParseReplyMode("dm"); err == nil {
		t.Errorf("expected error for unknown mode")
	}
}
