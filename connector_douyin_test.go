package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// testdata/douyin-messages.json 是 2026-09-24 从抖音 HTTP 拉取线路抓到的真实样本，
// 用于回归 protobuf 字段编号与事件归一化。
type douyinFixture struct {
	WebRID   string            `json:"web_rid"`
	RoomID   string            `json:"room_id"`
	Response string            `json:"response"`
	Messages map[string]string `json:"messages"`
}

func loadDouyinFixture(t *testing.T) douyinFixture {
	t.Helper()
	data, err := os.ReadFile("testdata/douyin-messages.json")
	if err != nil {
		t.Fatalf("读取样本失败: %v", err)
	}
	var fixture douyinFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("解析样本失败: %v", err)
	}
	return fixture
}

func decodeFixture(t *testing.T, encoded string) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("样本不是合法 base64: %v", err)
	}
	return data
}

func TestDouyinParseResponseEnvelope(t *testing.T) {
	fixture := loadDouyinFixture(t)
	response, err := douyinParseResponse(decodeFixture(t, fixture.Response))
	if err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if len(response.messages) == 0 {
		t.Fatal("响应里没有消息")
	}
	if response.cursor == "" {
		t.Error("响应缺少 cursor，增量拉取会重复拉取")
	}
	if response.internalExt == "" {
		t.Error("响应缺少 internal_ext")
	}
	if response.fetchIntervalMS != 1000 {
		t.Errorf("fetch_interval 期望 1000，实际 %d", response.fetchIntervalMS)
	}
	for _, message := range response.messages {
		if message.method == "" {
			t.Fatal("存在没有 method 的消息")
		}
		if len(message.payload) == 0 {
			t.Fatalf("消息 %s 缺少 payload", message.method)
		}
	}
}

func TestDouyinEventMapping(t *testing.T) {
	fixture := loadDouyinFixture(t)
	connector := &douyinConnector{gifts: map[int64]douyinGift{2001: {ID: 2001, Name: "玫瑰", Value: 1}}}
	cases := []struct {
		method string
		kind   EventKind
	}{
		{"WebcastChatMessage", KindChat},
		{"WebcastMemberMessage", KindEnter},
		{"WebcastLikeMessage", KindLike},
		{"WebcastSocialMessage", KindFollow},
		{"WebcastRoomUserSeqMessage", KindSystem},
	}
	for _, item := range cases {
		encoded, ok := fixture.Messages[item.method]
		if !ok {
			t.Fatalf("样本缺少 %s", item.method)
		}
		event, ok := connector.eventFromMessage(douyinPushMessage{method: item.method, payload: decodeFixture(t, encoded)})
		if !ok {
			t.Fatalf("%s 未转换成事件", item.method)
		}
		if event.Kind != item.kind {
			t.Errorf("%s 期望类型 %s，实际 %s", item.method, item.kind, event.Kind)
		}
		if item.kind != KindSystem {
			if event.User == nil || event.User.Name == "" {
				t.Errorf("%s 缺少用户昵称", item.method)
			}
		}
		if event.Raw["roomId"] != fixture.RoomID {
			t.Errorf("%s 房间号不匹配: %v", item.method, event.Raw["roomId"])
		}
	}
}

func TestDouyinChatEventText(t *testing.T) {
	fixture := loadDouyinFixture(t)
	connector := &douyinConnector{}
	event, ok := connector.eventFromMessage(douyinPushMessage{
		method:  "WebcastChatMessage",
		payload: decodeFixture(t, fixture.Messages["WebcastChatMessage"]),
	})
	if !ok {
		t.Fatal("弹幕未转换成事件")
	}
	if strings.TrimSpace(event.Text) == "" {
		t.Error("弹幕内容为空")
	}
	if event.User == nil {
		t.Fatal("弹幕缺少用户信息")
	}
}

func TestDouyinGiftEventUsesGiftTable(t *testing.T) {
	connector := &douyinConnector{gifts: map[int64]douyinGift{2001: {ID: 2001, Name: "玫瑰", Icon: "https://example.com/rose.png", Value: 1}}}
	// 礼物消息：common=1、gift_id=2、repeat_count=5、user=7。
	payload := []byte{
		0x0a, 0x02, 0x08, 0x01, // common
		0x10, 0xd1, 0x0f, // gift_id = 2001
		0x28, 0x05, // repeat_count = 5
		0x3a, 0x03, 0x1a, 0x01, 0x61, // user { nickname: "a" }
	}
	event, ok := connector.eventFromMessage(douyinPushMessage{method: "WebcastGiftMessage", payload: payload})
	if !ok {
		t.Fatal("礼物未转换成事件")
	}
	if event.Kind != KindGift || event.Gift == nil {
		t.Fatalf("礼物事件结构不正确: %+v", event)
	}
	if event.Gift.Name != "玫瑰" || event.Gift.Count != 5 {
		t.Errorf("礼物信息不正确: %+v", event.Gift)
	}
	if event.Gift.Value != 1 || event.Gift.Icon == "" {
		t.Errorf("礼物价值或图标缺失: %+v", event.Gift)
	}
	if event.User == nil || event.User.Name != "a" {
		t.Errorf("礼物用户信息不正确: %+v", event.User)
	}
}

func TestDouyinGiftEventFallsBackToDetail(t *testing.T) {
	connector := &douyinConnector{gifts: map[int64]douyinGift{}}
	// gift=15 里带 name=16 的兜底路径。
	payload := []byte{
		0x10, 0xe8, 0x07, // gift_id = 1000
		0x7a, 0x06, 0x82, 0x01, 0x03, 0xe7, 0x8e, 0xab, // gift { f16: "玫瑰" }
	}
	event, ok := connector.eventFromMessage(douyinPushMessage{method: "WebcastGiftMessage", payload: payload})
	if !ok {
		t.Fatal("礼物未转换成事件")
	}
	if event.Gift == nil || event.Gift.Name == "" {
		t.Fatalf("礼物缺少名称: %+v", event.Gift)
	}
	if event.Gift.Count != 1 {
		t.Errorf("默认数量应为 1，实际 %d", event.Gift.Count)
	}
}

func TestDouyinControlMessageEndsRoom(t *testing.T) {
	// ControlMessage.status=3 表示直播已结束。
	payload := []byte{0x0a, 0x02, 0x08, 0x01, 0x10, 0x03}
	if status := douyinControlStatus(payload); status != douyinControlRoomFinished {
		t.Fatalf("直播结束状态解析失败: %d", status)
	}
}

func TestNormalizeRoomInput(t *testing.T) {
	cases := map[string]string{
		"452111294161":                                  "452111294161",
		"https://live.douyin.com/452111294161":          "452111294161",
		"https://live.douyin.com/452111294161?x=1":      "452111294161",
		"https://live.douyin.com/?web_rid=947825246438": "947825246438",
		"【直播】来我的直播间 https://live.douyin.com/452111294161 复制打开": "452111294161",
		"  452111294161  ": "452111294161",
		"":                 "",
		// 分享短链与分享码（用户直接从 App 复制的内容）。
		"85jECCpxkX4":                       "85jECCpxkX4",
		"https://v.douyin.com/85jECCpxkX4/": "85jECCpxkX4",
		"https://v.douyin.com/85jECCpxkX4":  "85jECCpxkX4",
		"7.62 复制打开抖音，看看【某某的直播】https://v.douyin.com/85jECCpxkX4/ 快来围观": "85jECCpxkX4",
		// 分享短链跳转后的地址里带 room_id。
		"https://webcast.amemv.com/douyin/webcast/reflow/7689131924585270067?iid=MS4w&utm_source=copy": "7689131924585270067",
	}
	for input, expected := range cases {
		if actual := normalizeRoomInput(input); actual != expected {
			t.Errorf("normalizeRoomInput(%q) = %q，期望 %q", input, actual, expected)
		}
	}
}

func TestDouyinShareLinkResolution(t *testing.T) {
	// 分享短链 302 的目标地址（真实抓取片段）。
	location := "https://webcast.amemv.com/douyin/webcast/reflow/7689131924585270067?iid=MS4wLjABAAAAhTEucNZboMYyFr7u&utm_source=copy&enable_replay=1"
	match := douyinShareRoomPattern.FindStringSubmatch(location)
	if len(match) != 2 || match[1] != "7689131924585270067" {
		t.Fatalf("分享链接里的 room_id 解析失败: %v", match)
	}
	if douyinShareRoomPattern.MatchString("https://www.douyin.com/video/7300000000000000000") {
		t.Error("普通视频链接不应被当成直播间")
	}
}

func TestDouyinRoomIDExtraction(t *testing.T) {
	// 直播间页面 SSR 数据里的片段（未开播时 roomId 为空）。
	live := `{"roomInfo":{"roomId":"7689119271137430281","web_rid":"452111294161","anchor":{"nickname":"拉克钓鱼"},"user_count_str":"4000+"}}`
	match := douyinRoomIDPattern.FindStringSubmatch(live)
	if len(match) != 2 || match[1] != "7689119271137430281" {
		t.Fatalf("room_id 解析失败: %v", match)
	}
	if got := douyinAnchorPattern.FindStringSubmatch(live); len(got) != 2 || got[1] != "拉克钓鱼" {
		t.Errorf("主播昵称解析失败: %v", got)
	}
	if got := douyinViewerPattern.FindStringSubmatch(live); len(got) != 2 || got[1] != "4000+" {
		t.Errorf("在线人数解析失败: %v", got)
	}
	offline := `{"roomInfo":{"roomId":"","web_rid":"123456"}}`
	if douyinRoomIDPattern.MatchString(offline) {
		t.Error("未开播的直播间不应解析出 room_id")
	}
}

func TestProtoFieldsRejectsTruncated(t *testing.T) {
	if _, err := protoFields([]byte{0x0a, 0x05, 0x01}); err == nil {
		t.Error("截断的长度前缀字段应当报错")
	}
	if _, err := protoFields([]byte{0x0a}); err == nil {
		t.Error("缺少长度的字段应当报错")
	}
}
