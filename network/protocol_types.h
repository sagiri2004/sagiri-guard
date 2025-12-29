#ifndef PROTOCOL_TYPES_H
#define PROTOCOL_TYPES_H

#include <stdint.h>
#include "protocol.h"
#include "tcp.h"

// Protocol message representation (decoded where possible)
typedef struct protocol_message {
    uint8_t  type;        // MSG_*
    uint32_t data_len;    // raw payload length
    char*    data;        // raw payload (malloc'd for non-inline types)

    // Decoded fields (optional depending on type)
    char     device_id[PROTOCOL_MAX_DEVICE_ID + 1];
    char     token[PROTOCOL_MAX_TOKEN + 1];
    char     session_id[PROTOCOL_MAX_SESSION + 1];

    char     file_name[PROTOCOL_MAX_FILENAME + 1];
    uint64_t file_size;
    uint32_t chunk_offset;
    uint32_t chunk_len;

    uint16_t status_code; // for ACK/ERROR
    char     status_msg[PROTOCOL_MAX_MESSAGE + 1];
} protocol_message_t;

// Opaque protocol server type
typedef struct protocol_server protocol_server_t;

// Protocol callback types
typedef void (*protocol_message_cb)(SOCKET client_fd, const protocol_message_t* msg, void* user_data);
typedef void (*protocol_disconnect_cb)(SOCKET client_fd, const char* device_id, void* user_data);

#endif
