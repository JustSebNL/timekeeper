//go:build windows

package main

import (
	_ "embed"
	"runtime"

	"github.com/getlantern/systray"
)

//go:embed build/windows/icon.ico
var systemTrayIcon []byte

func startSystemTray(app *DesktopApp) {
	go func() {
		runtime.LockOSThread()
		systray.Run(func() {
			systray.SetIcon(systemTrayIcon)
			systray.SetTooltip("TimeKeeper")
			openItem := systray.AddMenuItem("Open TimeKeeper", "Show the TimeKeeper desktop window")
			exitItem := systray.AddMenuItem("Exit", "Exit TimeKeeper")

			go func() {
				for {
					select {
					case <-openItem.ClickedCh:
						app.showWindow()
					case <-exitItem.ClickedCh:
						app.exit()
					}
				}
			}()
		}, nil)
	}()
}

func stopSystemTray() { systray.Quit() }
