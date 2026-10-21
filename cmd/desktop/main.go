package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log"
	"time"

	"github.com/JustSebNL/timekeeper/internal/desktop"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed assets/*
var embeddedAssets embed.FS

func main() {
	coreURL := flag.String("core-url", desktop.DefaultLocalPrimaryURL, "preferred local TimeKeeper Core endpoint")
	coreExecutable := flag.String("core-exe", "", "absolute path to the TimeKeeper Core executable")
	databasePath := flag.String("db", "", "absolute path to the Core database")
	dashboardPath := flag.String("ui", "", "absolute path to the dashboard index.html")
	hideOnClose := flag.Bool("hide-on-close", false, "hide the desktop window to the system tray when closed")
	flag.Parse()

	app := newDesktopApp(desktop.Config{
		CoreURL:        *coreURL,
		FallbackURL:    desktop.DefaultLocalFallbackURL,
		CoreExecutable: *coreExecutable,
		DatabasePath:   *databasePath,
		DashboardPath:  *dashboardPath,
	}, func(ctx context.Context, cfg desktop.Config) (coreSession, error) {
		return desktop.StartOrAttach(ctx, cfg, desktop.ExecLauncher{})
	})
	app.notifications = wailsNotificationProvider{}
	app.window = wailsWindowController{}
	app.hideOnClose = *hideOnClose
	assets, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		log.Fatalf("load embedded desktop assets: %v", err)
	}

	err = wails.Run(&options.App{
		Title:       "TimeKeeper",
		Width:       1000,
		Height:      700,
		MinWidth:    760,
		MinHeight:   520,
		AssetServer: &assetserver.Options{Assets: assets},
		OnStartup: func(ctx context.Context) {
			app.startup(ctx)
			startSystemTray(app)
		},
		OnShutdown: func(ctx context.Context) {
			stopSystemTray()
			shutdownCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
			defer cancel()
			if err := app.shutdown(shutdownCtx); err != nil {
				log.Printf("desktop Core shutdown: %v", err)
			}
		},
		OnBeforeClose: func(ctx context.Context) bool {
			return app.beforeClose(ctx)
		},
		Bind: []interface{}{app},
	})
	if err != nil {
		log.Fatalf("run TimeKeeper desktop: %v", err)
	}
}
