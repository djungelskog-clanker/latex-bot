package bot

import (
	"context"
	"strings"
	"testing"

	"github.com/slack-go/slack"

	"latex-bot/internal/render"
)

func newCommandBot() (*Bot, *fakeUploader, *fakeRenderer, *fakeResponder) {
	bot, uploader, renderer := newTestBot()
	responder := &fakeResponder{}
	bot.Responder = responder
	bot.CommandUploadChannel = "C9"
	return bot, uploader, renderer, responder
}

func command(text string) slack.SlashCommand {
	return slack.SlashCommand{Command: "/math", Text: text, UserID: "U1", ChannelID: "D1", ResponseURL: "https://hooks.example/1"}
}

func TestCommandRendersBareFormula(t *testing.T) {
	bot, uploader, renderer, responder := newCommandBot()
	bot.HandleCommand(context.Background(), command(`\frac{a}{b}`))
	if len(renderer.rendered) != 1 || renderer.rendered[0].Tex != `\frac{a}{b}` || !renderer.rendered[0].Display {
		t.Fatalf("unexpected renders %+v", renderer.rendered)
	}
	if len(uploader.calls) != 1 || uploader.calls[0].Channel != "C9" {
		t.Fatalf("expected one upload to the upload channel, got %+v", uploader.calls)
	}
	if len(responder.messages) != 1 {
		t.Fatalf("expected one response, got %d", len(responder.messages))
	}
	message := responder.messages[0]
	if message.ResponseType != slack.ResponseTypeInChannel {
		t.Errorf("response type = %q", message.ResponseType)
	}
	image, ok := message.Blocks.BlockSet[1].(*slack.ImageBlock)
	if !ok || image.SlackFile == nil || image.SlackFile.ID != "F1" {
		t.Errorf("expected image block for F1, got %+v", message.Blocks.BlockSet)
	}
}

func TestCommandKeepsDollarSyntax(t *testing.T) {
	bot, _, renderer, _ := newCommandBot()
	bot.HandleCommand(context.Background(), command(`so t$a/b$ and $c$`))
	if len(renderer.messages) != 1 {
		t.Fatalf("expected whole-message render, got %d", len(renderer.messages))
	}
}

func TestCommandWithoutTextShowsUsage(t *testing.T) {
	bot, uploader, _, responder := newCommandBot()
	bot.HandleCommand(context.Background(), command("   "))
	if len(uploader.calls) != 0 || len(responder.messages) != 1 || responder.messages[0].ResponseType != slack.ResponseTypeEphemeral || !strings.Contains(responder.messages[0].Text, "usage") {
		t.Errorf("expected ephemeral usage, got %+v", responder.messages)
	}
}

func TestCommandReportsBrokenFormula(t *testing.T) {
	bot, uploader, renderer, responder := newCommandBot()
	renderer.failMessageErr = &render.InvalidFormulaError{Reason: "broken"}
	bot.HandleCommand(context.Background(), command("broken"))
	if len(uploader.calls) != 0 || len(responder.messages) != 1 || responder.messages[0].ResponseType != slack.ResponseTypeEphemeral {
		t.Errorf("expected ephemeral error, got %+v", responder.messages)
	}
}

func TestCommandRetriesResponse(t *testing.T) {
	bot, _, _, responder := newCommandBot()
	responder.failures = 1
	bot.HandleCommand(context.Background(), command("x"))
	if len(responder.messages) != 2 || responder.messages[1].ResponseType != slack.ResponseTypeInChannel {
		t.Errorf("expected a retried in-channel response, got %d messages", len(responder.messages))
	}
}

func TestCommandMentionsPublicUploads(t *testing.T) {
	bot, _, _, responder := newCommandBot()
	bot.HandleCommand(context.Background(), command("x"))
	notice, ok := responder.messages[0].Blocks.BlockSet[2].(*slack.ContextBlock)
	if !ok || !strings.Contains(notice.ContextElements.Elements[0].(*slack.TextBlockObject).Text, "publicly accessible in <#C9>") {
		t.Errorf("expected a public notice, got %+v", responder.messages[0].Blocks.BlockSet)
	}
}

func TestCommandWithoutUploadChannelIsDisabled(t *testing.T) {
	bot, uploader, _, responder := newCommandBot()
	bot.CommandUploadChannel = ""
	bot.HandleCommand(context.Background(), command("x"))
	if len(uploader.calls) != 0 || responder.messages[0].ResponseType != slack.ResponseTypeEphemeral {
		t.Errorf("expected command to be disabled")
	}
}
