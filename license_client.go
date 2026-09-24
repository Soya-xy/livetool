package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type licenseClient struct {
	client    *http.Client
	token     string
	serverURL string
}

type remoteAuthSession struct {
	ExpiresAt        string   `json:"expires_at"`
	SessionExpiresAt string   `json:"session_expires_at"`
	Features         []string `json:"features"`
	SafeCodeSet      bool     `json:"safe_code_set"`
}

func newLicenseClient() *licenseClient {
	return &licenseClient{client: &http.Client{Timeout: 10 * time.Second}}
}

func normalizeServerURL(input string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(input))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", errors.New("请输入有效的授权服务器根地址")
	}
	loopback := strings.EqualFold(u.Hostname(), "localhost") || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if u.Scheme != "https" && !loopback {
		return "", errors.New("远程授权服务器必须使用 HTTPS")
	}
	return u.Scheme + "://" + u.Host, nil
}

func (c *licenseClient) Login(server, code string, platform Platform, installID string) (remoteAuthSession, error) {
	base, err := normalizeServerURL(server)
	if err != nil {
		return remoteAuthSession{}, err
	}
	c.Logout()
	var response struct {
		Session string `json:"session"`
		remoteAuthSession
	}
	if err := c.request(base, "/v1/auth/login", http.MethodPost, map[string]any{"code": code, "platform": platform, "client_id": installID}, "", &response); err != nil {
		return remoteAuthSession{}, err
	}
	if response.Session == "" || !validDate(response.ExpiresAt) || !validDate(response.SessionExpiresAt) || !validEntitlements(response.Features) {
		return remoteAuthSession{}, errors.New("授权服务返回的数据不完整")
	}
	c.token, c.serverURL = response.Session, base
	return response.remoteAuthSession, nil
}

func (c *licenseClient) Status(server string) (remoteAuthSession, error) {
	base, err := normalizeServerURL(server)
	if err != nil {
		return remoteAuthSession{}, err
	}
	if c.token == "" || c.serverURL != base {
		return remoteAuthSession{}, errors.New("当前没有有效的远程授权会话")
	}
	var response remoteAuthSession
	if err := c.request(base, "/v1/auth/status", http.MethodGet, nil, c.token, &response); err != nil {
		c.clear()
		return remoteAuthSession{}, err
	}
	if !validDate(response.ExpiresAt) || !validDate(response.SessionExpiresAt) || !validEntitlements(response.Features) {
		c.clear()
		return remoteAuthSession{}, errors.New("授权服务返回的数据不完整")
	}
	return response, nil
}

func (c *licenseClient) Logout() error {
	token, base := c.token, c.serverURL
	c.clear()
	if token == "" || base == "" {
		return nil
	}
	return c.request(base, "/v1/auth/logout", http.MethodPost, nil, token, nil)
}

func (c *licenseClient) SetSafeCode(server, code string) error {
	base, err := c.requireSession(server)
	if err != nil {
		return err
	}
	return c.request(base, "/v1/auth/safe-code", http.MethodPost, map[string]string{"safe_code": code}, c.token, nil)
}

func (c *licenseClient) Unbind(server, safeCode string) error {
	base, err := c.requireSession(server)
	if err != nil {
		return err
	}
	if err := c.request(base, "/v1/auth/unbind", http.MethodPost, map[string]string{"safe_code": safeCode}, c.token, nil); err != nil {
		return err
	}
	c.clear()
	return nil
}

func (c *licenseClient) requireSession(server string) (string, error) {
	base, err := normalizeServerURL(server)
	if err != nil {
		return "", err
	}
	if c.token == "" || base != c.serverURL {
		return "", errors.New("当前没有有效的远程授权会话")
	}
	return base, nil
}

func (c *licenseClient) clear()        { c.token, c.serverURL = "", "" }
func (c *licenseClient) Token() string { return c.token }

func (c *licenseClient) request(base, path, method string, body any, token string, result any) error {
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(context.Background(), method, base+path, bytes.NewReader(encoded))
	if err != nil {
		return errors.New("无法连接授权服务器，请检查地址和网络")
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.client.Do(req)
	if err != nil {
		return errors.New("无法连接授权服务器，请检查地址和网络")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return errors.New("授权服务器返回了无效响应")
	}
	var payload struct {
		OK      *bool  `json:"ok"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return errors.New("授权服务器返回了无效响应")
	}
	if result != nil {
		if err := json.Unmarshal(data, result); err != nil {
			return errors.New("授权服务器返回了无效响应")
		}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || (payload.OK != nil && !*payload.OK) {
		if payload.Message != "" {
			return errors.New(payload.Message)
		}
		return fmt.Errorf("授权服务请求失败 (%d)", response.StatusCode)
	}
	return nil
}

func validDate(value string) bool {
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

func validEntitlements(value []string) bool {
	for _, item := range value {
		if item != "overlay" && item != "slot" && item != "serial" {
			return false
		}
	}
	return value != nil
}
