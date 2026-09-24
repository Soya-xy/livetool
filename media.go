package main

import (
	"errors"
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
