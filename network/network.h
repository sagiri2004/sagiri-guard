#ifndef NETWORK_H
#define NETWORK_H

#include <stddef.h>
#include <stdint.h>
#include <sys/socket.h>
#include <sys/types.h>
#include <netdb.h>
#include "protocol.h"

typedef int SOCKET;

typedef struct tcp_server tcp_server_t;

typedef void (*tcp_connection_cb)(SOCKET client_fd, void* user_data);

#ifndef INVALID_SOCKET
#define INVALID_SOCKET (-1)
#endif

/**
 * @brief Khởi tạo lớp network nền tảng (WSAStartup trên Windows, no-op trên POSIX).
 */
int network_init(void);

/**
 * @brief Dọn dẹp tài nguyên network của nền tảng (WSACleanup trên Windows, no-op trên POSIX).
 */
void network_cleanup(void);

// TCP (legacy socket helpers)
SOCKET tcp_server_start(const char* host, int port);
SOCKET tcp_client_connect(const char* host, int port);
SOCKET tcp_accept(SOCKET server_fd);
ssize_t tcp_send(SOCKET fd, const char* buf, size_t len);
ssize_t tcp_recv(SOCKET fd, char* buf, size_t len);
int tcp_close(SOCKET fd);

// TCP threaded server API
int tcp_server_create(const char* host, int port, tcp_connection_cb handler, void* user_data, tcp_server_t** out_server);
int tcp_server_stop(tcp_server_t* server);
void tcp_server_destroy(tcp_server_t* server);

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

typedef struct protocol_server protocol_server_t;

typedef void (*protocol_message_cb)(SOCKET client_fd, const protocol_message_t* msg, void* user_data);

// Protocol Client Functions
int protocol_send_login(SOCKET fd, const char* device_id, const char* token);
int protocol_send_command(SOCKET fd, const char* json, size_t json_len);
int protocol_send_file_meta(SOCKET fd, const char* file_name, uint64_t file_size);
int protocol_send_file_chunk(SOCKET fd, const char* session_id, const char* token, uint32_t offset, const char* chunk, uint32_t chunk_len);
int protocol_send_file_done(SOCKET fd, const char* session_id, const char* token);
int protocol_send_ack(SOCKET fd, uint16_t status_code, const char* msg);
int protocol_recv_message(SOCKET fd, protocol_message_t* msg);
void protocol_message_free(protocol_message_t* msg);
uint8_t protocol_message_get_type(const protocol_message_t* msg);

// Protocol Server Functions (multi-client handled in C)
int protocol_server_create(const char* host, int port, protocol_message_cb on_message, void* user_data, protocol_server_t** out_server);
int protocol_server_stop(protocol_server_t* server);
void protocol_server_destroy(protocol_server_t* server);

// Device registry (server-side) for sending to a device by ID
int protocol_device_is_online(const char* device_id);
int protocol_send_to_device(const char* device_id, const char* json, size_t json_len);

#endif
