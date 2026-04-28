package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:     "TrustDB Attest",
		Width:     1200,
		Height:    800,
		MinWidth:  960,
		MinHeight: 640,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		// Native drag-and-drop: the WebView forwards OS file drops
		// so we can call OnFileDrop in TypeScript and get real
		// filesystem paths — essential because the browser File
		// API can't read paths, only content.
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:     true,
			DisableWebViewDrop: true,
			CSSDropProperty:    "--wails-drop-target",
			CSSDropValue:       "drop",
		},
		// Match the Loki-inspired dark command surface so native
		// Windows chrome does not flash a pale slab during startup.
		BackgroundColour: &options.RGBA{R: 7, G: 8, B: 7, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
			Theme:                windows.Dark,
			CustomTheme: &windows.ThemeSettings{
				DarkModeTitleBar:          windows.RGB(7, 8, 7),
				DarkModeTitleBarInactive:  windows.RGB(17, 19, 17),
				DarkModeTitleText:         windows.RGB(248, 255, 233),
				DarkModeTitleTextInactive: windows.RGB(105, 113, 100),
				DarkModeBorder:            windows.RGB(0, 255, 34),
				DarkModeBorderInactive:    windows.RGB(37, 42, 35),
			},
		},
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
