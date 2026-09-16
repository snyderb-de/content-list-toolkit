//go:build !gui

package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// Window size defaults, shared with the View menu's reset item.
const (
	defaultWindowWidth  = 1100
	defaultWindowHeight = 720
)

func launchGUI(startDir string) error {
	app := newApp(startDir)
	return wails.Run(&options.App{
		Title:     "Content List Toolkit",
		Width:     defaultWindowWidth,
		Height:    defaultWindowHeight,
		MinWidth:  800,
		MinHeight: 600,
		// MaxWidth and MaxHeight are deliberately left at zero. Wails reads a
		// zero as unbounded, and any value here would cap both maximise and
		// full screen.
		Menu:             buildAppMenu(app),
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.startup,
		Bind: []interface{}{
			app,
		},
	})
}
