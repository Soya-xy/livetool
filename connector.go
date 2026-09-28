package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// 连接状态。取值与原版 webcastState 的 state 一一对应：
// connect / reConnect / success / error / break / timeout。
const (
	connectorStateDisconnected = "disconnected" // 未连接
	connectorStateConnecting   = "connecting"   // 正在连接..
	connectorStateReconnecting = "reconnecting" // 重新连接到直播间..
	connectorStateConnected    = "connected"    // 已连接到直播间
	connectorStateError        = "error"        // 连接到直播间失败
	connectorStateBreak        = "break"        // 直播间连接断开
	connectorStateTimeout      = "timeout"      // 连接超时
)

// 接入方式，写入 ConnectorStatus.Mode 并展示在设置页。
const (
	connectorModeSimulator = "simulator"
	connectorModePolling   = "polling"
	connectorModeWebSocket = "websocket"
)

const (
	// connectorConnectTimeout 是解析房间信息等首包动作的超时时间，超时按「连接超时」上报。
	connectorConnectTimeout = 15 * time.Second
	// connectorMinBackoff / connectorMaxBackoff 是重连退避区间。
	connectorMinBackoff = 2 * time.Second
	connectorMaxBackoff = 30 * time.Second
	// connectorStopTimeout 是主动断开时等待连接器退出的上限。
	connectorStopTimeout = 5 * time.Second
)

// connectorSink 是连接器向宿主投递事件与状态的回调集合。
type connectorSink struct {
	// Emit 投递一条归一化事件。
	Emit func(LiveEvent)
	// Connected 在房间解析成功、真正开始收流时调用，detail 会写入日志。
	Connected func(detail string)
	// RoomID 上报解析后的真实房间号（抖音的 web_rid 与内部 room_id 不同）。
	RoomID func(roomID string)
}

// liveConnector 是所有平台连接器的统一接口：Run 阻塞推送事件，ctx 取消后立即返回。
// 返回 nil 表示连接正常收尾（例如主播下播），返回 error 表示连接失败。
type liveConnector interface {
	Run(ctx context.Context, sink connectorSink) error
}

// connectorFailure 描述一次连接失败。fatal 为 true 时不再重连（房间不存在、未开播等）。
type connectorFailure struct {
	state   string
	message string
	fatal   bool
}

func (e *connectorFailure) Error() string { return e.message }

func fatalFailure(state, message string) error {
	return &connectorFailure{state: state, message: message, fatal: true}
}

func retryFailure(state, message string) error {
	return &connectorFailure{state: state, message: message}
}

// connectorRun 是一次连接器运行的句柄。
type connectorRun struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// startConnector 启动（替换）当前连接器，立即返回；状态通过 conn:status 推送。
func (s *AppService) startConnector(platform Platform, roomID string) {
	s.stopConnector()
	ctx, cancel := context.WithCancel(context.Background())
	run := &connectorRun{cancel: cancel, done: make(chan struct{})}
	s.connMu.Lock()
	s.connector = run
	s.connMu.Unlock()
	go func() {
		defer close(run.done)
		s.runConnector(ctx, platform, roomID)
	}()
}

// stopConnector 取消并等待当前连接器退出；没有连接器时直接返回。
func (s *AppService) stopConnector() {
	s.connMu.Lock()
	run := s.connector
	s.connector = nil
	s.connMu.Unlock()
	if run == nil {
		return
	}
	run.cancel()
	select {
	case <-run.done:
	case <-time.After(connectorStopTimeout):
	}
}

// runConnector 是连接器的重连主循环：每次失败按退避重试，致命错误直接结束。
func (s *AppService) runConnector(ctx context.Context, platform Platform, roomID string) {
	attempt := 0
	for {
		if ctx.Err() != nil {
			return
		}
		attempt++
		state := connectorStateConnecting
		if attempt > 1 {
			state = connectorStateReconnecting
		}
		s.setConnectionState(state, "")

		connector, err := s.newConnector(platform, roomID)
		if err != nil {
			s.setConnectionState(connectorStateError, err.Error())
			s.log("error", "connector", "无法连接直播间", err.Error())
			return
		}
		err = connector.Run(ctx, s.connectorSinkFor(platform, roomID))
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			// 连接器正常收尾（例如主播下播）。
			s.setConnectionState(connectorStateBreak, "直播已结束")
			s.log("warn", "connector", "直播间连接断开", "直播已结束")
			return
		}

		state, message, fatal := connectorStateError, err.Error(), false
		var failure *connectorFailure
		if errors.As(err, &failure) {
			state, message, fatal = failure.state, failure.message, failure.fatal
		}
		s.setConnectionState(state, message)
		s.log("error", "connector", "直播间连接失败", message)
		if fatal {
			return
		}
		s.bumpReconnects()
		select {
		case <-ctx.Done():
			return
		case <-time.After(connectorBackoff(attempt)):
		}
	}
}

// connectorBackoff 按尝试次数线性放大退避时间，上限 connectorMaxBackoff。
func connectorBackoff(attempt int) time.Duration {
	delay := time.Duration(attempt) * connectorMinBackoff
	if delay > connectorMaxBackoff {
		delay = connectorMaxBackoff
	}
	return delay
}

// newConnector 按平台创建连接器；未实现的平台返回可读错误。
func (s *AppService) newConnector(platform Platform, roomID string) (liveConnector, error) {
	switch platform {
	case PlatformSimulator:
		return &simulatorConnector{}, nil
	case PlatformDouyin:
		return newDouyinConnector(s, roomID), nil
	default:
		return nil, fmt.Errorf("%s 连接器尚未实现", platformLabel(platform))
	}
}

// connectorSinkFor 构造连接器回调：事件补齐来源信息后入队，状态变化即时推送前端。
func (s *AppService) connectorSinkFor(platform Platform, roomID string) connectorSink {
	return connectorSink{
		Emit: func(event LiveEvent) {
			if event.Source == "" {
				event.Source = string(platform)
			}
			if event.RoomID == "" {
				event.RoomID = roomID
			}
			if event.ID == "" {
				event.ID = randomID()
			}
			if event.Timestamp == 0 {
				event.Timestamp = nowMillis()
			}
			s.enqueueEvent(event)
		},
		Connected: func(detail string) {
			s.setConnectionState(connectorStateConnected, "")
			s.log("info", "connector", "已连接到直播间", detail)
		},
		RoomID: func(resolved string) {
			s.mu.Lock()
			s.connection.RoomID = resolved
			status := s.connection
			s.mu.Unlock()
			s.emit("conn:status", status)
		},
	}
}

func (s *AppService) setConnectionState(state, lastError string) {
	s.mu.Lock()
	s.connection.State = state
	if state == connectorStateConnected {
		s.connection.LastError = ""
	} else if lastError != "" {
		s.connection.LastError = lastError
	}
	status := s.connection
	s.mu.Unlock()
	s.emit("conn:status", status)
}

func (s *AppService) bumpReconnects() {
	s.mu.Lock()
	s.connection.Reconnects++
	status := s.connection
	s.mu.Unlock()
	s.emit("conn:status", status)
}

// simulatorConnector 是本地模拟事件源：保持连接状态，事件由「模拟事件」手动注入。
type simulatorConnector struct{}

func (c *simulatorConnector) Run(ctx context.Context, sink connectorSink) error {
	sink.Connected("本地模拟事件源")
	<-ctx.Done()
	return nil
}

// platformLabel 返回平台的中文名，用于界面与日志。
func platformLabel(platform Platform) string {
	switch platform {
	case PlatformDouyin:
		return "抖音"
	case PlatformKuaishou:
		return "快手"
	case PlatformShipinhao:
		return "视频号"
	case PlatformBilibili:
		return "B站"
	case PlatformTikTok:
		return "TIKTOK"
	case PlatformDouyu:
		return "斗鱼"
	case PlatformXiaohongshu:
		return "小红书"
	case PlatformSimulator:
		return "本地模拟器"
	default:
		return string(platform)
	}
}

// connectorMode 返回平台使用的接入方式，前端「连接器状态」里会显示。
func connectorMode(platform Platform) string {
	switch platform {
	case PlatformSimulator:
		return connectorModeSimulator
	case PlatformDouyin:
		return connectorModePolling
	default:
		return connectorModeWebSocket
	}
}

// normalizeRoomInput 从用户输入里取出直播间标识：支持纯数字直播间号、分享短链（v.douyin.com/xxxx）、
// 直播间链接与分享文本。
func normalizeRoomInput(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	if isNumeric(trimmed) {
		return trimmed
	}
	// 分享文本或链接里的短链：v.douyin.com/<码>
	if match := douyinShareLinkPattern.FindStringSubmatch(trimmed); len(match) == 2 {
		return match[1]
	}
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Host != "" {
		if value := parsed.Query().Get("web_rid"); value != "" {
			return value
		}
		segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		for index := len(segments) - 1; index >= 0; index-- {
			if digits := longestDigitRun(segments[index]); digits != "" {
				return digits
			}
		}
		// 形如 v.douyin.com/<码> 的短链：整段就是分享码。
		if strings.Contains(parsed.Host, "douyin.com") {
			for index := len(segments) - 1; index >= 0; index-- {
				if isShareCode(segments[index]) {
					return segments[index]
				}
			}
		}
		return ""
	}
	// 单独粘贴的分享码（含字母，例如 85jECCpxkX4）。
	if isShareCode(trimmed) {
		return trimmed
	}
	// 分享文本：取最长的一段连续数字（直播间号至少 6 位）。
	return longestDigitRun(trimmed)
}

// douyinShareLinkPattern 匹配分享短链里的分享码。
var douyinShareLinkPattern = regexp.MustCompile(`v\.douyin\.com/([A-Za-z0-9_-]{4,32})`)

// isShareCode 判断一段文本是不是抖音分享码：只含字母数字、带字母、长度合理。
func isShareCode(value string) bool {
	if len(value) < 4 || len(value) > 32 {
		return false
	}
	hasLetter := false
	for _, char := range value {
		switch {
		case char >= '0' && char <= '9':
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z':
			hasLetter = true
		case char == '_' || char == '-':
		default:
			return false
		}
	}
	return hasLetter
}

// longestDigitRun 返回字符串里最长的一段连续数字，长度不足 6 位时返回空串。
func longestDigitRun(input string) string {
	best, current := "", ""
	for _, char := range input {
		if char >= '0' && char <= '9' {
			current += string(char)
			if len(current) > len(best) {
				best = current
			}
			continue
		}
		current = ""
	}
	if len(best) < 6 {
		return ""
	}
	return best
}
