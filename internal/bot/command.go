package bot

import (
	"bytes"
	"context"
	"strings"
	"time"

	"github.com/slack-go/slack"

	"latex-bot/internal/mathextract"
)

const (
	commandUsage    = "usage: `/math \\frac{a}{b}` or `/math t$sum_(k=1)^n k$ is neat`. anything without `$` is rendered as one formula. heads up: everything rendered with /math is publicly accessible."
	responseRetries = 3
	responseBackoff = time.Second
)

type Responder interface {
	Respond(ctx context.Context, responseURL string, message *slack.WebhookMessage) error
}

type WebhookResponder struct{}

func (WebhookResponder) Respond(ctx context.Context, responseURL string, message *slack.WebhookMessage) error {
	return slack.PostWebhookContext(ctx, responseURL, message)
}

func commandParts(text string) []mathextract.Part {
	if parts := mathextract.Parse(text); len(parts) > 0 {
		return parts
	}
	return mathextract.Parse("$$" + text + "$$")
}

func (b *Bot) HandleCommand(ctx context.Context, command slack.SlashCommand) {
	text := strings.TrimSpace(command.Text)
	if b.CommandUploadChannel == "" {
		b.respondEphemeral(ctx, command.ResponseURL, "/math isn't set up yet, ask whoever runs latex-bot")
		return
	}
	if text == "" {
		b.respondEphemeral(ctx, command.ResponseURL, commandUsage)
		return
	}
	parts := commandParts(text)
	if len(parts) == 0 {
		b.respondEphemeral(ctx, command.ResponseURL, "that formula is too long. "+commandUsage)
		return
	}
	b.Logger.Info("rendering command", "user", command.UserID, "formulas", len(mathextract.Formulas(parts)))
	image, alt, ok := b.renderParts(ctx, parts)
	if !ok {
		b.respondEphemeral(ctx, command.ResponseURL, "couldn't render that, check the syntax. "+commandUsage)
		return
	}

	file, err := b.Uploader.UploadFileContext(ctx, slack.UploadFileParameters{
		Reader:   bytes.NewReader(image),
		FileSize: len(image),
		Filename: imageFilename,
		Title:    "math",
		AltTxt:   alt,
		Channel:  b.CommandUploadChannel,
	})
	if err != nil {
		b.Logger.Error("command upload failed", "error", err)
		b.respondEphemeral(ctx, command.ResponseURL, "rendered it but the upload failed, try again in a bit")
		return
	}

	message := &slack.WebhookMessage{
		ResponseType: slack.ResponseTypeInChannel,
		Text:         alt,
		Blocks: &slack.Blocks{BlockSet: []slack.Block{
			slack.NewContextBlock("", slack.NewTextBlockObject(slack.MarkdownType, "<@"+command.UserID+"> rendered:", false, false)),
			&slack.ImageBlock{Type: slack.MBTImage, AltText: alt, SlackFile: &slack.SlackFileObject{ID: file.ID}},
			slack.NewContextBlock("", slack.NewTextBlockObject(slack.MarkdownType, "images from `/math` are publicly accessible in <#"+b.CommandUploadChannel+">", false, false)),
		}},
	}
	for attempt := 1; ; attempt++ {
		err = b.Responder.Respond(ctx, command.ResponseURL, message)
		if err == nil {
			b.Logger.Info("answered command", "user", command.UserID)
			return
		}
		if attempt == responseRetries {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(responseBackoff * time.Duration(attempt)):
		}
	}
	b.Logger.Error("command response failed", "error", err)
	b.respondEphemeral(ctx, command.ResponseURL, "rendered it but couldn't post the image here, sorry")
}

func (b *Bot) respondEphemeral(ctx context.Context, responseURL, text string) {
	err := b.Responder.Respond(ctx, responseURL, &slack.WebhookMessage{ResponseType: slack.ResponseTypeEphemeral, Text: text})
	if err != nil {
		b.Logger.Error("ephemeral response failed", "error", err)
	}
}
