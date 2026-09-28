package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 联网回归：只有显式设置 LIVETOOL_LIVE_ROOM 时才跑，CI 与本地默认跳过。
//
//	LIVETOOL_LIVE_ROOM=452111294161 go test -run 'Live' -v .
func TestDouyinConnectorLive(t *testing.T) {
	room := os.Getenv("LIVETOOL_LIVE_ROOM")
	if room == "" {
		t.Skip("未设置 LIVETOOL_LIVE_ROOM，跳过联网测试")
	}
	connector := newDouyinConnector(nil, room)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	events := make(chan LiveEvent, 64)
	connected := make(chan string, 1)
	sink := connectorSink{
		Emit: func(event LiveEvent) {
			select {
			case events <- event:
			default:
			}
		},
		Connected: func(detail string) {
			select {
			case connected <- detail:
			default:
			}
		},
		RoomID: func(string) {},
	}
	go func() {
		if err := connector.Run(ctx, sink); err != nil {
			t.Logf("连接器退出：%v", err)
		}
	}()

	select {
	case detail := <-connected:
		t.Logf("已连接：%s", detail)
	case <-ctx.Done():
		t.Fatal("90 秒内没有连上直播间")
	}
	if connector.roomID == "" {
		t.Fatal("没有解析出 room_id")
	}
	if connector.anchor == "" {
		t.Log("注意：没有解析到主播昵称")
	}
	deadline := time.After(45 * time.Second)
	kinds := map[EventKind]int{}
	for {
		select {
		case event := <-events:
			kinds[event.Kind]++
			// 安静的小房间可能只有系统消息，因此只要求「收到过事件」。
			if len(kinds) >= 1 {
				t.Logf("收到事件类型：%v", kinds)
				if count := len(connector.gifts); count == 0 {
					t.Error("礼物表为空，礼物消息只能显示编号")
				} else {
					t.Logf("礼物表条目：%d", count)
				}
				return
			}
		case <-deadline:
			t.Fatalf("45 秒内没有收到任何事件：%v", kinds)
		}
	}
}

// TestAppServiceConnectLive 覆盖完整链路：ConnConnect → 连接器 → 事件队列 → 弹幕日记。
// 同样只在设置 LIVETOOL_LIVE_ROOM 时运行。
func TestAppServiceConnectLive(t *testing.T) {
	room := os.Getenv("LIVETOOL_LIVE_ROOM")
	if room == "" {
		t.Skip("未设置 LIVETOOL_LIVE_ROOM，跳过联网测试")
	}
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	service, err := NewAppService(store, dir, "test")
	if err != nil {
		t.Fatalf("创建服务失败: %v", err)
	}
	defer func() {
		service.stopConnector()
		service.stopEventWorkers()
		store.Close()
	}()

	status, err := service.ConnConnect(PlatformDouyin, room)
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	if status.State != connectorStateConnecting {
		t.Errorf("连接后状态应为 connecting，实际 %s", status.State)
	}

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) && service.ConnStatus().State != connectorStateConnected {
		time.Sleep(500 * time.Millisecond)
	}
	current := service.ConnStatus()
	if current.State != connectorStateConnected {
		t.Fatalf("60 秒内没有连上直播间：%+v", current)
	}
	t.Logf("已连接：platform=%s room=%s mode=%s", current.Platform, current.RoomID, current.Mode)

	deadline = time.Now().Add(60 * time.Second)
	var records []DanmakuRecord
	for time.Now().Before(deadline) {
		records, err = service.DanmakuQuery(DanmakuFilter{Source: string(PlatformDouyin), Limit: 5})
		if err != nil {
			t.Fatalf("查询弹幕日记失败: %v", err)
		}
		if len(records) > 0 {
			break
		}
		time.Sleep(time.Second)
	}
	if len(records) == 0 {
		t.Fatal("60 秒内没有弹幕记录落库")
	}
	t.Logf("日记首条：%s %s %s", records[0].Source, records[0].UserName, records[0].Text)

	after := service.ConnDisconnect()
	if after.State != connectorStateDisconnected {
		t.Errorf("断开后状态应为 disconnected，实际 %s", after.State)
	}
}
