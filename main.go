package main

import (
	"embed"
	"log"

	"nooboo/models"
	"nooboo/services"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	cfg := services.NewConfigService()

	app := application.New(application.Options{
		Name:        "NooBoo",
		Description: "The Sims 4 mod manager",
		Services: []application.Service{
			application.NewService(cfg),
			application.NewService(services.NewModsService(cfg)),
			application.NewService(services.NewTrayService(cfg)),
			application.NewService(services.NewPackService(cfg)),
			application.NewService(services.NewBackupService(cfg)),
			application.NewService(services.NewExceptionService(cfg)),
			application.NewService(services.NewUpdateService()),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "NooBoo",
		Width:     760,
		Height:    520,
		MinWidth:  500,
		MinHeight: 500,
		// MaximiseButtonState:   application.ButtonDisabled,
		// FullscreenButtonState: application.ButtonDisabled,
		BackgroundColour: application.NewRGB(226, 241, 250),
		URL:              "/",
		EnableFileDrop:   true,
	})

	win.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		app.Event.Emit("files-dropped", e.Context().DroppedFiles())
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// progress is the event the frontend listens on during file work.
func init() {
	application.RegisterEvent[models.Progress]("progress")
}
