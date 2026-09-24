package main

import (
	"embed"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist all:resources
var embeddedFiles embed.FS

//go:embed updater-public-key.pem
var updaterPublicKey []byte

var Version = "0.2.0"

func main() {
	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	// Keep Electron's productName-based userData directory so the existing
	// data/app.db, logs and configs remain available after the Wails migration.
	appDir := filepath.Join(configDir, "阿比整蛊复刻版")
	for _, dir := range []string{filepath.Join(appDir, "data"), filepath.Join(appDir, "logs"), filepath.Join(appDir, "assets")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			log.Fatal(err)
		}
	}
	if err := copyEmbeddedAssets(filepath.Join(appDir, "assets")); err != nil {
		log.Fatal(err)
	}
	store, err := OpenStore(filepath.Join(appDir, "data", "app.db"))
	if err != nil {
		log.Fatal(err)
	}
	service, err := NewAppService(store, appDir, Version)
	if err != nil {
		log.Fatal(err)
	}
	assets, err := fs.Sub(embeddedFiles, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}
	app := application.New(application.Options{
		Name:        "阿比直播工具",
		Description: "直播互动控制台",
		Services:    []application.Service{application.NewService(service)},
		Assets:      application.AssetOptions{Handler: newAssetHandler(assets, service)},
		Logger:      slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
		OnShutdown:  func() { service.shutdown() },
	})
	service.bind(app)
	if err := service.initUpdater(); err != nil {
		log.Fatal(err)
	}
	mainWindow := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "main", Title: "阿比直播工具", URL: "/#/control",
		Width: 900, Height: 600, MinWidth: 760, MinHeight: 500,
		BackgroundType:     application.BackgroundTypeSolid,
		BackgroundColour:   application.NewRGBA(244, 246, 249, 255),
		UseApplicationMenu: true,
	})
	service.setMainWindow(mainWindow)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func copyEmbeddedAssets(destination string) error {
	return fs.WalkDir(embeddedFiles, "resources", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path == "resources" {
				return os.MkdirAll(destination, 0o700)
			}
			return os.MkdirAll(filepath.Join(destination, filepath.FromSlash(path[len("resources/"):])), 0o700)
		}
		data, err := embeddedFiles.ReadFile(path)
		if err != nil {
			return err
		}
		output := filepath.Join(destination, filepath.FromSlash(path[len("resources/"):]))
		if _, err := os.Stat(output); err == nil {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(output), 0o700); err != nil {
			return err
		}
		return os.WriteFile(output, data, 0o600)
	})
}
