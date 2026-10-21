//go:build !windows

package main

// The current Wails desktop target is Windows. Other targets retain ordinary
// window-close behavior until their tray event-loop integration is implemented.
func startSystemTray(*DesktopApp) {}

func stopSystemTray() {}
