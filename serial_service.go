package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"
)

type serialPulseRun struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func (s *AppService) SerialPorts() []SerialPort {
	paths, err := serial.GetPortsList()
	if err != nil {
		s.log("warn", "serial", "读取串口列表失败", err.Error())
		return []SerialPort{}
	}
	sort.Strings(paths)
	ports := make([]SerialPort, 0, len(paths))
	for _, path := range paths {
		if path = strings.TrimSpace(path); path != "" {
			ports = append(ports, SerialPort{Path: path})
		}
	}
	return ports
}

func (s *AppService) SerialPulse(action Action) OperationResult {
	if err := s.requireEntitlement("serial"); err != nil {
		return OperationResult{Message: err.Error()}
	}
	if err := validateSerialAction(action); err != nil {
		return OperationResult{Message: err.Error()}
	}
	if strings.EqualFold(strings.TrimSpace(action.Port), "VIRTUAL-COM1") {
		return OperationResult{Message: "这是旧版虚拟串口配置，请改选设备管理器中的实际串口"}
	}

	ctx, cancel := context.WithCancel(context.Background())
	run := &serialPulseRun{cancel: cancel, done: make(chan struct{})}
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		cancel()
		return OperationResult{Message: "应用正在退出，未发送串口数据"}
	}
	s.serialMu.Lock()
	if s.serialPulses == nil {
		s.serialPulses = map[*serialPulseRun]struct{}{}
	}
	s.serialPulses[run] = struct{}{}
	s.serialMu.Unlock()
	s.mu.Unlock()

	defer func() {
		cancel()
		s.serialMu.Lock()
		delete(s.serialPulses, run)
		s.serialMu.Unlock()
		close(run.done)
	}()

	lock := s.serialPortLock(action.Port)
	lock.Lock()
	defer lock.Unlock()
	if err := ctx.Err(); err != nil {
		return OperationResult{OK: true, Message: "串口脉冲已停止"}
	}
	if err := sendSerialPulse(ctx, action); err != nil {
		if errors.Is(err, context.Canceled) {
			if err != context.Canceled {
				s.log("warn", "serial", "串口脉冲停止时关闭状态未能确认", fmt.Sprintf("port=%s: %v", action.Port, err))
				return OperationResult{Message: fmt.Sprintf("串口 %s 已停止，但关闭字节或端口关闭失败：%v", action.Port, err)}
			}
			s.log("info", "serial", "串口脉冲已停止", "port="+action.Port)
			return OperationResult{OK: true, Message: "串口脉冲已停止，并已发送关闭字节"}
		}
		s.log("warn", "serial", "串口脉冲失败", fmt.Sprintf("port=%s: %v", action.Port, err))
		return OperationResult{Message: fmt.Sprintf("串口 %s 执行失败：%v", action.Port, err)}
	}
	s.log("info", "serial", "串口脉冲已发送", fmt.Sprintf("port=%s baud=%d duration=%dms", action.Port, action.Baud, action.PulseMS))
	return OperationResult{OK: true, Message: "串口脉冲已完成"}
}

func (s *AppService) SerialStopAll() {
	s.serialMu.Lock()
	runs := make([]*serialPulseRun, 0, len(s.serialPulses))
	for run := range s.serialPulses {
		runs = append(runs, run)
		run.cancel()
	}
	s.serialMu.Unlock()

	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for index, run := range runs {
		select {
		case <-run.done:
		case <-deadline.C:
			s.log("warn", "serial", "等待串口输出关闭超时", fmt.Sprintf("remaining=%d", len(runs)-index))
			return
		}
	}
	s.log("info", "serial", "已请求停止全部串口脉冲", fmt.Sprintf("active=%d", len(runs)))
}

func (s *AppService) serialPortLock(port string) *sync.Mutex {
	key := strings.ToLower(strings.TrimSpace(port))
	s.serialMu.Lock()
	defer s.serialMu.Unlock()
	if s.serialPortLocks == nil {
		s.serialPortLocks = map[string]*sync.Mutex{}
	}
	if s.serialPortLocks[key] == nil {
		s.serialPortLocks[key] = &sync.Mutex{}
	}
	return s.serialPortLocks[key]
}

func validateSerialAction(action Action) error {
	if strings.TrimSpace(action.Port) == "" {
		return errors.New("请先选择串口")
	}
	baud := action.Baud
	if baud == 0 {
		baud = 9600
	}
	if baud < 300 || baud > 4_000_000 {
		return errors.New("波特率需要在 300 到 4000000 之间")
	}
	if action.PulseMS < 1 || action.PulseMS > 60_000 {
		return errors.New("脉冲时长需要在 1 到 60000 毫秒之间")
	}
	if len(action.OnBytes) == 0 || len(action.OnBytes) > 4096 {
		return errors.New("开启字节需要包含 1 到 4096 个值")
	}
	if len(action.OffBytes) == 0 || len(action.OffBytes) > 4096 {
		return errors.New("关闭字节需要包含 1 到 4096 个值，以便释放输出")
	}
	for _, list := range [][]int{action.OnBytes, action.OffBytes} {
		for _, value := range list {
			if value < 0 || value > 255 {
				return errors.New("串口字节值必须在 0 到 255 之间")
			}
		}
	}
	return nil
}

func sendSerialPulse(ctx context.Context, action Action) (result error) {
	baud := action.Baud
	if baud == 0 {
		baud = 9600
	}
	mode := &serial.Mode{
		BaudRate: baud,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
		// Keep modem-control lines low; this action controls the configured bytes only.
		InitialStatusBits: &serial.ModemOutputBits{},
	}
	port, err := serial.Open(strings.TrimSpace(action.Port), mode)
	if err != nil {
		return err
	}
	defer func() {
		if offErr := writeSerialBytes(port, action.OffBytes); offErr != nil {
			result = errors.Join(result, fmt.Errorf("发送关闭字节失败: %w", offErr))
		} else if drainErr := port.Drain(); drainErr != nil {
			result = errors.Join(result, fmt.Errorf("等待关闭字节写入失败: %w", drainErr))
		}
		if closeErr := port.Close(); closeErr != nil {
			result = errors.Join(result, fmt.Errorf("关闭串口失败: %w", closeErr))
		}
	}()

	if err := writeSerialBytes(port, action.OnBytes); err != nil {
		return fmt.Errorf("发送开启字节失败: %w", err)
	}
	if err := port.Drain(); err != nil {
		return fmt.Errorf("等待开启字节写入失败: %w", err)
	}
	timer := time.NewTimer(time.Duration(action.PulseMS) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return context.Canceled
	case <-timer.C:
		return nil
	}
}

func writeSerialBytes(port serial.Port, values []int) error {
	data := make([]byte, len(values))
	for index, value := range values {
		data[index] = byte(value)
	}
	for len(data) > 0 {
		written, err := port.Write(data)
		if err != nil {
			return err
		}
		if written <= 0 {
			return errors.New("串口未写入数据")
		}
		if written > len(data) {
			return errors.New("串口返回了无效的写入长度")
		}
		data = data[written:]
	}
	return nil
}
