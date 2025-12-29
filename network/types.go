package network

/*
#include <stdlib.h>
#include "network.h"
*/
import "C"

import "unsafe"

// ProtocolMessageType represents the type of protocol message
type ProtocolMessageType uint8

const (
	MsgLogin     ProtocolMessageType = 0x01
	MsgCommand   ProtocolMessageType = 0x02
	MsgFileMeta  ProtocolMessageType = 0x03
	MsgFileChunk ProtocolMessageType = 0x04
	MsgFileDone  ProtocolMessageType = 0x05
	MsgAck       ProtocolMessageType = 0x06
	MsgError     ProtocolMessageType = 0x7F
)

// ProtocolMessage represents a decoded protocol frame
type ProtocolMessage struct {
	Type        ProtocolMessageType
	Raw         []byte
	DeviceID    string
	Token       string
	SessionID   string
	CommandJSON []byte

	FileName    string
	FileSize    uint64
	ChunkOffset uint32
	ChunkLen    uint32
	ChunkData   []byte

	StatusCode uint16
	StatusMsg  string
}

// convertProtocolMessage converts a C protocol message to Go
func convertProtocolMessage(cMsg *C.protocol_message_t) *ProtocolMessage {
	msgType := C.protocol_message_get_type(cMsg)
	pm := &ProtocolMessage{
		Type:        ProtocolMessageType(msgType),
		DeviceID:    C.GoString(&cMsg.device_id[0]),
		Token:       C.GoString(&cMsg.token[0]),
		SessionID:   C.GoString(&cMsg.session_id[0]),
		FileName:    C.GoString(&cMsg.file_name[0]),
		FileSize:    uint64(cMsg.file_size),
		ChunkOffset: uint32(cMsg.chunk_offset),
		ChunkLen:    uint32(cMsg.chunk_len),
		StatusCode:  uint16(cMsg.status_code),
		StatusMsg:   C.GoString(&cMsg.status_msg[0]),
	}

	if cMsg.data != nil && cMsg.data_len > 0 {
		pm.Raw = C.GoBytes(unsafe.Pointer(cMsg.data), C.int(cMsg.data_len))
	}

	switch pm.Type {
	case MsgCommand:
		pm.CommandJSON = pm.Raw
	case MsgFileChunk:
		if len(pm.Raw) >= 2 {
			sidLen := int(pm.Raw[0])
			tokLen := int(pm.Raw[1])
			pos := 2 + sidLen + tokLen + 8
			if pos <= len(pm.Raw) {
				pm.ChunkData = pm.Raw[pos:]
			}
		}
	}

	return pm
}
