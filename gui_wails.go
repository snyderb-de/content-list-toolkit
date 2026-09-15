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

func launchGUI(startDir string) error {
	app := newApp(startDir)
	return wails.Run(&options.App{
		Title:            "Content List Toolkit",
		Width:            1100,
		Height:           720,
		MinWidth:         800,
		MinHeight:        600,
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
