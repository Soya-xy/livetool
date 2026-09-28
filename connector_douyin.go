package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 抖音连接器（HTTP 拉取线路）。
//
// 原版把抖音分成主线路 / 备用线路 / 浏览器三条线，其中主线路要跑 webmssdk 的 JS 签名。
// 这里实现的是原版参数串里 a_bogus 为字面占位值的那条拉取线路：只需要 ttwid 设备
// Cookie，不需要签名，实测可直接拿到弹幕 / 礼物 / 进场 / 点赞 / 关注。
//
// 端点与参数串取自《直播平台链接器实现分析》第 2 章：
//
//	im_path=/webcast/im/fetch/
//	aid=6383&live_id=1&device_platform=web&language=zh-CN
//	&room_id=%s&internal_ext=%s&cursor=%s&resp_content_type=protobuf
//	&version_code=180800&last_rtt=0&did_rule=3&a_bogus=1
const (
	douyinUserAgent    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	douyinLiveHost     = "https://live.douyin.com"
	douyinFetchURL     = douyinLiveHost + "/webcast/im/fetch/"
	douyinGiftURL      = douyinLiveHost + "/webcast/gift/list/"
	douyinRoomURL      = douyinLiveHost + "/%s"
	douyinShortLinkURL = "https://v.douyin.com/%s/"
	douyinVersion      = "180800"
	// douyinPollIntervalMS 是兜底轮询间隔，正常以服务端返回的 fetch_interval 为准。
	douyinPollIntervalMS = 1000
	douyinRequestTimeout = 20 * time.Second
	// douyinLivenessInterval 是「回直播间页面确认还在播」的间隔。
	douyinLivenessInterval = 5 * time.Minute
	douyinGiftCacheName    = "douyin-gifts.json"
	douyinGiftCacheTTL     = 12 * time.Hour
)

var (
	// 直播间页面的 SSR 数据里带有真实 room_id 与主播信息，未开播时 room_id 为空。
	// 同一段 JSON 既可能以原样、也可能以 \" 转义的形式出现，所以反斜杠都写成可选。
	douyinRoomIDPattern = regexp.MustCompile(`"roomId\\?":\\?"(\d{15,})`)
	douyinAnchorPattern = regexp.MustCompile(`"nickname\\?":\\?"([^"\\]{1,40})`)
	douyinViewerPattern = regexp.MustCompile(`"user_count_str\\?":\\?"([^"\\]{1,20})`)
	// 分享短链 302 到 https://webcast.amemv.com/douyin/webcast/reflow/<room_id>?...
	douyinShareRoomPattern = regexp.MustCompile(`/(?:reflow|webcast/reflow)/(\d{10,})`)
)

type douyinConnector struct {
	service *AppService
	input   string
	client  *http.Client
	webRID  string
	roomID  string
	anchor  string
	viewers string

	giftMu sync.Mutex
	gifts  map[int64]douyinGift
}

type douyinGift struct {
	ID    int64   `json:"id"`
	Name  string  `json:"name"`
	Icon  string  `json:"icon"`
	Value float64 `json:"value"`
}

type douyinGiftTable struct {
	FetchedAt time.Time    `json:"fetchedAt"`
	Gifts     []douyinGift `json:"gifts"`
}

func newDouyinConnector(service *AppService, input string) *douyinConnector {
	jar, _ := cookiejar.New(nil)
	return &douyinConnector{
		service: service,
		input:   strings.TrimSpace(input),
		client:  &http.Client{Jar: jar, Timeout: douyinRequestTimeout},
		gifts:   map[int64]douyinGift{},
	}
}

// Run 解析房间、拉取礼物表，然后按服务端下发的间隔增量拉取消息。
func (c *douyinConnector) Run(ctx context.Context, sink connectorSink) error {
	// 首包（解析直播间页面）单独设超时，超时按「连接超时」上报。
	resolveCtx, cancel := context.WithTimeout(ctx, connectorConnectTimeout)
	err := c.resolveRoom(resolveCtx)
	cancel()
	if err != nil {
		return err
	}
	sink.RoomID(c.roomID)
	sink.Connected(c.describe())
	c.loadGiftTable(ctx)

	cursor, internalExt := "", ""
	interval := time.Duration(douyinPollIntervalMS) * time.Millisecond
	lastLivenessCheck := time.Now()
	for {
		if ctx.Err() != nil {
			return nil
		}
		response, err := c.fetch(ctx, cursor, internalExt)
		if err != nil {
			return err
		}
		for _, message := range response.messages {
			if message.method == "WebcastControlMessage" && douyinControlStatus(message.payload) == douyinControlRoomFinished {
				return nil
			}
			if event, ok := c.eventFromMessage(message); ok {
				sink.Emit(event)
			}
		}
		if response.cursor != "" {
			cursor = response.cursor
		}
		if response.internalExt != "" {
			internalExt = response.internalExt
		}
		if response.fetchIntervalMS > 0 {
			interval = time.Duration(response.fetchIntervalMS) * time.Millisecond
		}
		// 拉取接口对无效 room_id 也会返回空流，因此每隔一段时间回到直播间页面确认还在播。
		if time.Since(lastLivenessCheck) > douyinLivenessInterval {
			lastLivenessCheck = time.Now()
			if !c.roomStillLive(ctx) {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(interval):
		}
	}
}

// roomStillLive 重新拉一次直播间页面，确认主播还在播（roomId 为空说明已下播）。
func (c *douyinConnector) roomStillLive(ctx context.Context) bool {
	if c.webRID == "" {
		return true
	}
	response, err := c.get(ctx, fmt.Sprintf(douyinRoomURL, url.PathEscape(c.webRID)))
	if err != nil {
		return true
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return true
	}
	return douyinRoomIDPattern.Match(body)
}

// describe 组装连接成功日志：分享链接解析时拿不到主播与在线人数，只写房间号。
func (c *douyinConnector) describe() string {
	parts := []string{"抖音 room=" + c.roomID}
	if c.anchor != "" {
		parts = append(parts, "主播="+c.anchor)
	}
	if c.viewers != "" {
		parts = append(parts, "在线="+c.viewers)
	}
	return strings.Join(parts, " ")
}

// resolveRoom 把用户填写的直播间号 / 分享链接 / 分享码解析成抖音内部 room_id，并准备好设备 Cookie。
func (c *douyinConnector) resolveRoom(ctx context.Context) error {
	identifier := normalizeRoomInput(c.input)
	if identifier == "" {
		return fatalFailure(connectorStateError, "无法识别直播间号：请填写直播间号、分享链接（v.douyin.com/xxxx）或直播间链接")
	}
	// 19 位以上的纯数字本身就是 room_id。
	if isNumeric(identifier) && len(identifier) >= 15 {
		c.roomID, c.webRID = identifier, identifier
		c.ensureDeviceCookie(ctx)
		return nil
	}
	// 含字母的输入先按分享码解析：v.douyin.com/<码> 会 302 到 webcast.amemv.com/.../reflow/<room_id>。
	if !isNumeric(identifier) {
		roomID, found, err := c.resolveShareCode(ctx, identifier)
		if err != nil {
			return err
		}
		if found {
			c.roomID, c.webRID = roomID, identifier
			c.ensureDeviceCookie(ctx)
			return nil
		}
	}
	c.webRID = identifier

	page, err := c.get(ctx, fmt.Sprintf(douyinRoomURL, url.PathEscape(identifier)))
	if err != nil {
		return err
	}
	body, err := io.ReadAll(io.LimitReader(page.Body, 4<<20))
	page.Body.Close()
	if err != nil {
		return retryFailure(connectorStateError, "读取直播间页面失败："+err.Error())
	}
	html := string(body)
	if match := douyinRoomIDPattern.FindStringSubmatch(html); len(match) == 2 {
		c.roomID = match[1]
	} else if !isNumeric(identifier) {
		return fatalFailure(connectorStateError, "抖音网页版不支持用抖音号进入直播间：请改用直播间号或分享链接（v.douyin.com/xxxx）")
	} else {
		return fatalFailure(connectorStateError, "直播间不存在或当前未开播")
	}
	if match := firstDouyinMatch(douyinAnchorPattern, html); match != "" {
		c.anchor = match
	}
	if match := firstDouyinMatch(douyinViewerPattern, html); match != "" {
		c.viewers = match
	}
	c.ensureDeviceCookie(ctx)
	return nil
}

// resolveShareCode 把分享码（v.douyin.com/<码>）解析成 room_id。
// 不是短链、或短链指向的不是进行中的直播间时返回 found=false，由调用方继续按直播间号处理。
func (c *douyinConnector) resolveShareCode(ctx context.Context, code string) (string, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(douyinShortLinkURL, url.PathEscape(code)), nil)
	if err != nil {
		return "", false, retryFailure(connectorStateError, err.Error())
	}
	request.Header.Set("User-Agent", douyinUserAgent)
	request.Header.Set("Referer", douyinLiveHost+"/")
	// 分享短链靠 302 跳转带出房间号，所以这里不跟随跳转。
	client := &http.Client{
		Timeout:       douyinRequestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return "", false, retryFailure(connectorStateTimeout, "连接超时")
		}
		return "", false, retryFailure(connectorStateError, "解析分享链接失败："+err.Error())
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<16))
	location := response.Header.Get("Location")
	if match := douyinShareRoomPattern.FindStringSubmatch(location); len(match) == 2 {
		return match[1], true, nil
	}
	return "", false, nil
}

// firstDouyinMatch 取第一个有效匹配；SSR 数据里未渲染的字段会写成 $undefined。
func firstDouyinMatch(pattern *regexp.Regexp, html string) string {
	for _, match := range pattern.FindAllStringSubmatch(html, 8) {
		if len(match) == 2 && match[1] != "" && match[1] != "$undefined" {
			return match[1]
		}
	}
	return ""
}

// ensureDeviceCookie 保证请求带有 ttwid：抖音的拉取线路只认这个设备标识。
func (c *douyinConnector) ensureDeviceCookie(ctx context.Context) {
	if c.hasCookie(douyinLiveHost, "ttwid") {
		return
	}
	response, err := c.get(ctx, douyinLiveHost+"/")
	if err != nil {
		return
	}
	io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
}

func (c *douyinConnector) hasCookie(target, name string) bool {
	if c.client.Jar == nil {
		return false
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return false
	}
	for _, cookie := range c.client.Jar.Cookies(parsed) {
		if cookie.Name == name && cookie.Value != "" {
			return true
		}
	}
	return false
}

func (c *douyinConnector) get(ctx context.Context, target string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, retryFailure(connectorStateError, err.Error())
	}
	request.Header.Set("User-Agent", douyinUserAgent)
	request.Header.Set("Referer", douyinLiveHost+"/")
	request.Header.Set("Accept", "*/*")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	response, err := c.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, retryFailure(connectorStateTimeout, "连接超时")
		}
		return nil, retryFailure(connectorStateError, "请求抖音失败："+err.Error())
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, retryFailure(connectorStateError, fmt.Sprintf("抖音返回异常状态 %d", response.StatusCode))
	}
	return response, nil
}

// fetch 拉取一批增量消息。
func (c *douyinConnector) fetch(ctx context.Context, cursor, internalExt string) (douyinResponse, error) {
	query := url.Values{}
	query.Set("aid", "6383")
	query.Set("app_name", "douyin_web")
	query.Set("live_id", "1")
	query.Set("device_platform", "web")
	query.Set("language", "zh-CN")
	query.Set("enter_from", "web_live")
	query.Set("cookie_enabled", "true")
	query.Set("screen_width", "1920")
	query.Set("screen_height", "1080")
	query.Set("browser_language", "zh-CN")
	query.Set("browser_platform", "Win32")
	query.Set("browser_name", "Mozilla")
	query.Set("browser_version", douyinUserAgent)
	query.Set("im_path", "/webcast/im/fetch/")
	query.Set("identity", "audience")
	query.Set("room_id", c.roomID)
	query.Set("resp_content_type", "protobuf")
	query.Set("version_code", douyinVersion)
	query.Set("last_rtt", "0")
	query.Set("did_rule", "3")
	query.Set("a_bogus", "1")
	query.Set("cursor", cursor)
	query.Set("internal_ext", internalExt)

	response, err := c.get(ctx, douyinFetchURL+"?"+query.Encode())
	if err != nil {
		return douyinResponse{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return douyinResponse{}, retryFailure(connectorStateBreak, "读取抖音消息失败："+err.Error())
	}
	parsed, err := douyinParseResponse(body)
	if err != nil {
		return douyinResponse{}, retryFailure(connectorStateBreak, "解析抖音消息失败："+err.Error())
	}
	return parsed, nil
}

// loadGiftTable 拉取礼物表并缓存到本地：礼物消息里只有 gift_id，需要它换名称与价值。
func (c *douyinConnector) loadGiftTable(ctx context.Context) {
	if cached, ok := c.readGiftCache(); ok {
		c.setGifts(cached)
		return
	}
	query := url.Values{}
	query.Set("aid", "6383")
	query.Set("app_name", "douyin_web")
	query.Set("live_id", "1")
	query.Set("device_platform", "web")
	query.Set("language", "zh-CN")
	query.Set("enter_from", "web_live")
	query.Set("cookie_enabled", "true")
	query.Set("screen_width", "2048")
	query.Set("screen_height", "1152")
	query.Set("browser_language", "zh-CN")

	response, err := c.get(ctx, douyinGiftURL+"?"+query.Encode())
	if err != nil {
		c.log("warn", "connector", "抖音礼物表获取失败，礼物将只显示编号", err.Error())
		return
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		c.log("warn", "connector", "抖音礼物表读取失败", err.Error())
		return
	}
	gifts, err := douyinParseGiftTable(body)
	if err != nil {
		c.log("warn", "connector", "抖音礼物表解析失败", err.Error())
		return
	}
	c.setGifts(gifts)
	c.writeGiftCache(gifts)
	c.log("info", "connector", "抖音礼物表已更新", fmt.Sprintf("gifts=%d", len(gifts)))
}

// log 写入应用日志；连接器没有宿主服务时（单元测试）静默跳过。
func (c *douyinConnector) log(level, category, message, detail string) {
	if c.service != nil {
		c.service.log(level, category, message, detail)
	}
}

func (c *douyinConnector) setGifts(gifts []douyinGift) {
	c.giftMu.Lock()
	c.gifts = make(map[int64]douyinGift, len(gifts))
	icons := make(map[string]string, len(gifts))
	for _, gift := range gifts {
		c.gifts[gift.ID] = gift
		if gift.Name != "" && gift.Icon != "" {
			icons[gift.Name] = gift.Icon
		}
	}
	c.giftMu.Unlock()
	// 礼物菜单要用礼物名称换图片，这里把名称表交给主进程。
	if c.service != nil {
		c.service.setGiftIcons(icons)
	}
}

func (c *douyinConnector) gift(id int64) (douyinGift, bool) {
	c.giftMu.Lock()
	defer c.giftMu.Unlock()
	gift, ok := c.gifts[id]
	return gift, ok
}

func (c *douyinConnector) giftCachePath() string {
	if c.service == nil {
		return ""
	}
	return filepath.Join(c.service.appDir, "data", douyinGiftCacheName)
}

func (c *douyinConnector) readGiftCache() ([]douyinGift, bool) {
	path := c.giftCachePath()
	if path == "" {
		return nil, false
	}
	info, err := os.Stat(path)
	if err != nil || time.Since(info.ModTime()) > douyinGiftCacheTTL {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var table douyinGiftTable
	if err := json.Unmarshal(data, &table); err != nil || len(table.Gifts) == 0 {
		return nil, false
	}
	return table.Gifts, true
}

func (c *douyinConnector) writeGiftCache(gifts []douyinGift) {
	path := c.giftCachePath()
	if path == "" {
		return
	}
	data, err := json.Marshal(douyinGiftTable{FetchedAt: time.Now(), Gifts: gifts})
	if err != nil {
		return
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		c.log("warn", "connector", "抖音礼物表缓存写入失败", err.Error())
	}
}

func douyinParseGiftTable(data []byte) ([]douyinGift, error) {
	var payload struct {
		Data struct {
			Gifts []struct {
				ID           int64   `json:"id"`
				Name         string  `json:"name"`
				DiamondCount float64 `json:"diamond_count"`
				Image        struct {
					URLList []string `json:"url_list"`
				} `json:"image"`
			} `json:"gifts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	gifts := make([]douyinGift, 0, len(payload.Data.Gifts))
	for _, item := range payload.Data.Gifts {
		icon := ""
		if len(item.Image.URLList) > 0 {
			icon = item.Image.URLList[0]
		}
		gifts = append(gifts, douyinGift{ID: item.ID, Name: item.Name, Icon: icon, Value: item.DiamondCount})
	}
	return gifts, nil
}

// ── 消息解析 ────────────────────────────────────────────────────────────────

// douyinControlRoomFinished 是 ControlMessage.status 里表示「直播已结束」的取值。
const douyinControlRoomFinished = 3

type douyinResponse struct {
	messages        []douyinPushMessage
	cursor          string
	internalExt     string
	fetchIntervalMS int
}

type douyinPushMessage struct {
	method  string
	payload []byte
	msgID   uint64
	msgType uint64
}

// douyinParseResponse 解析 Response：messages_list=1、cursor=2、fetch_interval=3、
// internal_ext=5；每条 Message 里 method=1、payload=2、msg_id=3、msg_type=4。
func douyinParseResponse(data []byte) (douyinResponse, error) {
	fields, err := protoFields(data)
	if err != nil {
		return douyinResponse{}, err
	}
	response := douyinResponse{
		cursor:          firstString(fields, 2),
		internalExt:     firstString(fields, 5),
		fetchIntervalMS: int(firstUint(fields, 3)),
	}
	for _, raw := range allBytes(fields, 1) {
		messageFields, err := protoFields(raw)
		if err != nil {
			continue
		}
		response.messages = append(response.messages, douyinPushMessage{
			method:  firstString(messageFields, 1),
			payload: firstBytes(messageFields, 2),
			msgID:   firstUint(messageFields, 3),
			msgType: firstUint(messageFields, 4),
		})
	}
	return response, nil
}

// douyinControlStatus 读取 ControlMessage.status（字段 2）。
func douyinControlStatus(payload []byte) uint64 {
	fields, err := protoFields(payload)
	if err != nil {
		return 0
	}
	return firstUint(fields, 2)
}

// douyinEventFromMessage 把一条抖音推送消息转换成统一事件；不关心的方法返回 false。
func (c *douyinConnector) eventFromMessage(message douyinPushMessage) (LiveEvent, bool) {
	fields, err := protoFields(message.payload)
	if err != nil {
		return LiveEvent{}, false
	}
	common := firstBytes(fields, 1)
	user := douyinParseUser(firstBytes(fields, 2))

	switch message.method {
	case "WebcastChatMessage":
		text := strings.TrimSpace(firstString(fields, 3))
		if text == "" {
			return LiveEvent{}, false
		}
		return LiveEvent{Kind: KindChat, User: user, Text: text, Raw: douyinCommonRaw(common)}, true
	case "WebcastMemberMessage":
		// MyMemberMessage：3 member_count、9 enter_type、10 action、11 action_description。
		count := int(firstUint(fields, 3))
		return LiveEvent{Kind: KindEnter, User: user, Count: count, Text: firstString(fields, 11), Raw: douyinCommonRaw(common)}, true
	case "WebcastLikeMessage":
		count := int(firstUint(fields, 2))
		user = douyinParseUser(firstBytes(fields, 5))
		return LiveEvent{Kind: KindLike, User: user, Count: count, Raw: douyinCommonRaw(common)}, true
	case "WebcastSocialMessage":
		return LiveEvent{Kind: KindFollow, User: user, Count: int(firstUint(fields, 6)), Raw: douyinCommonRaw(common)}, true
	case "WebcastGiftMessage":
		return c.giftEvent(fields, common), true
	case "WebcastRoomUserSeqMessage":
		total := firstString(fields, 9)
		if total == "" {
			total = strconv.FormatUint(firstUint(fields, 3), 10)
		}
		return LiveEvent{Kind: KindSystem, Text: "在线人数 " + total, Count: int(firstUint(fields, 3)), Raw: douyinCommonRaw(common)}, true
	case "WebcastRoomStatsMessage":
		display := firstString(fields, 4)
		if display == "" {
			return LiveEvent{}, false
		}
		return LiveEvent{Kind: KindSystem, Text: display, Count: int(firstUint(fields, 5)), Raw: douyinCommonRaw(common)}, true
	default:
		return LiveEvent{}, false
	}
}

// giftEvent 解析礼物消息（MyGiftMessage）：gift_id=2、repeat_count=5、combo_count=6、
// user=7、repeat_end=9、gift=15（GiftStruct，礼物表缺失时兜底取 name=15 / describe=2）。
func (c *douyinConnector) giftEvent(fields []protoField, common []byte) LiveEvent {
	giftID := int64(firstUint(fields, 2))
	count := int(firstUint(fields, 5))
	if count == 0 {
		count = int(firstUint(fields, 6))
	}
	if count == 0 {
		count = 1
	}
	gift := LiveGift{ID: strconv.FormatInt(giftID, 10), Count: count}
	if info, ok := c.gift(giftID); ok {
		gift.Name, gift.Icon, gift.Value = info.Name, info.Icon, info.Value
	} else if detail, err := protoFields(firstBytes(fields, 15)); err == nil {
		gift.Name = firstString(detail, 15)
		if gift.Name == "" {
			gift.Name = firstString(detail, 2)
		}
	}
	if gift.Name == "" {
		gift.Name = "礼物" + gift.ID
	}
	return LiveEvent{
		Kind: KindGift,
		User: douyinParseUser(firstBytes(fields, 7)),
		Gift: &gift,
		Text: gift.Name,
		Raw:  douyinCommonRaw(common),
	}
}

// douyinParseUser 解析 User：id=1、nickname=3、avatar=9、sec_uid=73、id_str=1028。
// 未登录时抖音会把昵称与用户号打码（例如「小**」「111111」）。
func douyinParseUser(data []byte) *LiveUser {
	if len(data) == 0 {
		return nil
	}
	fields, err := protoFields(data)
	if err != nil {
		return nil
	}
	user := &LiveUser{
		ID:   firstString(fields, 1028),
		Name: strings.TrimSpace(firstString(fields, 3)),
	}
	if user.ID == "" {
		if id := firstUint(fields, 1); id != 0 {
			user.ID = strconv.FormatUint(id, 10)
		}
	}
	if user.ID == "" && user.Name == "" {
		return nil
	}
	return user
}

// douyinCommonRaw 提取 CommonMessageData 里的消息号与房间号，用于去重与诊断。
func douyinCommonRaw(common []byte) map[string]any {
	if len(common) == 0 {
		return nil
	}
	fields, err := protoFields(common)
	if err != nil {
		return nil
	}
	return map[string]any{
		"msgId":    strconv.FormatUint(firstUint(fields, 2), 10),
		"roomId":   strconv.FormatUint(firstUint(fields, 3), 10),
		"createAt": firstUint(fields, 4),
	}
}

var _ = errors.New
