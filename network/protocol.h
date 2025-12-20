#ifndef PROTOCOL_H
#define PROTOCOL_H

#include <stdint.h>

// Message types (1 byte)
#define MSG_LOGIN        0x01  // Agent -> Backend: login with device & token
#define MSG_COMMAND      0x02  // Backend -> Agent: JSON command payload
#define MSG_FILE_META    0x03  // Either direction: announce file upload/download meta
#define MSG_FILE_CHUNK   0x04  // Either direction: file data chunk
#define MSG_FILE_DONE    0x05  // Either direction: file transfer finished
#define MSG_ACK          0x06  // Generic ACK with status code & message
#define MSG_ERROR        0x7F  // Generic error

// Protocol states (server side)
#define STATE_WAIT_LOGIN 0
#define STATE_LOGGED_IN  1

// Limits
#define PROTOCOL_MAX_PAYLOAD   (1024 * 1024)   // 1MB per frame
#define PROTOCOL_MAX_DEVICE_ID 255
#define PROTOCOL_MAX_TOKEN     1024
#define PROTOCOL_MAX_SESSION   128
#define PROTOCOL_MAX_FILENAME  512
#define PROTOCOL_MAX_MESSAGE   1024

#pragma pack(push, 1)
typedef struct protocol_frame_header {
    uint8_t  type;          // MSG_*
    uint32_t length_be;     // payload length in network byte order
} protocol_frame_header_t;
#pragma pack(pop)

#endif

