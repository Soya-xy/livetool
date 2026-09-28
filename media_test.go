package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// 本地素材路由的回归测试。
//
// 这里曾经有一个只在 Windows 上出现的缺陷：校验用的是 filepath.Clean，
// 它会把 "水果机/8.png" 规范化成 "水果机\8.png"，于是所有**子目录**素材都被判成
// 非法路径返回 404（根目录下的单个素材不受影响，所以一直没被发现）。
func TestServeLocalAssetNestedPaths(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "水果机")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("创建素材目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "8.png"), []byte("fake-png"), 0o600); err != nil {
		t.Fatalf("写入素材失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "fruit.mp3"), []byte("fake-mp3"), 0o600); err != nil {
		t.Fatalf("写入素材失败: %v", err)
	}

	service := &AppService{assetsRoot: root}
	service.settings.AssetsRoot = root

	cases := []struct {
		target string
		status int
	}{
		{"/__local_assets/%E6%B0%B4%E6%9E%9C%E6%9C%BA/8.png", http.StatusOK},
		{"/__local_assets/水果机/8.png", http.StatusOK},
		{"/__local_assets/fruit.mp3", http.StatusOK},
		{"/__local_assets/%E6%B0%B4%E6%9E%9C%E6%9C%BA/../fruit.mp3", http.StatusNotFound},
		{"/__local_assets/../app.db", http.StatusNotFound},
		{"/__local_assets/%2e%2e/app.db", http.StatusNotFound},
		{"/__local_assets/..%2fapp.db", http.StatusNotFound},
		{"/__local_assets/水果机\\8.png", http.StatusNotFound},
		{"/__local_assets/", http.StatusNotFound},
		{"/__local_assets/missing.png", http.StatusNotFound},
	}
	for _, item := range cases {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, item.target, nil)
		service.serveLocalAsset(recorder, request)
		if recorder.Code != item.status {
			t.Errorf("%s → %d，期望 %d", item.target, recorder.Code, item.status)
		}
	}
}

func TestMediaURLKeepsNestedPathReadable(t *testing.T) {
	root := t.TempDir()
	service := &AppService{assetsRoot: root}
	service.settings.AssetsRoot = root

	url, err := service.mediaURL(filepath.Join(root, "水果机", "8.png"))
	if err != nil {
		t.Fatalf("生成素材地址失败: %v", err)
	}
	// 中文目录名要按 UTF-8 百分号编码，路径分隔符保持斜杠。
	if url != "/__local_assets/%E6%B0%B4%E6%9E%9C%E6%9C%BA/8.png" {
		t.Errorf("素材地址不正确: %s", url)
	}
	if _, err := service.mediaURL(filepath.Join(filepath.Dir(root), "outside.png")); err == nil {
		t.Error("素材目录之外的路径应当被拒绝")
	}
}
