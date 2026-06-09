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
	"github.com/mandloideep/miniclaw/internal/services/attachments"
	"github.com/mandloideep/miniclaw/internal/services/categories"
	"github.com/mandloideep/miniclaw/internal/services/digest"
	"github.com/mandloideep/miniclaw/internal/services/email"
	"github.com/mandloideep/miniclaw/internal/services/gmailoauth"
	"github.com/mandloideep/miniclaw/internal/services/greet"
	"github.com/mandloideep/miniclaw/internal/services/inbox"
	"github.com/mandloideep/miniclaw/internal/services/keychain"
	"github.com/mandloideep/miniclaw/internal/services/msoauth"
	"github.com/mandloideep/miniclaw/internal/services/ollama"
	"github.com/mandloideep/miniclaw/internal/services/planner"
	"github.com/mandloideep/miniclaw/internal/services/snooze"
	"github.com/mandloideep/miniclaw/internal/services/summary"
	"github.com/mandloideep/miniclaw/internal/services/telegram"
	"github.com/mandloideep/miniclaw/internal/services/triage"
	"github.com/mandloideep/miniclaw/internal/services/workspace"
)

//go:embed all:frontend/dist
var assets embed.FS

// SyncProgress is the payload of the "sync_progress" event the backend
// fires whenever an account sync starts, finishes, or fails. The status
// bar subscribes to it and shows a live "syncing N/N" pill.
type SyncProgress struct {
	AccountID    int64  `json:"accountId"`
	EmailAddress string `json:"emailAddress"`
	Phase        string `json:"phase"` // "start" | "done" | "error"
	Written      int    `json:"written,omitempty"`
	Err          string `json:"err,omitempty"`
}

func init() {
	// Registering events at init makes them visible to the binding generator,
	// so the frontend gets typed wrappers for free.
	application.RegisterEvent[string]("time")
	application.RegisterEvent[SyncProgress]("sync_progress")
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
	attachSvc := attachments.New(pool)
	imapSyncer := email.NewIMAPSyncer(pool, accountSvc, triageSvc, func(from, unsub string) string {
		return string(categories.Classify(from, unsub))
	})
	imapSyncer.AttachInlineStore(attachSvc)
	gmailSync := gmailoauth.NewSyncer(pool, accountSvc)
	gmailSync.AttachInlineStore(attachSvc)
	gmailSync.AttachTriage(triageSvc)
	gmailSync.AttachThreadDeriver(email.DeriveThreadID)
	gmailAuth := gmailoauth.New()
	msSync := msoauth.NewSyncer(pool, accountSvc)
	msAuth := msoauth.New()
	smtpSender := email.NewSMTPSender(accountSvc)
	llm := ollama.New()
	summarizer := summary.New(pool, llm)
	tg := telegram.New(pool)
	wsSvc := workspace.New(pool)
	digestSvc := digest.New(pool, accountSvc, wsSvc, tg)
	snoozeSvc := snooze.New(pool)
	snoozeSvc.AttachNotifier(tg)
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
			application.NewService(msAuth),
			application.NewService(msSync),
			application.NewService(inbox.New(pool)),
			application.NewService(attachSvc),
			application.NewService(snoozeSvc),
			application.NewService(planner.NewCalendar(pool, accountSvc)),
			application.NewService(planner.NewTodos(pool)),
			application.NewService(planner.NewNotes(pool)),
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
		app.Event.Emit("sync_progress", SyncProgress{
			AccountID: accountID, EmailAddress: acc.EmailAddress, Phase: "start",
		})
		written := 0
		var syncErr error
		switch acc.AuthKind {
		case account.AuthIMAP:
			written, syncErr = imapSyncer.Sync(c, accountID)
		case account.AuthGmailOAuth:
			written, syncErr = gmailSync.Sync(c, accountID)
		case account.AuthMSOAuth:
			written, syncErr = msSync.Sync(c, accountID)
		}
		if syncErr != nil {
			app.Event.Emit("sync_progress", SyncProgress{
				AccountID: accountID, EmailAddress: acc.EmailAddress,
				Phase: "error", Err: syncErr.Error(),
			})
			return syncErr
		}
		_, _ = summarizer.Summarize(c, accountID)
		app.Event.Emit("sync_progress", SyncProgress{
			AccountID: accountID, EmailAddress: acc.EmailAddress,
			Phase: "done", Written: written,
		})
		return nil
	})
	go sched.Start(schedCtx)
	go digestSvc.Start(schedCtx)
	go snoozeSvc.Start(schedCtx)
	go warmOllamaModels(schedCtx, accountSvc, llm)

	// Real-time IMAP push: keeps one IDLE connection per IMAP account so new
	// mail is noticed within hundreds of ms instead of waiting for the next
	// cadence tick. Failures back off and reconnect; non-IMAP accounts are
	// ignored.
	idleSvc := email.NewIDLE(accountSvc, func(c context.Context, accountID int64) {
		if _, e := imapSyncer.Sync(c, accountID); e != nil {
			log.Printf("idle sync account %d: %v", accountID, e)
			return
		}
		_, _ = summarizer.Summarize(c, accountID)
	})
	go idleSvc.Start(schedCtx)

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

// warmOllamaModels asks Ollama to load each configured model into memory
// shortly after boot, so the first summarisation pass doesn't trip the
// generate timeout while the model cold-starts. Best-effort: if Ollama
// is down or the model isn't pulled yet, the failure just logs and
// SetLastError flips so the UI banner surfaces it.
func warmOllamaModels(ctx context.Context, accountSvc *account.Service, llm *ollama.Service) {
	// Small delay so the UI is up before we hit Ollama — keeps the first
	// frame snappy even when warm-up blocks for a while.
	select {
	case <-ctx.Done():
		return
	case <-time.After(2 * time.Second):
	}
	accounts, err := accountSvc.List(ctx)
	if err != nil {
		log.Printf("warm: list accounts: %v", err)
		return
	}
	seen := map[string]bool{}
	for _, a := range accounts {
		m := a.OllamaModel
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		if err := llm.Warm(ctx, m); err != nil {
			log.Printf("warm %s: %v", m, err)
			continue
		}
		log.Printf("warm %s: ready", m)
	}
}
