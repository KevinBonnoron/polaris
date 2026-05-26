package main

import (
	"embed"
	"os"
	goruntime "runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// GDK_BACKEND is forced to x11 on Wayland via a cgo constructor in
// env_linux.go — setting it from main() runs too late, after cgo
// libraries have already cached the value.

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func main() {
	if goruntime.GOOS == "linux" {
		if _, set := os.LookupEnv("WEBKIT_DISABLE_DMABUF_RENDERER"); !set {
			_ = os.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "1")
		}
		if _, set := os.LookupEnv("WEBKIT_DISABLE_COMPOSITING_MODE"); !set {
			_ = os.Setenv("WEBKIT_DISABLE_COMPOSITING_MODE", "1")
		}
	}

	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "Polaris",
		Width:  1440,
		Height: 900,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 11, G: 12, B: 16, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
		Linux: &linux.Options{
			Icon:             appIcon,
			ProgramName:      "Polaris",
			WebviewGpuPolicy: linux.WebviewGpuPolicyOnDemand,
		},
		Windows: &windows.Options{
			Theme: windows.Dark,
		},
		Mac: &mac.Options{
			About: &mac.AboutInfo{
				Title: "Polaris",
				Icon:  appIcon,
			},
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
