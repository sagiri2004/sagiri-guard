#include "network.h"
#include <errno.h>
#include <stdlib.h>
#include <string.h>
#include <arpa/inet.h>

static uint64_t ntohll_u64(uint64_t v) {
#if __BYTE_ORDER__ == __ORDER_LITTLE_ENDIAN__
    return ((uint64_t)ntohl((uint32_t)(v & 0xFFFFFFFFULL)) << 32) | ntohl((uint32_t)(v >> 32));
#else
    return v;
#endif
}

static uint64_t htonll_u64(uint64_t v) {
#if __BYTE_ORDER__ == __ORDER_LITTLE_ENDIAN__
    return ((uint64_t)htonl((uint32_t)(v & 0xFFFFFFFFULL)) << 32) | htonl((uint32_t)(v >> 32));
#else
    return v;
#endif
}

static int recv_all_bytes(SOCKET fd, void* buf, size_t len) {
    size_t bytes_count = 0;
    while (bytes_count < len) {
        ssize_t n = recv(fd, (char*)buf + bytes_count, len - bytes_count, 0);
        if (n <= 0) return -1;
        bytes_count += n;
    }
    return 0;
}

static int protocol_send_frame(SOCKET fd, uint8_t type, const void* payload, uint32_t payload_len) {
    if (fd == INVALID_SOCKET) {
        return -1;
    }
    if (payload_len > PROTOCOL_MAX_PAYLOAD) {
        return -1;
    }

    protocol_frame_header_t hdr;
    hdr.type = type;
    hdr.length_be = htonl(payload_len);

    if (send_all(fd, (const char*)&hdr, sizeof(hdr)) != 0) {
        return -1;
    }
    if (payload_len > 0 && payload) {
        if (send_all(fd, (const char*)payload, payload_len) != 0) {
            return -1;
        }
    }
    return 0;
}

int protocol_send_login(SOCKET fd, const char* device_id, const char* token) {
    if (fd == INVALID_SOCKET || !device_id || !token) {
        return -1;
    }
    size_t device_len = strlen(device_id);
    size_t token_len = strlen(token);
    if (device_len == 0 || device_len > PROTOCOL_MAX_DEVICE_ID) {
        return -1;
    }
    if (token_len == 0 || token_len > PROTOCOL_MAX_TOKEN) {
        return -1;
    }

    uint32_t payload_len = 1 + 2 + (uint32_t)device_len + (uint32_t)token_len;
    char* payload = malloc(payload_len);
    if (!payload) return -1;

    payload[0] = (uint8_t)device_len;
    uint16_t token_len_net = htons((uint16_t)token_len);
    memcpy(payload + 1, &token_len_net, sizeof(token_len_net));
    memcpy(payload + 1 + 2, device_id, device_len);
    memcpy(payload + 1 + 2 + device_len, token, token_len);

    int rc = protocol_send_frame(fd, MSG_LOGIN, payload, payload_len);
    free(payload);
    return rc;
}

int protocol_send_command(SOCKET fd, const char* json, size_t json_len) {
    if (!json || json_len == 0 || json_len > PROTOCOL_MAX_PAYLOAD) {
        return -1;
    }
    return protocol_send_frame(fd, MSG_COMMAND, json, (uint32_t)json_len);
}

int protocol_send_file_meta(SOCKET fd, const char* file_name, uint64_t file_size) {
    if (!file_name) return -1;
    size_t name_len = strlen(file_name);
    if (name_len == 0 || name_len > PROTOCOL_MAX_FILENAME) return -1;

    uint32_t payload_len = 2 + 8 + (uint32_t)name_len;
    char* payload = malloc(payload_len);
    if (!payload) return -1;

    uint16_t name_len_net = htons((uint16_t)name_len);
    uint64_t size_net = htonll_u64(file_size);
    memcpy(payload, &name_len_net, sizeof(name_len_net));
    memcpy(payload + 2, &size_net, sizeof(size_net));
    memcpy(payload + 2 + 8, file_name, name_len);

    int rc = protocol_send_frame(fd, MSG_FILE_META, payload, payload_len);
    free(payload);
    return rc;
}

int protocol_send_file_chunk(SOCKET fd, const char* session_id, const char* token, uint32_t offset, const char* chunk, uint32_t chunk_len) {
    if (!chunk || chunk_len == 0 || chunk_len > PROTOCOL_MAX_PAYLOAD) {
        return -1;
    }
    size_t sid_len = session_id ? strlen(session_id) : 0;
    size_t tok_len = token ? strlen(token) : 0;
    if (sid_len > PROTOCOL_MAX_SESSION || tok_len > PROTOCOL_MAX_TOKEN) {
        return -1;
    }
    uint32_t payload_len = 1 + 1 + (uint32_t)sid_len + (uint32_t)tok_len + 4 + 4 + chunk_len;
    char* payload = malloc(payload_len);
    if (!payload) return -1;

    payload[0] = (uint8_t)sid_len;
    payload[1] = (uint8_t)tok_len;
    size_t pos = 2;
    if (sid_len > 0) {
        memcpy(payload + pos, session_id, sid_len);
        pos += sid_len;
    }
    if (tok_len > 0) {
        memcpy(payload + pos, token, tok_len);
        pos += tok_len;
    }
    uint32_t offset_net = htonl(offset);
    uint32_t len_net = htonl(chunk_len);
    memcpy(payload + pos, &offset_net, sizeof(offset_net));
    pos += 4;
    memcpy(payload + pos, &len_net, sizeof(len_net));
    pos += 4;
    memcpy(payload + pos, chunk, chunk_len);

    int rc = protocol_send_frame(fd, MSG_FILE_CHUNK, payload, payload_len);
    free(payload);
    return rc;
}

int protocol_send_file_done(SOCKET fd, const char* session_id, const char* token) {
    size_t sid_len = session_id ? strlen(session_id) : 0;
    size_t tok_len = token ? strlen(token) : 0;
    if (sid_len > PROTOCOL_MAX_SESSION || tok_len > PROTOCOL_MAX_TOKEN) {
        return -1;
    }
    uint32_t payload_len = 1 + 1 + (uint32_t)sid_len + (uint32_t)tok_len;
    char* payload = malloc(payload_len);
    if (!payload) return -1;
    payload[0] = (uint8_t)sid_len;
    payload[1] = (uint8_t)tok_len;
    size_t pos = 2;
    if (sid_len > 0) {
        memcpy(payload + pos, session_id, sid_len);
        pos += sid_len;
    }
    if (tok_len > 0) {
        memcpy(payload + pos, token, tok_len);
        pos += tok_len;
    }
    int rc = protocol_send_frame(fd, MSG_FILE_DONE, payload, payload_len);
    free(payload);
    return rc;
}

int protocol_send_ack(SOCKET fd, uint16_t status_code, const char* msg_text) {
    size_t msg_len = msg_text ? strlen(msg_text) : 0;
    if (msg_len > PROTOCOL_MAX_MESSAGE) return -1;

    uint32_t payload_len = 2 + 2 + (uint32_t)msg_len;
    char* payload = malloc(payload_len);
    if (!payload) return -1;

    uint16_t code_net = htons(status_code);
    uint16_t msg_len_net = htons((uint16_t)msg_len);
    memcpy(payload, &code_net, sizeof(code_net));
    memcpy(payload + 2, &msg_len_net, sizeof(msg_len_net));
    if (msg_len > 0) {
        memcpy(payload + 4, msg_text, msg_len);
    }

    int rc = protocol_send_frame(fd, MSG_ACK, payload, payload_len);
    free(payload);
    return rc;
}

int protocol_recv_message(SOCKET fd, protocol_message_t* msg) {
    if (!msg || fd == INVALID_SOCKET) {
        return -1;
    }
    memset(msg, 0, sizeof(protocol_message_t));

    protocol_frame_header_t hdr;
    if (recv_all_bytes(fd, &hdr, sizeof(hdr)) < 0) {
        return -1;
    }
    uint32_t payload_len = ntohl(hdr.length_be);
    if (payload_len > PROTOCOL_MAX_PAYLOAD) {
        return -1;
    }

    msg->type = hdr.type;
    msg->data_len = payload_len;

    if (payload_len > 0) {
        msg->data = malloc(payload_len);
        if (!msg->data) return -1;
        if (recv_all_bytes(fd, msg->data, payload_len) < 0) {
            free(msg->data);
            msg->data = NULL;
            return -1;
        }
    }

    const unsigned char* p = (const unsigned char*)msg->data;
    size_t remain = payload_len;

    switch (msg->type) {
    case MSG_LOGIN: {
        if (remain < 3) break;
        uint8_t dev_len = p[0];
        uint16_t token_len = ntohs(*(const uint16_t*)(p + 1));
        if (remain < 3 + dev_len + token_len) break;
        if (dev_len > PROTOCOL_MAX_DEVICE_ID || token_len > PROTOCOL_MAX_TOKEN) break;
        memcpy(msg->device_id, p + 3, dev_len);
        msg->device_id[dev_len] = '\0';
        memcpy(msg->token, p + 3 + dev_len, token_len);
        msg->token[token_len] = '\0';
        break;
    }
    case MSG_COMMAND: {
        // raw JSON already in msg->data
        break;
    }
    case MSG_FILE_META: {
        if (remain < 2 + 8) break;
        uint16_t name_len = ntohs(*(const uint16_t*)p);
        if (name_len > PROTOCOL_MAX_FILENAME) break;
        if (remain < 2 + 8 + name_len) break;
        uint64_t size_net;
        memcpy(&size_net, p + 2, sizeof(size_net));
        msg->file_size = ntohll_u64(size_net);
        memcpy(msg->file_name, p + 2 + 8, name_len);
        msg->file_name[name_len] = '\0';
        break;
    }
    case MSG_FILE_CHUNK: {
        if (remain < 2) break;
        uint8_t sid_len = p[0];
        uint8_t tok_len = p[1];
        size_t pos = 2;
        if (sid_len > PROTOCOL_MAX_SESSION || tok_len > PROTOCOL_MAX_TOKEN) break;
        if (remain < pos + sid_len + tok_len + 8) break;
        memcpy(msg->session_id, p + pos, sid_len);
        msg->session_id[sid_len] = '\0';
        pos += sid_len;
        memcpy(msg->token, p + pos, tok_len);
        msg->token[tok_len] = '\0';
        pos += tok_len;
        msg->chunk_offset = ntohl(*(const uint32_t*)(p + pos));
        msg->chunk_len = ntohl(*(const uint32_t*)(p + pos + 4));
        // data already stored starting at pos+8
        break;
    }
    case MSG_FILE_DONE: {
        if (remain < 2) break;
        uint8_t sid_len = p[0];
        uint8_t tok_len = p[1];
        size_t pos = 2;
        if (sid_len > PROTOCOL_MAX_SESSION || tok_len > PROTOCOL_MAX_TOKEN) break;
        if (remain < pos + sid_len + tok_len) break;
        memcpy(msg->session_id, p + pos, sid_len);
        msg->session_id[sid_len] = '\0';
        pos += sid_len;
        memcpy(msg->token, p + pos, tok_len);
        msg->token[tok_len] = '\0';
        break;
    }
    case MSG_ACK:
    case MSG_ERROR: {
        if (remain < 4) break;
        msg->status_code = ntohs(*(const uint16_t*)p);
        uint16_t mlen = ntohs(*(const uint16_t*)(p + 2));
        if (mlen > PROTOCOL_MAX_MESSAGE) break;
        if (remain < 4 + mlen) break;
        memcpy(msg->status_msg, p + 4, mlen);
        msg->status_msg[mlen] = '\0';
        break;
    }
    default:
        break;
    }

    return 0;
}

void protocol_message_free(protocol_message_t* msg) {
    if (!msg) return;
    if (msg->data) {
        free(msg->data);
        msg->data = NULL;
    }
}

uint8_t protocol_message_get_type(const protocol_message_t* msg) {
    if (!msg) return MSG_ERROR;
    return msg->type;
}
