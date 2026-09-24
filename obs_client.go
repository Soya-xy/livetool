package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type obsClient struct {
	mu            sync.Mutex
	writeMu       sync.Mutex
	socket        *websocket.Conn
	connected     bool
	virtualCamera bool
	url           string
	password      string
	message       string
	identified    chan error
	pending       map[string]chan obsResponse
	nextRequest   uint64
}

type OBSConnectionStatus struct {
	Connected           bool   `json:"connected"`
	URL                 string `json:"url,omitempty"`
	Message             string `json:"message"`
	VirtualCameraActive bool   `json:"virtualCameraActive,omitempty"`
}

type obsResponse struct {
	data map[string]any
	err  error
}

type obsMessage struct {
	Op int `json:"op"`
	D  struct {
		RPCVersion     int `json:"rpcVersion"`
		Authentication *struct {
			Salt      string `json:"salt"`
			Challenge string `json:"challenge"`
		} `json:"authentication"`
		RequestID     string `json:"requestId"`
		RequestStatus struct {
			Result  bool   `json:"result"`
			Comment string `json:"comment"`
		} `json:"requestStatus"`
		ResponseData map[string]any `json:"responseData"`
	} `json:"d"`
}

func newOBSClient() *obsClient {
	return &obsClient{message: "OBS 未连接", pending: map[string]chan obsResponse{}}
}

func (c *obsClient) Connect(address, password string) (OBSConnectionStatus, error) {
	c.Disconnect()
	if !strings.HasPrefix(address, "ws://") && !strings.HasPrefix(address, "wss://") {
		return c.setFailure("OBS 地址必须以 ws:// 或 wss:// 开头")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, address, nil)
	if err != nil {
		return c.setFailure("无法连接 OBS WebSocket，请检查 OBS 是否已启动和密码是否正确。")
	}
	c.mu.Lock()
	c.socket, c.url, c.password, c.connected = conn, address, password, false
	c.message = "正在验证 OBS WebSocket 身份…"
	c.identified = make(chan error, 1)
	identified := c.identified
	c.mu.Unlock()
	_ = conn.SetReadDeadline(time.Now().Add(8 * time.Second))
	var hello obsMessage
	if err := conn.ReadJSON(&hello); err != nil || hello.Op != 0 {
		conn.Close()
		if err == nil {
			err = errors.New("OBS WebSocket 未发送 Hello 消息")
		}
		return c.setFailure(err.Error())
	}
	identify := map[string]any{"rpcVersion": max(1, hello.D.RPCVersion)}
	if hello.D.Authentication != nil {
		identify["authentication"] = obsAuthentication(password, hello.D.Authentication.Salt, hello.D.Authentication.Challenge)
	}
	if err := conn.WriteJSON(map[string]any{"op": 1, "d": identify}); err != nil {
		conn.Close()
		return c.setFailure("发送 OBS 身份验证请求失败")
	}
	for {
		var message obsMessage
		if err := conn.ReadJSON(&message); err != nil {
			conn.Close()
			return c.setFailure("OBS WebSocket 身份验证失败或超时")
		}
		if message.Op == 2 {
			break
		}
		if message.Op == 0 {
			continue
		}
	}
	_ = conn.SetReadDeadline(time.Time{})
	c.mu.Lock()
	c.connected, c.message = true, "OBS WebSocket 已连接"
	c.identified = nil
	c.mu.Unlock()
	identified <- nil
	go c.readLoop(conn)
	return c.Status(), nil
}

func (c *obsClient) Command(requestType string, requestData map[string]string) (map[string]any, error) {
	c.mu.Lock()
	if !c.connected || c.socket == nil {
		c.mu.Unlock()
		return nil, errors.New("请先连接 OBS WebSocket")
	}
	c.nextRequest++
	requestID := fmt.Sprintf("livetool-%d", c.nextRequest)
	response := make(chan obsResponse, 1)
	c.pending[requestID] = response
	socket := c.socket
	c.mu.Unlock()
	data := map[string]any{}
	for key, value := range requestData {
		data[key] = value
	}
	c.writeMu.Lock()
	err := socket.WriteJSON(map[string]any{"op": 6, "d": map[string]any{"requestType": requestType, "requestId": requestID, "requestData": data}})
	c.writeMu.Unlock()
	if err != nil {
		c.removePending(requestID)
		return nil, fmt.Errorf("OBS 请求发送失败：%w", err)
	}
	timer := time.NewTimer(8 * time.Second)
	defer timer.Stop()
	select {
	case result := <-response:
		return result.data, result.err
	case <-timer.C:
		c.removePending(requestID)
		return nil, fmt.Errorf("OBS 请求 %s 超时", requestType)
	}
}

func (c *obsClient) StartVirtualCamera() (OBSConnectionStatus, error) {
	if _, err := c.Command("StartVirtualCam", nil); err != nil && !knownOBSState(err, "already running", "output is active", "already active") {
		return c.Status(), err
	}
	c.mu.Lock()
	c.virtualCamera, c.message = true, "OBS 虚拟摄像头已启动（输出当前 OBS 场景）"
	c.mu.Unlock()
	return c.Status(), nil
}

func (c *obsClient) StopVirtualCamera() (OBSConnectionStatus, error) {
	if _, err := c.Command("StopVirtualCam", nil); err != nil && !knownOBSState(err, "not running", "not active") {
		return c.Status(), err
	}
	c.mu.Lock()
	c.virtualCamera, c.message = false, "OBS 虚拟摄像头已停止"
	c.mu.Unlock()
	return c.Status(), nil
}

func (c *obsClient) Disconnect() {
	c.mu.Lock()
	conn := c.socket
	c.socket, c.connected, c.virtualCamera = nil, false, false
	c.message = "OBS WebSocket 已断开"
	for id, pending := range c.pending {
		delete(c.pending, id)
		pending <- obsResponse{err: errors.New(c.message)}
	}
	c.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

func (c *obsClient) Status() OBSConnectionStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return OBSConnectionStatus{Connected: c.connected, URL: c.url, Message: c.message, VirtualCameraActive: c.virtualCamera}
}

func (c *obsClient) setFailure(message string) (OBSConnectionStatus, error) {
	c.mu.Lock()
	c.connected, c.virtualCamera, c.message = false, false, message
	c.mu.Unlock()
	return c.Status(), errors.New(message)
}

func (c *obsClient) readLoop(conn *websocket.Conn) {
	for {
		var message obsMessage
		if err := conn.ReadJSON(&message); err != nil {
			c.mu.Lock()
			if c.socket == conn {
				c.socket, c.connected, c.virtualCamera = nil, false, false
				c.message = "OBS WebSocket 已断开"
				for id, pending := range c.pending {
					delete(c.pending, id)
					pending <- obsResponse{err: errors.New(c.message)}
				}
			}
			c.mu.Unlock()
			return
		}
		if message.Op != 7 {
			continue
		}
		c.mu.Lock()
		pending := c.pending[message.D.RequestID]
		delete(c.pending, message.D.RequestID)
		c.mu.Unlock()
		if pending == nil {
			continue
		}
		if !message.D.RequestStatus.Result {
			pending <- obsResponse{err: errors.New(message.D.RequestStatus.Comment)}
		} else {
			pending <- obsResponse{data: message.D.ResponseData}
		}
	}
}

func (c *obsClient) removePending(id string) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func obsAuthentication(password, salt, challenge string) string {
	first := sha256.Sum256([]byte(password + salt))
	secret := base64.StdEncoding.EncodeToString(first[:])
	second := sha256.Sum256([]byte(secret + challenge))
	return base64.StdEncoding.EncodeToString(second[:])
}

func knownOBSState(err error, phrases ...string) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, phrase := range phrases {
		if strings.Contains(message, phrase) {
			return true
		}
	}
	return false
}
