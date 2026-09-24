package main

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const localAssetURLPrefix = "/__local_assets/"

func newAssetHandler(frontend fs.FS, service *AppService) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(localAssetURLPrefix, http.HandlerFunc(service.serveLocalAsset))
	mux.Handle("/", application.AssetFileServerFS(frontend))
	return mux
}

// FeatureSelectLockMedia copies a chosen image or video into the app's media
// directory and returns a portable path suitable for feature settings.
func (s *AppService) FeatureSelectLockMedia() (string, error) {
	dialog := s.app.Dialog.OpenFile()
	dialog.SetOptions(&application.OpenFileDialogOptions{
		Title:                "选择锁屏背景图片或视频",
		CanChooseFiles:       true,
		CanChooseDirectories: false,
		Filters: []application.FileFilter{
			{DisplayName: "图片", Pattern: "*.png;*.jpg;*.jpeg;*.webp;*.gif;*.bmp"},
			{DisplayName: "视频", Pattern: "*.mp4;*.webm;*.mov;*.m4v"},
		},
		Window: s.mainWindow,
	})
	sourcePath, err := dialog.PromptForSingleSelection()
	if err != nil || sourcePath == "" {
		return "", err
	}

	extension := strings.ToLower(filepath.Ext(sourcePath))
	if !supportedLockMediaExtension(extension) {
		return "", errors.New("仅支持 PNG、JPG、WebP、GIF、BMP 图片和 MP4、WebM、MOV、M4V 视频")
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return "", err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return "", errors.New("所选文件为空或不是普通文件")
	}

	root, err := filepath.Abs(s.settingSnapshot().AssetsRoot)
	if err != nil {
		return "", err
	}
	relativeDir := filepath.Join("screen-lock", "backgrounds")
	destinationDir := filepath.Join(root, relativeDir)
	if err := os.MkdirAll(destinationDir, 0o700); err != nil {
		return "", err
	}
	filename := randomID() + extension
	destinationPath := filepath.Join(destinationDir, filename)
	destination, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(destination, source); err != nil {
		_ = destination.Close()
		_ = os.Remove(destinationPath)
		return "", err
	}
	if err := destination.Close(); err != nil {
		_ = os.Remove(destinationPath)
		return "", err
	}
	return filepath.ToSlash(filepath.Join(relativeDir, filename)), nil
}

func supportedLockMediaExtension(extension string) bool {
	switch strings.ToLower(extension) {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp", ".mp4", ".webm", ".mov", ".m4v":
		return true
	default:
		return false
	}
}

func (s *AppService) mediaURL(file string) (string, error) {
	root, err := filepath.Abs(s.settingSnapshot().AssetsRoot)
	if err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(file)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, absolute)
	if err != nil {
		return "", err
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("素材路径必须位于素材目录内")
	}
	return (&url.URL{Path: localAssetURLPrefix + filepath.ToSlash(relative)}).String(), nil
}

func (s *AppService) serveLocalAsset(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		response.Header().Set("Allow", "GET, HEAD")
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	relative := strings.TrimPrefix(request.URL.Path, localAssetURLPrefix)
	if relative == request.URL.Path || relative == "" || filepath.IsAbs(relative) || strings.Contains(relative, "\\") || filepath.Clean(relative) != relative {
		http.NotFound(response, request)
		return
	}

	root, err := filepath.Abs(s.settingSnapshot().AssetsRoot)
	if err != nil {
		http.NotFound(response, request)
		return
	}
	file, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		http.NotFound(response, request)
		return
	}
	inside, err := filepath.Rel(root, file)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		http.NotFound(response, request)
		return
	}

	// Resolve symlinks before opening the file so an in-root link cannot expose
	// files outside the configured media directory.
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		http.NotFound(response, request)
		return
	}
	realFile, err := filepath.EvalSymlinks(file)
	if err != nil {
		http.NotFound(response, request)
		return
	}
	inside, err = filepath.Rel(realRoot, realFile)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		http.NotFound(response, request)
		return
	}
	opened, err := os.Open(realFile)
	if err != nil {
		http.NotFound(response, request)
		return
	}
	defer opened.Close()
	info, err := opened.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(response, request)
		return
	}
	response.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(response, request, filepath.Base(realFile), info.ModTime(), opened)
}
