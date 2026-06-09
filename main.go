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
	"github.com/mandloideep/miniclaw/internal/services/categories"
	"github.com/mandloideep/miniclaw/internal/services/digest"
	"github.com/mandloideep/miniclaw/internal/services/email"
	"github.com/mandloideep/miniclaw/internal/services/gmailoauth"
	"github.com/mandloideep/miniclaw/internal/services/greet"
	"github.com/mandloideep/miniclaw/internal/services/inbox"
	"github.com/mandloideep/miniclaw/internal/services/keychain"
	"github.com/mandloideep/miniclaw/internal/services/ollama"
	"github.com/mandloideep/miniclaw/internal/services/summary"
	"github.com/mandloideep/miniclaw/internal/services/telegram"
	"github.com/mandloideep/miniclaw/internal/services/triage"
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
	triageSvc := triage.New(pool)
	catEngine := categories.New(pool)
	imapSyncer := email.NewIMAPSyncer(pool, accountSvc, triageSvc, func(from, unsub string) string {
		return string(categories.Classify(from, unsub))
	})
	gmailSync := gmailoauth.NewSyncer(pool, accountSvc)
	gmailAuth := gmailoauth.New()
	smtpSender := email.NewSMTPSender(accountSvc)
	llm := ollama.New()
	summarizer := summary.New(pool, llm)
	tg := telegram.New(pool)
	wsSvc := workspace.New(pool)
	digestSvc := digest.New(pool, accountSvc, wsSvc, tg)
	app := application.New(application.Options{
		Name:        "miniclaw",
		Description: "Local-AI email triage with Telegram digests",
		Services: []application.Service{
			application.NewService(greet.New()),
			application.NewService(keychain.New()),
			application.NewService(wsSvc),
			application.NewService(accountSvc),
			application.NewService(llm),
			application.NewService(imapSyncer),
			application.NewService(smtpSender),
			application.NewService(summarizer),
			application.NewService(tg),
			application.NewService(digestSvc),
			application.NewService(triageSvc),
			application.NewService(catEngine),
			application.NewService(gmailAuth),
			application.NewService(gmailSync),
			application.NewService(inbox.New(pool)),
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
		// Pick ingest path by auth kind. Errors here are real (transport,
		// auth) and worth surfacing in the log; summary failures are
		// swallowed inside Summarize.
		acc, err := accountSvc.Get(c, accountID)
		if err != nil {
			return err
		}
		switch acc.AuthKind {
		case account.AuthIMAP:
			if _, e := imapSyncer.Sync(c, accountID); e != nil {
				return e
			}
		case account.AuthGmailOAuth:
			if _, e := gmailSync.Sync(c, accountID); e != nil {
				return e
			}
		}
		_, _ = summarizer.Summarize(c, accountID)
		return nil
	})
	go sched.Start(schedCtx)
	go digestSvc.Start(schedCtx)

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
