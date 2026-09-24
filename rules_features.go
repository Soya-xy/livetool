package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

func (s *AppService) FeatureTest(id FeatureID) OperationResult {
	if err := s.requireEntitlement(featureEntitlement(id)); err != nil {
		return OperationResult{Message: err.Error()}
	}
	config := s.settingForFeature(id)
	if !config.Enabled {
		return OperationResult{Message: "请先启用此功能"}
	}
	if id == FeatureVirtualCamera {
		settings := s.settingSnapshot()
		if !s.obs.Status().Connected {
			if _, err := s.obs.Connect(settings.OBSURL, settings.OBSPassword); err != nil {
				return OperationResult{Message: err.Error()}
			}
		}
		var status OBSConnectionStatus
		var err error
		if s.obs.Status().VirtualCameraActive {
			status, err = s.obs.StopVirtualCamera()
		} else {
			status, err = s.obs.StartVirtualCamera()
		}
		return operationFrom(status.Message, err)
	}
	kind := KindGift
	text := ""
	if id == FeatureDanmakuAssistant || id == FeatureVoiceBroadcast {
		kind, text = KindChat, "这是一条测试弹幕"
	}
	if id == FeatureDanmakuAssistant {
		keywords := splitFeatureList(featureValue(config, "keywords", "666, 欧皇"))
		if len(keywords) == 0 {
			return OperationResult{Message: "请先配置弹幕助手关键词"}
		}
		text = keywords[0]
		s.mu.Lock()
		delete(s.featureCooldown, id)
		s.mu.Unlock()
	}
	event := LiveEvent{ID: randomID(), Source: "simulator", Kind: kind, User: &LiveUser{Name: "功能测试"}, Timestamp: nowMillis()}
	if kind == KindGift {
		event.Gift = &LiveGift{Name: "测试礼物", Count: 1}
	} else {
		event.Text = text
	}
	var err error
	if id == FeatureDanmakuAssistant {
		err = s.runDanmakuAssistant(config, event)
	} else {
		err = s.executeFeature(id, config, event, "触发", nil)
	}
	if err != nil {
		s.log("warn", "feature", "功能测试执行失败", string(id)+": "+err.Error())
		return OperationResult{Message: err.Error()}
	}
	message := fmt.Sprintf("%s 已在组件窗口/绿幕窗口执行", id)
	if id == FeatureDanmakuAssistant {
		message = "已测试关键词回复：" + text
	} else if id == FeatureVoiceBroadcast {
		message = "已发送测试语音播报"
	}
	return OperationResult{OK: true, Message: message}
}

func (s *AppService) FeatureShow(id FeatureID) OperationResult {
	if err := s.requireEntitlement(featureEntitlement(id)); err != nil {
		return OperationResult{Message: err.Error()}
	}
	config := s.settingForFeature(id)
	if !config.Enabled {
		return OperationResult{Message: "请先启用此功能"}
	}
	shown, err := s.showFeaturePreview(id, config)
	if err != nil {
		return OperationResult{Message: err.Error()}
	}
	if shown {
		return OperationResult{OK: true, Message: "组件已显示在组件窗口"}
	}
	return OperationResult{OK: true, Message: "功能已启用，等待对应事件触发"}
}

func (s *AppService) FeatureIncrement(id FeatureID, key string, amount int) (int, error) {
	if err := s.requireEntitlement("slot"); err != nil {
		return 0, err
	}
	if id != FeatureWoodfish || key != "currentMerit" {
		return 0, errors.New("不允许更新此功能状态")
	}
	config := s.settingForFeature(id)
	current := featureValue(config, key, 0)
	value := max(0, current+max(0, amount))
	config.Values[key] = value
	if err := s.saveFeatureConfig(id, config); err != nil {
		return 0, err
	}
	return value, nil
}

func (s *AppService) InputRun(action Action) OperationResult {
	return s.inputRun(context.Background(), action)
}

func (s *AppService) inputRun(ctx context.Context, action Action) OperationResult {
	if !s.authorized("") {
		return OperationResult{Message: "请先验证卡密后再执行键鼠动作"}
	}
	if action.Kind != "key" && action.Kind != "mouse" {
		return OperationResult{Message: "键鼠动作类型无效"}
	}
	if len(action.Steps) == 0 {
		return OperationResult{Message: "请先配置键鼠步骤"}
	}
	if err := runSystemInput(ctx, action); err != nil {
		s.log("warn", "input-helper", "键鼠动作执行失败", err.Error())
		return OperationResult{Message: err.Error()}
	}
	s.log("info", "input-helper", action.Kind+" action sent", "steps="+fmt.Sprint(len(action.Steps)))
	return OperationResult{OK: true, Message: "键鼠动作已发送"}
}
func (s *AppService) InputFindImage(image string, threshold float64) ImageSearchResult {
	return ImageSearchResult{OK: false, Message: fmt.Sprintf("未执行系统查图：%s threshold=%.2f", filepath.Base(image), threshold)}
}
func (s *AppService) OBSConnect(address, password string) OperationResult {
	if err := s.requireEntitlement("overlay"); err != nil {
		return OperationResult{Message: err.Error()}
	}
	s.settingsMu.Lock()
	s.settings.OBSURL = address
	if password != "" && password != "***" {
		s.settings.OBSPassword = password
	}
	actualPassword := s.settings.OBSPassword
	s.settingsMu.Unlock()
	if err := s.persistSettings(); err != nil {
		return OperationResult{Message: err.Error()}
	}
	status, err := s.obs.Connect(address, actualPassword)
	if err != nil {
		s.log("warn", "obs", "连接 OBS 失败", err.Error())
		return OperationResult{Message: err.Error()}
	}
	s.log("info", "obs", "已连接 OBS", address)
	return OperationResult{OK: true, Message: status.Message}
}
func (s *AppService) OBSCommand(command string, args map[string]string) OperationResult {
	if err := s.requireEntitlement("overlay"); err != nil {
		return OperationResult{Message: err.Error()}
	}
	_, err := s.obs.Command(command, args)
	if err != nil {
		return OperationResult{Message: err.Error()}
	}
	return OperationResult{OK: true, Message: s.obs.Status().Message}
}
func (s *AppService) OBSStartVirtualCamera() OperationResult {
	if err := s.requireEntitlement("overlay"); err != nil {
		return OperationResult{Message: err.Error()}
	}
	if !s.obs.Status().Connected {
		settings := s.settingSnapshot()
		if _, err := s.obs.Connect(settings.OBSURL, settings.OBSPassword); err != nil {
			return OperationResult{Message: err.Error()}
		}
	}
	status, err := s.obs.StartVirtualCamera()
	return operationFrom(status.Message, err)
}
func (s *AppService) OBSStopVirtualCamera() OperationResult {
	if err := s.requireEntitlement("overlay"); err != nil {
		return OperationResult{Message: err.Error()}
	}
	status, err := s.obs.StopVirtualCamera()
	return operationFrom(status.Message, err)
}
func (s *AppService) OBSStatus() OBSConnectionStatus { return s.obs.Status() }

func (s *AppService) processEvent(ctx context.Context, event LiveEvent) {
	record := recordFromEvent(event, s.settingSnapshot().StoreRaw)
	id, err := s.store.InsertRecord(context.Background(), record)
	if err != nil {
		s.log("error", "danmaku", "写入弹幕记录失败", err.Error())
		return
	}
	record.ID = id
	s.emit("danmaku:append", record)
	s.log("info", event.Source, fmt.Sprintf("%s user=%s text=%s", event.Kind, userName(event), eventText(event)), "")
	rules, err := s.store.ListRules(context.Background())
	if err != nil {
		s.log("error", "rules", "读取规则失败", err.Error())
		return
	}
	outcome := s.runRules(ctx, event, rules)
	if ctx.Err() == nil {
		s.runFeatureRules(event)
	} else if outcome.Result == "none" {
		outcome = RuleOutcome{Result: "failed", Reason: "应用关闭时事件处理已取消"}
	}
	if err := s.store.UpdateRecordResult(context.Background(), id, outcome); err != nil {
		s.log("error", "danmaku", "更新弹幕处理结果失败", err.Error())
	}
	record.MatchedRuleID, record.MatchedRuleName = outcome.RuleID, outcome.RuleName
	record.ActionResult, record.FailReason = outcome.Result, outcome.Reason
	s.emit("danmaku:append", record)
}

func (s *AppService) runRules(ctx context.Context, event LiveEvent, rules []Rule) RuleOutcome {
	candidates := make([]Rule, 0, len(rules))
	for _, rule := range rules {
		if matchRule(rule, event) {
			candidates = append(candidates, rule)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Priority == candidates[j].Priority {
			return candidates[i].UpdatedAt < candidates[j].UpdatedAt
		}
		return candidates[i].Priority > candidates[j].Priority
	})
	for _, rule := range candidates {
		if ctx.Err() != nil {
			return RuleOutcome{Result: "failed", Reason: "应用关闭时事件处理已取消"}
		}
		var queueLock *sync.Mutex
		if rule.Concurrency == "queue" {
			queueLock = s.ruleMutex(rule.ID)
			queueLock.Lock()
		}
		if reason := s.startRule(rule); reason != "" {
			if queueLock != nil {
				queueLock.Unlock()
			}
			s.log("debug", "rule", fmt.Sprintf("[rule:%s] skipped: %s", rule.Name, reason), "")
			continue
		}
		s.log("info", "rule", fmt.Sprintf("[rule:%s] matched priority=%d", rule.Name, rule.Priority), "")
		outcome := RuleOutcome{RuleID: rule.ID, RuleName: rule.Name, Result: "ok"}
		for _, action := range rule.Actions {
			repeats := max(1, action.Repeat)
			for i := 0; i < repeats; i++ {
				if !waitForContext(ctx, time.Duration(action.DelayMS)*time.Millisecond) {
					outcome.Result, outcome.Reason = "failed", "应用关闭时事件处理已取消"
					break
				}
				result := s.executeAction(ctx, action, event, rule)
				if !result.OK {
					outcome.Result, outcome.Reason = "failed", result.Message
					s.log("warn", "rule", fmt.Sprintf("[rule:%s] action failed", rule.Name), result.Message)
					break
				}
				if ctx.Err() != nil {
					outcome.Result, outcome.Reason = "failed", "应用关闭时事件处理已取消"
					break
				}
			}
			if outcome.Result == "failed" {
				break
			}
		}
		s.finishRule(rule.ID)
		if queueLock != nil {
			queueLock.Unlock()
		}
		return outcome
	}
	if len(candidates) > 0 {
		return RuleOutcome{Result: "none", Reason: "所有匹配规则均被跳过"}
	}
	return RuleOutcome{Result: "none"}
}

func (s *AppService) startRule(rule Rule) string {
	s.ruleStateMu.Lock()
	defer s.ruleStateMu.Unlock()
	now := nowMillis()
	if now < s.ruleNextAllowed[rule.ID] {
		return fmt.Sprintf("冷却中，还需 %dms", s.ruleNextAllowed[rule.ID]-now)
	}
	if rule.Probability != nil && rand.Float64() > *rule.Probability {
		return "概率未命中"
	}
	if rule.Concurrency == "exclusive" && s.ruleActive[rule.ID] > 0 {
		return "互斥动作正在执行"
	}
	if rule.CooldownMS > 0 {
		s.ruleNextAllowed[rule.ID] = now + int64(rule.CooldownMS)
	}
	s.ruleActive[rule.ID]++
	return ""
}

func (s *AppService) finishRule(id string) {
	s.ruleStateMu.Lock()
	s.ruleActive[id] = max(0, s.ruleActive[id]-1)
	s.ruleStateMu.Unlock()
}

func (s *AppService) ruleMutex(id string) *sync.Mutex {
	s.ruleStateMu.Lock()
	defer s.ruleStateMu.Unlock()
	if s.ruleLocks[id] == nil {
		s.ruleLocks[id] = &sync.Mutex{}
	}
	return s.ruleLocks[id]
}

func matchRule(rule Rule, event LiveEvent) bool {
	if !rule.Enabled || !containsKind(rule.Trigger.Kinds, event.Kind) {
		return false
	}
	if len(rule.Trigger.Source) > 0 && !containsString(rule.Trigger.Source, event.Source) {
		return false
	}
	name := userName(event)
	if len(rule.Trigger.Users) > 0 && !containsString(rule.Trigger.Users, name) {
		return false
	}
	count := event.Count
	if event.Gift != nil && event.Gift.Count > 0 {
		count = event.Gift.Count
	}
	if rule.Trigger.MinCount > 0 && count < rule.Trigger.MinCount {
		return false
	}
	if len(rule.Trigger.GiftNames) > 0 && (event.Gift == nil || !containsString(rule.Trigger.GiftNames, event.Gift.Name)) {
		return false
	}
	keywords := make([]string, 0, len(rule.Trigger.Keywords))
	for _, item := range rule.Trigger.Keywords {
		if item != "" {
			keywords = append(keywords, item)
		}
	}
	if len(keywords) == 0 {
		return true
	}
	content := eventText(event)
	for _, keyword := range keywords {
		switch rule.Trigger.KeywordMode {
		case "exact":
			if content == keyword {
				return true
			}
		case "regex":
			if re, err := regexp.Compile("(?i)" + keyword); err == nil && re.MatchString(content) {
				return true
			}
		default:
			if strings.Contains(strings.ToLower(content), strings.ToLower(keyword)) {
				return true
			}
		}
	}
	return false
}

func (s *AppService) runFeatureRules(event LiveEvent) {
	settings := s.settingSnapshot()
	for _, id := range featureIDs() {
		config := settings.Features[id]
		if !config.Enabled || !s.authorized(featureEntitlement(id)) {
			continue
		}
		if event.Kind == KindGift && event.Gift != nil {
			giftName := strings.ToLower(strings.TrimSpace(event.Gift.Name))
			var selected *FeatureGiftRule
			for index := range config.GiftRules {
				rule := &config.GiftRules[index]
				if !rule.Enabled {
					continue
				}
				if strings.TrimSpace(rule.GiftName) != "*" && strings.ToLower(strings.TrimSpace(rule.GiftName)) == giftName {
					selected = rule
					break
				}
			}
			if selected == nil {
				for index := range config.GiftRules {
					rule := &config.GiftRules[index]
					if rule.Enabled && strings.TrimSpace(rule.GiftName) == "*" {
						selected = rule
						break
					}
				}
			}
			if selected != nil {
				if err := s.executeFeature(id, config, event, selected.Action, selected.Values); err != nil {
					s.log("warn", "feature", fmt.Sprintf("[feature:%s] 执行失败", id), err.Error())
				} else {
					s.log("info", "feature", fmt.Sprintf("[feature:%s] gift=%s action=%s", id, event.Gift.Name, selected.Action), "")
				}
			}
		}
		if event.Kind == KindChat && strings.TrimSpace(event.Text) != "" {
			if id == FeatureDanmakuAssistant {
				if err := s.runDanmakuAssistant(config, event); err != nil {
					s.log("warn", "feature", "弹幕助手执行失败", err.Error())
				}
			}
			if id == FeatureVoiceBroadcast && featureValue(config, "chatEnabled", true) {
				if err := s.executeFeature(id, config, event, "触发", nil); err != nil {
					s.log("warn", "feature", "语音播报执行失败", err.Error())
				}
			}
		}
	}
}

func (s *AppService) runDanmakuAssistant(config FeatureConfig, event LiveEvent) error {
	text := strings.TrimSpace(event.Text)
	mode := featureValue(config, "keywordMode", "contains")
	matched := false
	for _, keyword := range splitFeatureList(featureValue(config, "keywords", "666, 欧皇")) {
		if mode == "exact" {
			matched = strings.EqualFold(text, keyword)
		} else {
			matched = strings.Contains(strings.ToLower(text), strings.ToLower(keyword))
		}
		if matched {
			break
		}
	}
	if !matched {
		return nil
	}
	now := nowMillis()
	cooldown := featureValue(config, "cooldownMs", 3000)
	s.mu.Lock()
	if now-s.featureCooldown[FeatureDanmakuAssistant] < int64(cooldown) {
		s.mu.Unlock()
		return nil
	}
	s.featureCooldown[FeatureDanmakuAssistant] = now
	s.mu.Unlock()
	return s.executeFeature(FeatureDanmakuAssistant, config, event, "触发", nil)
}

func (s *AppService) executeAction(ctx context.Context, action Action, event LiveEvent, rule Rule) OperationResult {
	switch action.Kind {
	case "video":
		return operationFrom("", s.OverlayPlayVideo(OverlayVideoPayload{Path: action.Path, Lane: action.Lane, DurationMS: action.DurationMS, Loop: action.Loop, Chroma: action.Chroma}))
	case "drop":
		return operationFrom("", s.OverlayDrop(map[string]any{"image": action.Image, "count": action.Count, "gravity": action.Gravity, "bounce": action.Bounce, "durationMs": action.DurationMS, "maxVisible": action.MaxVisible}))
	case "slot":
		return operationFrom("", s.OverlaySlot(map[string]any{"theme": action.Theme, "pool": action.Pool, "weights": action.Weights}))
	case "audio":
		return operationFrom("", s.AudioPlay(map[string]any{"path": action.Path, "volume": action.Volume, "interrupt": action.Interrupt, "loop": action.Loop}))
	case "key", "mouse":
		return s.inputRun(ctx, action)
	case "serial":
		return s.SerialPulse(action)
	case "obs":
		return s.OBSCommand(action.Command, action.Args)
	default:
		return OperationResult{Message: "不支持的动作类型：" + action.Kind}
	}
}

func recordFromEvent(event LiveEvent, includeRaw bool) DanmakuRecord {
	record := DanmakuRecord{Source: event.Source, RoomID: event.RoomID, Kind: event.Kind, Text: event.Text, TS: event.Timestamp, CreatedAt: nowMillis(), ActionResult: "none"}
	if event.User != nil {
		record.UserID, record.UserName = event.User.ID, event.User.Name
	}
	if event.Gift != nil {
		record.GiftName, record.GiftCount, record.GiftValue = event.Gift.Name, event.Gift.Count, event.Gift.Value
	}
	if event.Kind == KindLike {
		record.RepeatCount = event.Count
	}
	if includeRaw && event.Raw != nil {
		if data, err := json.Marshal(event.Raw); err == nil {
			record.RawJSON = string(data)
		}
	}
	return record
}

func (s *AppService) executeFeature(id FeatureID, config FeatureConfig, event LiveEvent, ruleAction string, ruleValues map[string]any) error {
	if err := s.requireEntitlement(featureEntitlement(id)); err != nil {
		return err
	}
	active := cloneFeatureConfig(config)
	if ruleValues != nil {
		for key, value := range ruleValues {
			active.Values[key] = value
		}
	}
	persist := func(key string, value any) { config.Values[key], active.Values[key] = value, value }
	save := func() error { return s.saveFeatureConfig(id, config) }
	widget := func(payload OverlayWidgetPayload) error { return s.OverlayWidget(payload) }
	giftCount := 1
	if event.Gift != nil && event.Gift.Count > 0 {
		giftCount = min(100, event.Gift.Count)
	}
	value := func(key string, fallback any) any { return featureAny(active, key, fallback) }
	switch id {
	case FeatureGreenWindow:
		settings := s.overlaySettings("green")
		settings.Background, _ = value("backgroundColor", "#00ff00").(string)
		settings.LaneCount, _ = numeric(value("laneCount", 3)), true
		settings.AlwaysOnTop, _ = value("alwaysOnTop", true).(bool)
		if _, err := s.OverlayUpdateSettings("green", map[string]any{"background": settings.Background, "laneCount": settings.LaneCount, "alwaysOnTop": settings.AlwaysOnTop}); err != nil {
			return err
		}
		if _, err := s.OverlayOpen("green"); err != nil {
			return err
		}
		path, _ := value("videoPath", "").(string)
		if strings.TrimSpace(path) != "" {
			duration := numeric(value("videoDurationMs", 8000))
			loop, _ := value("videoLoop", false).(bool)
			return s.OverlayPlayVideo(OverlayVideoPayload{Path: path, DurationMS: duration, Loop: loop})
		}
	case FeatureComponentWindow:
		if _, err := s.OverlayUpdateSettings("slot", map[string]any{"width": numeric(value("width", 960)), "height": numeric(value("height", 540)), "alwaysOnTop": value("alwaysOnTop", true)}); err != nil {
			return err
		}
		_, err := s.OverlayOpen("slot")
		return err
	case FeatureVirtualCamera:
		result := s.OBSStartVirtualCamera()
		if !result.OK {
			return errors.New(result.Message)
		}
	case FeatureSpeedCurve, FeatureSpeedIba:
		kind, title := "acceleration", "加速度 · 曲线队列"
		if id == FeatureSpeedIba {
			title = "加速度 · IB 批处理"
		}
		count := min(9999, numeric(value("targetCount", 10))*giftCount)
		return widget(OverlayWidgetPayload{FeatureID: id, Kind: kind, Title: title, Values: active.Values, Data: map[string]any{"targetCount": count, "giftCount": giftCount}})
	case FeatureImpactGift:
		count := min(100, numeric(value("count", 8))*giftCount)
		path, _ := value("imagePath", "images/平底锅.png").(string)
		if err := s.OverlayDrop(map[string]any{"image": path, "count": count, "gravity": value("gravity", 1800), "bounce": value("bounce", 0.45), "durationMs": 6000}); err != nil {
			return err
		}
		return widget(OverlayWidgetPayload{FeatureID: id, Kind: "notice", Title: "砸礼物", Values: active.Values, Data: map[string]any{"text": fmt.Sprintf("%s × %d · 已发送到绿幕窗口", giftName(event), count)}})
	case FeatureFryingPan:
		maxValue := numeric(value("maxValue", 100))
		current := featureValue(config, "currentValue", maxValue)
		change := numeric(value("damagePerGift", 10)) * giftCount
		if ruleAction == "减少" {
			change = -change
		}
		current = int(math.Max(0, math.Min(float64(maxValue), float64(current+change))))
		persist("currentValue", current)
		if err := save(); err != nil {
			return err
		}
		return widget(OverlayWidgetPayload{FeatureID: id, Kind: "health", Title: featureValue(active, "displayContent", "煮播血条"), Values: active.Values, Data: map[string]any{"value": current, "maxValue": maxValue}})
	case FeatureVoiceBroadcast:
		field := "template"
		if event.Kind == KindChat {
			field = "chatTemplate"
		}
		template := featureValue(active, field, "{user} 送出 {gift}，数量 {count}")
		text := formatFeatureTemplate(template, event)
		return widget(OverlayWidgetPayload{FeatureID: id, Kind: "speech", Title: "语音播报", Values: active.Values, Data: map[string]any{"text": text, "volume": value("volume", 0.8), "interrupt": value("interrupt", false)}})
	case FeatureDanmakuAssistant:
		text := formatFeatureTemplate(featureValue(active, "replyTemplate", "收到 {user}"), event)
		if err := widget(OverlayWidgetPayload{FeatureID: id, Kind: "reply", Title: "弹幕助手 · 本地回复预览", Values: active.Values, Data: map[string]any{"text": text, "user": userName(event)}}); err != nil {
			return err
		}
		s.log("info", "feature", "本地弹幕回复预览", text)
	case FeatureLiveClock:
		action := "start"
		if event.Kind == KindGift && ruleAction == "增加" {
			action = "add"
		} else if event.Kind == KindGift && ruleAction == "减少" {
			action = "subtract"
		}
		return widget(OverlayWidgetPayload{FeatureID: id, Kind: "timer", Title: featureValue(active, "displayContent", "直播倒计时"), Values: active.Values, Data: map[string]any{"action": action, "seconds": value("durationSeconds", 60), "deltaSeconds": numeric(value("secondsPerGift", 10)) * giftCount}})
	case FeatureWoodfish:
		count := featureValue(config, "currentMerit", 0)
		if event.Kind == KindGift {
			count += numeric(value("meritPerGift", featureValue(active, "meritPerClick", 1))) * giftCount
			persist("currentMerit", count)
			if err := save(); err != nil {
				return err
			}
		}
		return widget(OverlayWidgetPayload{FeatureID: id, Kind: "woodfish", Title: "电子木鱼", Values: active.Values, Data: map[string]any{"count": count}})
	case FeatureSlotMachine:
		pool := splitFeatureList(featureValue(active, "pool", "一等奖, 二等奖, 谢谢参与"))
		weights := parseWeights(featureValue(active, "weights", "1, 10, 89"), len(pool))
		images := make([]string, 14)
		for i := range images {
			images[i] = fmt.Sprintf("水果机/%d.png", i+1)
		}
		payload := map[string]any{"theme": value("theme", "default"), "pool": pool, "weights": weights, "images": images, "durationMs": value("durationMs", 2200)}
		if featureValue(active, "playMusic", true) {
			payload["audioPath"] = "fruit.mp3"
		}
		return s.OverlaySlot(payload)
	case FeatureGiftScreen:
		path, _ := value("imagePath", "images/平底锅.png").(string)
		if err := s.OverlayDrop(map[string]any{"image": path, "count": giftCount, "durationMs": value("durationMs", 3500), "gravity": 500, "bounce": 0.2, "maxVisible": value("maxVisible", 20)}); err != nil {
			return err
		}
		return widget(OverlayWidgetPayload{FeatureID: id, Kind: "notice", Title: "礼物飘屏", Values: active.Values, Data: map[string]any{"text": fmt.Sprintf("%s × %d · 已发送到绿幕窗口", giftName(event), giftCount)}})
	case FeatureLottery:
		return widget(OverlayWidgetPayload{FeatureID: id, Kind: "wheel", Title: "大转盘", Values: active.Values})
	case FeatureCounter:
		current := featureValue(config, "currentValue", featureValue(active, "initialValue", 0))
		delta := numeric(value("step", 1)) * giftCount
		if ruleAction == "减少" {
			delta = -delta
		}
		current = max(0, current+delta)
		persist("currentValue", current)
		if err := save(); err != nil {
			return err
		}
		return widget(OverlayWidgetPayload{FeatureID: id, Kind: "counter", Title: featureValue(active, "displayContent", "礼物计数"), Values: active.Values, Data: map[string]any{"value": current}})
	case FeatureGiftPool:
		pool := splitFeatureList(featureValue(active, "pool", "啤酒, 小心心, 平底锅"))
		if len(pool) == 0 {
			return errors.New("礼物贴纸池为空")
		}
		index := rand.Intn(len(pool))
		if !featureValue(active, "randomize", true) {
			index = featureValue(config, "nextStickerIndex", 0) % len(pool)
			persist("nextStickerIndex", (index+1)%len(pool))
			if err := save(); err != nil {
				return err
			}
		}
		sticker := pool[index]
		data := map[string]any{"sticker": sticker, "durationMs": value("durationMs", 3500)}
		if isImagePath(sticker) {
			data["sticker"] = filepath.Base(sticker)
			path, err := s.resolveAssetPath(sticker)
			if err != nil {
				return err
			}
			if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("找不到礼物贴纸图片：%s", filepath.Base(sticker))
			}
			mediaPath, err := s.mediaURL(path)
			if err != nil {
				return err
			}
			data["imagePath"] = mediaPath
		}
		return widget(OverlayWidgetPayload{FeatureID: id, Kind: "sticker", Title: "礼物咖", Values: active.Values, Data: data})
	case FeatureScreenLock:
		return widget(OverlayWidgetPayload{FeatureID: id, Kind: "lock", Title: "屏幕锁键", Values: active.Values, Data: map[string]any{"locked": true}})
	case FeatureMosquitoSlap:
		return widget(OverlayWidgetPayload{FeatureID: id, Kind: "mosquito", Title: "拍蚊子", Values: active.Values, Data: map[string]any{"durationMs": value("durationMs", 20000), "score": 0}})
	default:
		return fmt.Errorf("不支持的扩展功能：%s", id)
	}
	return nil
}

func (s *AppService) showFeaturePreview(id FeatureID, config FeatureConfig) (bool, error) {
	if s.hasEventWidget(id) {
		_, err := s.OverlayOpen("slot")
		return true, err
	}
	values := config.Values
	var payload OverlayWidgetPayload
	switch id {
	case FeatureSpeedCurve, FeatureSpeedIba:
		title := "加速度 · 曲线队列"
		if id == FeatureSpeedIba {
			title = "加速度 · IB 批处理"
		}
		payload = OverlayWidgetPayload{FeatureID: id, Kind: "acceleration", Title: title, Values: values, Data: map[string]any{"preview": true}}
	case FeatureFryingPan:
		maximum := featureValue(config, "maxValue", 100)
		payload = OverlayWidgetPayload{FeatureID: id, Kind: "health", Title: featureValue(config, "displayContent", "煮播血条"), Values: values, Data: map[string]any{"preview": true, "value": featureValue(config, "currentValue", maximum), "maxValue": maximum}}
	case FeatureImpactGift:
		payload = OverlayWidgetPayload{FeatureID: id, Kind: "notice", Title: "砸礼物", Values: values, Data: map[string]any{"preview": true, "text": "组件已启用 · 礼物效果将在绿幕窗口播放"}}
	case FeatureDanmakuAssistant:
		payload = OverlayWidgetPayload{FeatureID: id, Kind: "reply", Title: "弹幕助手", Values: values, Data: map[string]any{"preview": true, "text": featureValue(config, "replyTemplate", "收到 {user}")}}
	case FeatureLiveClock:
		payload = OverlayWidgetPayload{FeatureID: id, Kind: "timer", Title: featureValue(config, "displayContent", "直播倒计时"), Values: values, Data: map[string]any{"preview": true, "seconds": featureValue(config, "durationSeconds", 60)}}
	case FeatureWoodfish:
		payload = OverlayWidgetPayload{FeatureID: id, Kind: "woodfish", Title: "电子木鱼", Values: values, Data: map[string]any{"preview": true, "count": featureValue(config, "currentMerit", 0)}}
	case FeatureLottery:
		payload = OverlayWidgetPayload{FeatureID: id, Kind: "wheel", Title: "大转盘", Values: values, Data: map[string]any{"preview": true}}
	case FeatureCounter:
		payload = OverlayWidgetPayload{FeatureID: id, Kind: "counter", Title: featureValue(config, "displayContent", "礼物计数"), Values: values, Data: map[string]any{"preview": true, "value": featureValue(config, "currentValue", featureValue(config, "initialValue", 0))}}
	case FeatureGiftScreen:
		payload = OverlayWidgetPayload{FeatureID: id, Kind: "notice", Title: "礼物飘屏", Values: values, Data: map[string]any{"preview": true, "text": "组件已启用 · 礼物素材将在绿幕窗口飘屏"}}
	case FeatureGiftPool:
		pool := splitFeatureList(featureValue(config, "pool", "啤酒, 小心心, 平底锅"))
		sticker := "等待礼物"
		if len(pool) > 0 {
			sticker = pool[0]
		}
		data := map[string]any{"preview": true, "sticker": sticker}
		if isImagePath(sticker) {
			path, err := s.resolveAssetPath(sticker)
			if err != nil {
				return false, err
			}
			if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
				return false, fmt.Errorf("找不到礼物贴纸图片：%s", filepath.Base(sticker))
			}
			mediaPath, err := s.mediaURL(path)
			if err != nil {
				return false, err
			}
			data["sticker"], data["imagePath"] = filepath.Base(sticker), mediaPath
		}
		payload = OverlayWidgetPayload{FeatureID: id, Kind: "sticker", Title: "礼物咖", Values: values, Data: data}
	case FeatureScreenLock:
		payload = OverlayWidgetPayload{FeatureID: id, Kind: "lock", Title: "屏幕锁键", Values: values, Data: map[string]any{"preview": true, "locked": false}}
	case FeatureMosquitoSlap:
		payload = OverlayWidgetPayload{FeatureID: id, Kind: "mosquito", Title: "拍蚊子", Values: values, Data: map[string]any{"preview": true, "durationMs": featureValue(config, "durationMs", 20000), "score": 0}}
	default:
		return false, nil
	}
	return true, s.OverlayWidget(payload)
}

func (s *AppService) hasEventWidget(id FeatureID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	widget, ok := s.slotWidgets[id]
	return ok && widget.Data["preview"] != true
}

func (s *AppService) requireEntitlement(entitlement string) error {
	if !s.authorized(entitlement) {
		return fmt.Errorf("当前卡密未包含 %s 权限，或授权会话已过期", entitlement)
	}
	return nil
}

func (s *AppService) debugRuleByIndex(index int) {
	if !localDevelopmentModeAllowed() || !s.settingSnapshot().DevMode {
		return
	}
	rules, err := s.RulesList()
	if err != nil || index < 0 || index >= len(rules) {
		return
	}
	rule := rules[index]
	kind := KindGift
	if len(rule.Trigger.Kinds) > 0 {
		kind = rule.Trigger.Kinds[0]
	}
	event := LiveEvent{ID: randomID(), Source: "simulator", Kind: kind, User: &LiveUser{Name: "快捷键调试"}, Timestamp: nowMillis()}
	if kind == KindGift {
		name := "啤酒"
		if len(rule.Trigger.GiftNames) > 0 {
			name = rule.Trigger.GiftNames[0]
		} else if len(rule.Trigger.Keywords) > 0 {
			name = rule.Trigger.Keywords[0]
		}
		event.Gift = &LiveGift{Name: name, Count: max(1, rule.Trigger.MinCount)}
	} else if len(rule.Trigger.Keywords) > 0 {
		event.Text = rule.Trigger.Keywords[0]
	} else {
		event.Text = "调试弹幕"
	}
	s.enqueueEvent(event)
	s.log("info", "hotkey", "已发送规则调试事件", rule.Name)
}

func operationFrom(message string, err error) OperationResult {
	if err != nil {
		return OperationResult{Message: err.Error()}
	}
	return OperationResult{OK: true, Message: message}
}
func eventText(event LiveEvent) string {
	if event.Kind == KindGift && event.Gift != nil {
		return event.Gift.Name
	}
	if event.Kind == KindChat {
		return event.Text
	}
	if event.Text != "" {
		return event.Text
	}
	if event.Gift != nil {
		return event.Gift.Name
	}
	return ""
}
func userName(event LiveEvent) string {
	if event.User != nil && event.User.Name != "" {
		return event.User.Name
	}
	return "匿名用户"
}
func giftName(event LiveEvent) string {
	if event.Gift != nil && event.Gift.Name != "" {
		return event.Gift.Name
	}
	return "礼物"
}
func containsKind(items []EventKind, value EventKind) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
func numeric(value any) int {
	switch number := value.(type) {
	case int:
		return number
	case int64:
		return int(number)
	case float64:
		return int(number)
	case float32:
		return int(number)
	default:
		return 0
	}
}
func featureAny(config FeatureConfig, key string, fallback any) any {
	if value, ok := config.Values[key]; ok {
		return value
	}
	return fallback
}
func cloneFeatureConfig(config FeatureConfig) FeatureConfig {
	cloned := FeatureConfig{Enabled: config.Enabled, Values: cloneAnyMap(config.Values), GiftRules: make([]FeatureGiftRule, len(config.GiftRules))}
	for i, rule := range config.GiftRules {
		cloned.GiftRules[i] = rule
		cloned.GiftRules[i].Values = cloneAnyMap(rule.Values)
	}
	return cloned
}
func cloneAnyMap(input map[string]any) map[string]any {
	output := map[string]any{}
	for key, value := range input {
		output[key] = value
	}
	return output
}
func parseWeights(value string, length int) []int {
	parsed := make([]int, 0)
	for _, item := range splitFeatureList(value) {
		number := numericString(item)
		if number < 0 {
			number = 0
		}
		parsed = append(parsed, number)
	}
	weights := make([]int, length)
	total := 0
	for i := range weights {
		weights[i] = 1
		if i < len(parsed) {
			weights[i] = parsed[i]
		}
		total += weights[i]
	}
	if total == 0 {
		for i := range weights {
			weights[i] = 1
		}
	}
	return weights
}
func numericString(value string) int {
	var number float64
	_, _ = fmt.Sscan(value, &number)
	return int(number)
}
func isImagePath(value string) bool {
	ext := strings.ToLower(filepath.Ext(value))
	return ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".webp" || ext == ".svg"
}
