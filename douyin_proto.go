package main

import "errors"

// 抖音的弹幕推送是 protobuf 编码。字段编号取自原版二进制里的 FileDescriptorProto
// （`myapp/core/douyinlive/generated/pb`：PushFrame / MyResponse / Message /
// MyChatMessage / MyGiftMessage / MyMemberMessage / MyLikeMessage / MySocialMessage /
// MyRoomUserSeqMessage），并用真实抓包样本回归（testdata/douyin-messages.json）。
//
// 这里实现解码所需的最小 protobuf 子集（varint / 64 位 / 长度前缀 / 32 位），
// 避免为单个平台引入代码生成工具链。

const (
	protoWireVarint  = 0
	protoWireFixed64 = 1
	protoWireBytes   = 2
	protoWireFixed32 = 5
)

var (
	errProtoTruncated = errors.New("protobuf 数据不完整")
	errProtoOverflow  = errors.New("protobuf varint 溢出")
)

// protoField 是一个已解码的 protobuf 字段。
type protoField struct {
	number int
	wire   int
	value  uint64
	bytes  []byte
}

func (f protoField) isBytes() bool { return f.wire == protoWireBytes }

// protoFields 解析一段 protobuf 数据的所有顶层字段。
func protoFields(data []byte) ([]protoField, error) {
	fields := make([]protoField, 0, 8)
	pos := 0
	for pos < len(data) {
		key, next, err := protoReadVarint(data, pos)
		if err != nil {
			return nil, err
		}
		pos = next
		field := protoField{number: int(key >> 3), wire: int(key & 7)}
		switch field.wire {
		case protoWireVarint:
			value, next, err := protoReadVarint(data, pos)
			if err != nil {
				return nil, err
			}
			field.value, pos = value, next
		case protoWireFixed64:
			if pos+8 > len(data) {
				return nil, errProtoTruncated
			}
			field.value = uint64(data[pos]) | uint64(data[pos+1])<<8 | uint64(data[pos+2])<<16 |
				uint64(data[pos+3])<<24 | uint64(data[pos+4])<<32 | uint64(data[pos+5])<<40 |
				uint64(data[pos+6])<<48 | uint64(data[pos+7])<<56
			pos += 8
		case protoWireBytes:
			length, next, err := protoReadVarint(data, pos)
			if err != nil {
				return nil, err
			}
			pos = next
			if length > uint64(len(data)-pos) {
				return nil, errProtoTruncated
			}
			field.bytes = data[pos : pos+int(length)]
			pos += int(length)
		case protoWireFixed32:
			if pos+4 > len(data) {
				return nil, errProtoTruncated
			}
			field.value = uint64(data[pos]) | uint64(data[pos+1])<<8 | uint64(data[pos+2])<<16 | uint64(data[pos+3])<<24
			pos += 4
		default:
			// 未知 wire type（3/4 是已废弃的 group），无法继续解析。
			return nil, errProtoTruncated
		}
		fields = append(fields, field)
	}
	return fields, nil
}

func protoReadVarint(data []byte, pos int) (uint64, int, error) {
	var value uint64
	var shift uint
	for pos < len(data) {
		current := data[pos]
		pos++
		value |= uint64(current&0x7f) << shift
		if current < 0x80 {
			return value, pos, nil
		}
		shift += 7
		if shift >= 64 {
			return 0, pos, errProtoOverflow
		}
	}
	return 0, pos, errProtoTruncated
}

// firstBytes 返回指定字段编号的第一个长度前缀字段。
func firstBytes(fields []protoField, number int) []byte {
	for _, field := range fields {
		if field.number == number && field.isBytes() {
			return field.bytes
		}
	}
	return nil
}

// firstString 返回指定字段编号的第一个字符串字段。
func firstString(fields []protoField, number int) string {
	return string(firstBytes(fields, number))
}

// firstUint 返回指定字段编号的第一个 varint 字段。
func firstUint(fields []protoField, number int) uint64 {
	for _, field := range fields {
		if field.number == number && field.wire == protoWireVarint {
			return field.value
		}
	}
	return 0
}

// allBytes 返回指定字段编号的所有长度前缀字段。
func allBytes(fields []protoField, number int) [][]byte {
	var values [][]byte
	for _, field := range fields {
		if field.number == number && field.isBytes() {
			values = append(values, field.bytes)
		}
	}
	return values
}
