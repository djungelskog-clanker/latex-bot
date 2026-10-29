package bot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"

	"latex-bot/internal/mathextract"
	"latex-bot/internal/render"
)

const (
	imageFilename = "math.png"
	maxAltTextLen = 1000
)

type Uploader interface {
	UploadFileContext(ctx context.Context, params slack.UploadFileParameters) (*slack.FileSummary, error)
}

type Renderer interface {
	Render(ctx context.Context, formula mathextract.Formula) ([]byte, error)
	RenderMessage(ctx context.Context, parts []mathextract.Part) ([]byte, error)
}

type ReplyMode string

const (
	ReplyInChannel ReplyMode = "channel"
	ReplyInThread  ReplyMode = "thread"
)

func ParseReplyMode(value string) (ReplyMode, error) {
	switch mode := ReplyMode(strings.ToLower(strings.TrimSpace(value))); mode {
	case "":
		return ReplyInChannel, nil
	case ReplyInChannel, ReplyInThread:
		return mode, nil
	default:
		return "", fmt.Errorf("unknown reply mode %q, expected %q or %q", value, ReplyInChannel, ReplyInThread)
	}
}

type Bot struct {
	Uploader  Uploader
	Renderer  Renderer
	Responder Responder
	Logger    *slog.Logger
	ReplyMode ReplyMode

	CommandUploadChannel string
}

func (b *Bot) replyThread(message *slackevents.MessageEvent) string {
	switch {
	case message.ThreadTimeStamp != "":
		return message.ThreadTimeStamp
	case b.ReplyMode == ReplyInThread:
		return message.TimeStamp
	default:
		return ""
	}
}

var handledSubtypes = map[string]bool{"": true, "file_share": true, "thread_broadcast": true}

func shouldHandle(message *slackevents.MessageEvent) bool {
	return handledSubtypes[message.SubType] && message.BotID == "" && message.Text != ""
}

func (b *Bot) HandleMessage(ctx context.Context, message *slackevents.MessageEvent) {
	if !shouldHandle(message) {
		return
	}
	parts := mathextract.Parse(message.Text)
	if len(parts) == 0 {
		return
	}
	b.Logger.Info("rendering message", "channel", message.Channel, "formulas", len(mathextract.Formulas(parts)))
	image, alt, ok := b.renderParts(ctx, parts)
	if !ok {
		b.Logger.Info("nothing rendered", "channel", message.Channel)
		return
	}

	_, err := b.Uploader.UploadFileContext(ctx, slack.UploadFileParameters{
		Reader:          bytes.NewReader(image),
		FileSize:        len(image),
		Filename:        imageFilename,
		Title:           "math",
		AltTxt:          alt,
		Channel:         message.Channel,
		ThreadTimestamp: b.replyThread(message),
	})
	if err != nil {
		b.Logger.Error("upload failed", "channel", message.Channel, "error", err)
		return
	}
	b.Logger.Info("uploaded image", "channel", message.Channel, "bytes", len(image))
}

func (b *Bot) renderParts(ctx context.Context, parts []mathextract.Part) ([]byte, string, bool) {
	formulas := mathextract.Formulas(parts)
	if len(formulas) > 1 {
		image, err := b.Renderer.RenderMessage(ctx, parts)
		if err == nil {
			return image, altText(partsText(parts)), true
		}
		b.logRenderError("rendering whole message failed, falling back to formulas", "", err)
	}

	var images [][]byte
	var rendered []string
	for _, formula := range formulas {
		image, err := b.Renderer.Render(ctx, formula)
		if err != nil {
			b.logRenderError("skipping formula", formula.Tex, err)
			continue
		}
		images = append(images, image)
		rendered = append(rendered, formula.Tex)
	}
	if len(images) == 0 {
		return nil, "", false
	}
	stacked, err := render.Stack(images)
	if err != nil {
		b.Logger.Error("stacking images failed", "error", err)
		return nil, "", false
	}
	return stacked, altText(strings.Join(rendered, "\n")), true
}

func (b *Bot) logRenderError(message, tex string, err error) {
	var invalidErr *render.InvalidFormulaError
	if errors.As(err, &invalidErr) {
		b.Logger.Info(message, "tex", tex, "reason", invalidErr.Reason)
		return
	}
	b.Logger.Error(message, "tex", tex, "error", err)
}

func partsText(parts []mathextract.Part) string {
	var text strings.Builder
	for _, part := range parts {
		if part.Formula != nil {
			text.WriteString("$" + part.Formula.Tex + "$")
		} else {
			text.WriteString(part.Text)
		}
	}
	return text.String()
}

func altText(text string) string {
	if len(text) <= maxAltTextLen {
		return text
	}
	return strings.ToValidUTF8(text[:maxAltTextLen-1], "") + "…"
}
