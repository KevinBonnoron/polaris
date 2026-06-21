package main

import (
	"embed"
	"log"
	"os"
	goruntime "runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

// appRef is the running application, used by the event emitter, dialogs and the
// browser opener. Set once in main() before Run().
var appRef *application.App

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

	appRef = application.New(application.Options{
		Name:        "Polaris",
		Description: "Desktop cockpit to orchestrate multiple AI coding agents",
		Icon:        appIcon,
		Services: []application.Service{
			application.NewService(app),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})

	window := appRef.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Polaris",
		Width:            1440,
		Height:           900,
		BackgroundColour: application.NewRGB(11, 12, 16),
		URL:              "/",
		EnableFileDrop:   true,
	})
	window.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		appRef.Quit()
	})
	// Without EnableFileDrop the webview navigates to dropped files, blanking the
	// window. With it on, the native layer captures the drop and fires this event
	// only when it lands on a [data-file-drop-target] element; we relay the paths
	// (plus that element's attributes, which carry the target agent id) to the UI.
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		appRef.Event.Emit("files:dropped", map[string]any{
			"files":   e.Context().DroppedFiles(),
			"details": e.Context().DropTargetDetails(),
		})
	})

	if err := appRef.Run(); err != nil {
		log.Fatalln("Error:", err.Error())
	}
}
