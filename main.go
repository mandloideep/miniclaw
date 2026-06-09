// Package main wires the Wails v3 application: it embeds the built frontend,
// registers services from internal/, and opens the main window. Domain logic
// lives in internal/services/* — keep this file a slim wiring layer.
package main

import (
	"context"
	"embed"
	"log"
	"os"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/mandloideep/miniclaw/internal/db"
	"github.com/mandloideep/miniclaw/internal/scheduler"
	"github.com/mandloideep/miniclaw/internal/services/account"
	"github.com/mandloideep/miniclaw/internal/services/email"
	"github.com/mandloideep/miniclaw/internal/services/greet"
	"github.com/mandloideep/miniclaw/internal/services/keychain"
	"github.com/mandloideep/miniclaw/internal/services/ollama"
	"github.com/mandloideep/miniclaw/internal/services/summary"
	"github.com/mandloideep/miniclaw/internal/services/workspace"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	// Registering events at init makes them visible to the binding generator,
	// so the frontend gets typed wrappers for free.
	application.RegisterEvent[string]("time")
}

func main() {
	if err := run(); err != nil {
		log.Printf("miniclaw: %v", err)
		os.Exit(1)
	}
}

// run is main's body in error-returning form so deferred closes run on exit.
func run() error {
	ctx := context.Background()
	pool, err := db.Open(ctx, db.Config{})
	if err != nil {
		return err
	}
	defer func() { _ = pool.Close() }()

	accountSvc := account.New(pool)
	imapSyncer := email.NewIMAPSyncer(pool, accountSvc)
	smtpSender := email.NewSMTPSender(accountSvc)
	llm := ollama.New()
	summarizer := summary.New(pool, llm)
	app := application.New(application.Options{
		Name:        "miniclaw",
		Description: "Local-AI email triage with Telegram digests",
		Services: []application.Service{
			application.NewService(greet.New()),
			application.NewService(keychain.New()),
			application.NewService(workspace.New(pool)),
			application.NewService(accountSvc),
			application.NewService(llm),
			application.NewService(imapSyncer),
			application.NewService(smtpSender),
			application.NewService(summarizer),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "miniclaw",
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(27, 38, 54),
		URL:              "/",
	})

	// Start per-account ingest scheduler. Its context is cancelled when the
	// app exits Run, draining all per-account goroutines.
	schedCtx, cancelSched := context.WithCancel(ctx)
	defer cancelSched()
	sched := scheduler.New(accountSvc, func(c context.Context, accountID int64) error {
		if _, syncErr := imapSyncer.Sync(c, accountID); syncErr != nil {
			return syncErr
		}
		// Best-effort summarisation — a bad model config shouldn't stop
		// the ingest loop, so we log and swallow inside Summarize. Returning
		// nil here keeps the scheduler from spamming the log twice.
		_, _ = summarizer.Summarize(c, accountID)
		return nil
	})
	go sched.Start(schedCtx)

	go emitClockTick(app)
	return app.Run()
}

// emitClockTick is the scaffold's heartbeat — the frontend listens for "time"
// to confirm the bridge is live. Replace with real periodic work
// (sync, summarisation) once those services land.
func emitClockTick(app *application.App) {
	for {
		app.Event.Emit("time", time.Now().Format(time.RFC1123))
		time.Sleep(time.Second)
	}
}
