package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"latex-bot/internal/bot"
	"latex-bot/internal/render"
)

const (
	renderConcurrency = 2
	renderTimeout     = 10 * time.Second
	messageTimeout    = 2 * time.Minute
)

func requireEnv(logger *slog.Logger, name string) string {
	value := os.Getenv(name)
	if value == "" {
		logger.Error("missing environment variable", "name", name)
		os.Exit(1)
	}
	return value
}

func main() {
	var level slog.Level
	if err := level.UnmarshalText([]byte(os.Getenv("LATEX_BOT_LOG_LEVEL"))); err != nil {
		level = slog.LevelInfo
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	options := []slack.Option{slack.OptionAppLevelToken(requireEnv(logger, "SLACK_APP_TOKEN"))}
	if apiURL := os.Getenv("SLACK_API_URL"); apiURL != "" {
		options = append(options, slack.OptionAPIURL(apiURL))
	}
	api := slack.New(requireEnv(logger, "SLACK_BOT_TOKEN"), options...)
	client := socketmode.New(api)

	replyMode, err := bot.ParseReplyMode(os.Getenv("LATEX_BOT_REPLY_MODE"))
	if err != nil {
		logger.Error("invalid LATEX_BOT_REPLY_MODE", "error", err)
		os.Exit(1)
	}
	logger.Info("starting", "reply_mode", replyMode, "command_upload_channel", os.Getenv("LATEX_BOT_COMMAND_UPLOAD_CHANNEL"))

	latexBot := &bot.Bot{
		Uploader:  api,
		Renderer:  render.NewRenderer(renderConcurrency, renderTimeout),
		Responder: bot.WebhookResponder{},
		Logger:    logger,
		ReplyMode: replyMode,

		CommandUploadChannel: os.Getenv("LATEX_BOT_COMMAND_UPLOAD_CHANNEL"),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		for event := range client.Events {
			switch event.Type {
			case socketmode.EventTypeConnected:
				logger.Info("connected to slack")
			case socketmode.EventTypeConnectionError:
				logger.Warn("slack connection error, retrying")
			case socketmode.EventTypeSlashCommand:
				client.Ack(*event.Request)
				command, ok := event.Data.(slack.SlashCommand)
				if !ok {
					continue
				}
				go func() {
					commandCtx, cancel := context.WithTimeout(ctx, messageTimeout)
					defer cancel()
					latexBot.HandleCommand(commandCtx, command)
				}()
			case socketmode.EventTypeEventsAPI:
				client.Ack(*event.Request)
				eventsAPIEvent, ok := event.Data.(slackevents.EventsAPIEvent)
				if !ok || eventsAPIEvent.Type != slackevents.CallbackEvent {
					continue
				}
				message, ok := eventsAPIEvent.InnerEvent.Data.(*slackevents.MessageEvent)
				if !ok {
					logger.Debug("ignoring event", "type", eventsAPIEvent.InnerEvent.Type)
					continue
				}
				logger.Debug("message received", "channel", message.Channel, "subtype", message.SubType, "bot", message.BotID != "", "length", len(message.Text))
				go func() {
					messageCtx, cancel := context.WithTimeout(ctx, messageTimeout)
					defer cancel()
					latexBot.HandleMessage(messageCtx, message)
				}()
			}
		}
	}()

	if err := client.RunContext(ctx); err != nil && ctx.Err() == nil {
		logger.Error("socket mode stopped", "error", err)
		os.Exit(1)
	}
}
