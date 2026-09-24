package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/endpoint"
)

type licenseUpdateProvider struct {
	service *AppService
	mu      sync.Mutex
	used    map[string]*endpoint.Provider
}

func (p *licenseUpdateProvider) Name() string { return "license-update-service" }
func (p *licenseUpdateProvider) Check(ctx context.Context, request updater.CheckRequest) (*updater.Release, error) {
	p.service.authMu.RLock()
	token := p.service.license.Token()
	p.service.authMu.RUnlock()
	if token == "" {
		return nil, nil
	}
	base, err := normalizeServerURL(p.service.settingSnapshot().AuthServerURL)
	if err != nil {
		return nil, err
	}
	provider, err := endpoint.New(endpoint.Config{
		URL:     base + "/v1/updates/check",
		Channel: "stable",
		Headers: map[string]string{"Authorization": "Bearer " + token},
	})
	if err != nil {
		return nil, err
	}
	release, err := provider.Check(ctx, request)
	if err != nil || release == nil {
		return release, err
	}
	p.mu.Lock()
	if p.used == nil {
		p.used = map[string]*endpoint.Provider{}
	}
	p.used[release.Version] = provider
	p.mu.Unlock()
	return release, nil
}
func (p *licenseUpdateProvider) Download(ctx context.Context, release *updater.Release, destination io.Writer, progress func(written, total int64)) error {
	p.mu.Lock()
	provider := p.used[release.Version]
	delete(p.used, release.Version)
	p.mu.Unlock()
	if provider == nil {
		return errors.New("更新来源已失效，请重新检查更新")
	}
	return provider.Download(ctx, release, destination, progress)
}

func (s *AppService) initUpdater() error {
	provider := &licenseUpdateProvider{service: s, used: map[string]*endpoint.Provider{}}
	return s.app.Updater.Init(updater.Config{
		CurrentVersion: s.version,
		Providers:      []updater.Provider{provider},
		PublicKey:      updaterPublicKey,
		// The frontend checks on its own timer and asks the user before installing.
		CheckInterval: 0,
		Window:        updater.WindowNone,
	})
}

func (s *AppService) UpdaterCheck() OperationResult {
	if !s.authorized("") {
		return OperationResult{Message: "请先验证有效卡密，再检查更新"}
	}
	if s.app == nil || s.app.Updater == nil {
		return OperationResult{Message: "更新服务尚未初始化"}
	}
	release, err := s.app.Updater.Check(context.Background())
	if err != nil {
		return OperationResult{Message: err.Error()}
	}
	if release == nil {
		return OperationResult{OK: true, Message: "当前已是最新版本"}
	}
	return OperationResult{OK: true, Message: fmt.Sprintf("发现新版本 %s", release.Version)}
}

func (s *AppService) UpdaterInstall() OperationResult {
	if !s.authorized("") {
		return OperationResult{Message: "请先验证有效卡密，再安装更新"}
	}
	if s.app == nil || s.app.Updater == nil {
		return OperationResult{Message: "更新服务尚未初始化"}
	}
	if err := s.app.Updater.DownloadAndInstall(context.Background()); err != nil {
		return OperationResult{Message: err.Error()}
	}
	return OperationResult{OK: true, Message: "更新已准备完成，请重启应用"}
}

func (s *AppService) UpdaterRestart() OperationResult {
	if s.app == nil || s.app.Updater == nil {
		return OperationResult{Message: "更新服务尚未初始化"}
	}
	if err := s.app.Updater.Restart(context.Background()); err != nil {
		return OperationResult{Message: err.Error()}
	}
	return OperationResult{OK: true, Message: "正在重启应用"}
}
