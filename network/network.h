#ifndef NETWORK_H
#define NETWORK_H

#include "tcp.h"
#include "protocol_types.h"
#include "protocol.h"

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

// Helper functions to get Go bridge callbacks
protocol_message_cb protocol_get_message_bridge(void);
protocol_disconnect_cb protocol_get_disconnect_bridge(void);

// Protocol Server Functions (multi-client handled in C)
int protocol_server_create(const char* host, int port, protocol_message_cb on_message, protocol_disconnect_cb on_disconnect, void* user_data, protocol_server_t** out_server);
int protocol_server_stop(protocol_server_t* server);
void protocol_server_destroy(protocol_server_t* server);

// Device registry (server-side) for sending to a device by ID
int protocol_device_is_online(const char* device_id);
int protocol_send_to_device(const char* device_id, const char* json, size_t json_len);

#endif
