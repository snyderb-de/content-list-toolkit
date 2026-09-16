//go:build !gui

package main

import (
	goRuntime "runtime"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// buildAppMenu gives the window a menu bar.
//
// Two things depend on it.
//
// Full screen on macOS is not otherwise reachable. Wails only adds
// NSWindowCollectionBehaviorFullScreenPrimary to the window when the app is
// configured to start in full screen, so a normally started window has no
// full-screen capability at all and the green button only zooms. Calling the
// runtime directly works regardless, and a menu item is where a person looks
// for it.
//
// Copy and paste on macOS also depend on a menu. Without an Edit menu the
// standard shortcuts are never bound, so Cmd+C in a text field does nothing —
// which matters on a screen built around correcting text.
func buildAppMenu(app *App) *menu.Menu {
	appMenu := menu.NewMenu()

	if goRuntime.GOOS == "darwin" {
		appMenu.Append(menu.AppMenu())
		appMenu.Append(menu.EditMenu())
	}

	view := appMenu.AddSubmenu("View")
	view.AddText("Toggle Full Screen", keys.Combo("f", keys.CmdOrCtrlKey, keys.ControlKey), func(_ *menu.CallbackData) {
		toggleFullscreen(app)
	})
	view.AddText("Zoom / Maximise", keys.CmdOrCtrl("m"), func(_ *menu.CallbackData) {
		if app.ctx == nil {
			return
		}
		wailsRuntime.WindowToggleMaximise(app.ctx)
	})
	view.AddSeparator()
	view.AddText("Reset Window Size", nil, func(_ *menu.CallbackData) {
		if app.ctx == nil {
			return
		}
		wailsRuntime.WindowUnmaximise(app.ctx)
		wailsRuntime.WindowSetSize(app.ctx, defaultWindowWidth, defaultWindowHeight)
		wailsRuntime.WindowCenter(app.ctx)
	})

	return appMenu
}

// toggleFullscreen leaves full screen if the window is already in it, so the
// same menu item and shortcut work in both directions.
func toggleFullscreen(app *App) {
	if app.ctx == nil {
		return
	}
	if wailsRuntime.WindowIsFullscreen(app.ctx) {
		wailsRuntime.WindowUnfullscreen(app.ctx)
		return
	}
	wailsRuntime.WindowFullscreen(app.ctx)
}
