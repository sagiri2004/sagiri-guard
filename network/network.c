#include "network.h"
#include "protocol.h"

#include <errno.h>
#include <pthread.h>
#include <signal.h>
#include <stdatomic.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <stdbool.h>
#include <arpa/inet.h>
#include <netinet/in.h>

#define BACKLOG 16
struct tcp_server {
    SOCKET listen_fd;
    pthread_t accept_thread;
    tcp_connection_cb handler;
    void* user_data;
    atomic_bool running;
};

struct connection_task {
    tcp_server_t* server;
    SOCKET client_fd;
};

static void ignore_sigpipe(void) {
#ifdef SIGPIPE
    static atomic_bool installed = ATOMIC_VAR_INIT(false);
    bool expected = false;
    if (atomic_compare_exchange_strong(&installed, &expected, true)) {
        signal(SIGPIPE, SIG_IGN);
    }
#endif
}

int network_init(void) {
    ignore_sigpipe();
    return 0;
}

void network_cleanup(void) {
    // No-op on Linux
}

static int set_reuseaddr(SOCKET fd) {
    int opt = 1;
    return setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt));
}

static int get_addrinfo_for(const char* host, int port, int socktype, int flags, struct addrinfo** out) {
    struct addrinfo hints;
    char port_str[16];
    memset(&hints, 0, sizeof(hints));
    hints.ai_family = AF_UNSPEC;
    hints.ai_socktype = socktype;
    hints.ai_flags = flags;
    snprintf(port_str, sizeof(port_str), "%d", port);
    return getaddrinfo(host, port_str, &hints, out);
}

static SOCKET bind_socket(const char* host, int port, int socktype) {
    struct addrinfo* res = NULL;
    SOCKET fd = INVALID_SOCKET;

    if (get_addrinfo_for(host, port, socktype, AI_PASSIVE, &res) != 0) {
        return INVALID_SOCKET;
    }

    for (struct addrinfo* p = res; p; p = p->ai_next) {
        fd = socket(p->ai_family, p->ai_socktype, p->ai_protocol);
        if (fd == INVALID_SOCKET) {
            continue;
        }
        set_reuseaddr(fd);
        if (bind(fd, p->ai_addr, p->ai_addrlen) == 0) {
            break;
        }
        close(fd);
        fd = INVALID_SOCKET;
    }

    freeaddrinfo(res);
    return fd;
}

static SOCKET connect_socket(const char* host, int port, int socktype) {
    struct addrinfo* res = NULL;
    SOCKET fd = INVALID_SOCKET;

    if (get_addrinfo_for(host, port, socktype, 0, &res) != 0) {
        return INVALID_SOCKET;
    }

    for (struct addrinfo* p = res; p; p = p->ai_next) {
        fd = socket(p->ai_family, p->ai_socktype, p->ai_protocol);
        if (fd == INVALID_SOCKET) {
            continue;
        }
        if (connect(fd, p->ai_addr, p->ai_addrlen) == 0) {
            break;
        }
        close(fd);
        fd = INVALID_SOCKET;
    }

    freeaddrinfo(res);
    return fd;
}

static int send_all(SOCKET fd, const char* data, size_t len) {
    size_t total = 0;
    while (total < len) {
        ssize_t sent = send(fd, data + total, len - total, 0);
        if (sent < 0) {
            if (errno == EINTR) {
                continue;
            }
            return -1;
        }
        if (sent == 0) {
            break;
        }
        total += (size_t)sent;
    }
    return (total == len) ? 0 : -1;
}

SOCKET tcp_server_start(const char* host, int port) {
    if (port <= 0) {
        return INVALID_SOCKET;
    }
    SOCKET fd = bind_socket(host, port, SOCK_STREAM);
    if (fd == INVALID_SOCKET) {
        return INVALID_SOCKET;
    }
    if (listen(fd, BACKLOG) != 0) {
        close(fd);
        return INVALID_SOCKET;
    }
    return fd;
}

SOCKET tcp_accept(SOCKET server_fd) {
    if (server_fd == INVALID_SOCKET) {
        return INVALID_SOCKET;
    }
    SOCKET client_fd = accept(server_fd, NULL, NULL);
    if (client_fd < 0) {
        return INVALID_SOCKET;
    }
    return client_fd;
}

SOCKET tcp_client_connect(const char* host, int port) {
    if (!host || port <= 0) {
        return INVALID_SOCKET;
    }
    return connect_socket(host, port, SOCK_STREAM);
}

ssize_t tcp_send(SOCKET fd, const char* buf, size_t len) {
    if (fd == INVALID_SOCKET || !buf) {
        return -1;
    }
    if (len == 0) {
        return 0;
    }
    if (send_all(fd, buf, len) != 0) {
        return -1;
    }
    return (ssize_t)len;
}

ssize_t tcp_recv(SOCKET fd, char* buf, size_t len) {
    if (fd == INVALID_SOCKET || !buf || len == 0) {
        return -1;
    }
    while (1) {
        ssize_t received = recv(fd, buf, len, 0);
        if (received < 0) {
            if (errno == EINTR) {
                continue;
            }
            return -1;
        }
        return received;
    }
}

int tcp_close(SOCKET fd) {
    if (fd == INVALID_SOCKET) {
        return -1;
    }
    return close(fd);
}

static void* connection_worker(void* arg) {
    struct connection_task* task = (struct connection_task*)arg;
    if (!task) {
        return NULL;
    }

    tcp_server_t* server = task->server;
    SOCKET client_fd = task->client_fd;
    free(task);

    if (server && server->handler) {
        server->handler(client_fd, server->user_data);
    }

    tcp_close(client_fd);
    return NULL;
}

static void* accept_loop(void* arg) {
    tcp_server_t* server = (tcp_server_t*)arg;
    if (!server) {
        return NULL;
    }

    while (atomic_load_explicit(&server->running, memory_order_acquire)) {
        SOCKET client_fd = accept(server->listen_fd, NULL, NULL);
        if (client_fd < 0) {
            if (!atomic_load_explicit(&server->running, memory_order_acquire)) {
                break;
            }
            if (errno == EINTR) {
                continue;
            }
            continue;
        }

        struct connection_task* task = malloc(sizeof(struct connection_task));
        if (!task) {
            tcp_close(client_fd);
            continue;
        }

        task->server = server;
        task->client_fd = client_fd;

        pthread_t thread_id;
        int rc = pthread_create(&thread_id, NULL, connection_worker, task);
        if (rc != 0) {
            free(task);
            tcp_close(client_fd);
            continue;
        }
        pthread_detach(thread_id);
    }

    return NULL;
}

int tcp_server_create(const char* host, int port, tcp_connection_cb handler, void* user_data, tcp_server_t** out_server) {
    if (!out_server || !handler || port <= 0) {
        return -1;
    }

    SOCKET listen_fd = tcp_server_start(host, port);
    if (listen_fd == INVALID_SOCKET) {
        return -1;
    }

    tcp_server_t* server = calloc(1, sizeof(tcp_server_t));
    if (!server) {
        tcp_close(listen_fd);
        return -1;
    }

    server->listen_fd = listen_fd;
    server->handler = handler;
    server->user_data = user_data;
    atomic_init(&server->running, true);

    int rc = pthread_create(&server->accept_thread, NULL, accept_loop, server);
    if (rc != 0) {
        atomic_store(&server->running, false);
        tcp_close(listen_fd);
        free(server);
        return -1;
    }

    *out_server = server;
    return 0;
}

int tcp_server_stop(tcp_server_t* server) {
    if (!server) {
        return -1;
    }

    bool expected = true;
    if (!atomic_compare_exchange_strong(&server->running, &expected, false)) {
        return 0;
    }

    shutdown(server->listen_fd, SHUT_RDWR);
    tcp_close(server->listen_fd);
    pthread_join(server->accept_thread, NULL);
    return 0;
}

void tcp_server_destroy(tcp_server_t* server) {
    if (!server) {
        return;
    }
    tcp_server_stop(server);
    free(server);
}

// ========== Protocol Message Functions ==========

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

// Protocol server structure (forward declared in network.h)
struct protocol_server {
    tcp_server_t* tcp_server;
    protocol_message_cb on_message;
    void* user_data;
};

// ---------------------- Device registry (server side) ----------------------
typedef struct device_entry {
    char device_id[PROTOCOL_MAX_DEVICE_ID + 1];
    SOCKET fd;
    struct device_entry* next;
} device_entry_t;

static device_entry_t* g_devices = NULL;
static pthread_mutex_t g_devices_mu = PTHREAD_MUTEX_INITIALIZER;

static void registry_set(const char* device_id, SOCKET fd) {
    if (!device_id || device_id[0] == '\0' || fd == INVALID_SOCKET) return;
    pthread_mutex_lock(&g_devices_mu);
    device_entry_t* prev = NULL;
    device_entry_t* cur = g_devices;
    while (cur) {
        if (strncmp(cur->device_id, device_id, PROTOCOL_MAX_DEVICE_ID) == 0) {
            cur->fd = fd;
            pthread_mutex_unlock(&g_devices_mu);
            return;
        }
        prev = cur;
        cur = cur->next;
    }
    device_entry_t* e = calloc(1, sizeof(device_entry_t));
    if (e) {
        strncpy(e->device_id, device_id, PROTOCOL_MAX_DEVICE_ID);
        e->device_id[PROTOCOL_MAX_DEVICE_ID] = '\0';
        e->fd = fd;
        e->next = g_devices;
        g_devices = e;
    }
    pthread_mutex_unlock(&g_devices_mu);
}

static void registry_remove_fd(SOCKET fd) {
    if (fd == INVALID_SOCKET) return;
    pthread_mutex_lock(&g_devices_mu);
    device_entry_t* prev = NULL;
    device_entry_t* cur = g_devices;
    while (cur) {
        if (cur->fd == fd) {
            if (prev) prev->next = cur->next;
            else g_devices = cur->next;
            free(cur);
            break;
        }
        prev = cur;
        cur = cur->next;
    }
    pthread_mutex_unlock(&g_devices_mu);
}

static SOCKET registry_get(const char* device_id) {
    if (!device_id || device_id[0] == '\0') return INVALID_SOCKET;
    SOCKET fd = INVALID_SOCKET;
    pthread_mutex_lock(&g_devices_mu);
    for (device_entry_t* cur = g_devices; cur; cur = cur->next) {
        if (strncmp(cur->device_id, device_id, PROTOCOL_MAX_DEVICE_ID) == 0) {
            fd = cur->fd;
            break;
        }
    }
    pthread_mutex_unlock(&g_devices_mu);
    return fd;
}

int protocol_device_is_online(const char* device_id) {
    return registry_get(device_id) != INVALID_SOCKET;
}

int protocol_send_to_device(const char* device_id, const char* json, size_t json_len) {
    SOCKET fd = registry_get(device_id);
    if (fd == INVALID_SOCKET) return -1;
    int rc = protocol_send_command(fd, json, json_len);
    if (rc != 0) {
        registry_remove_fd(fd);
    }
    return rc;
}

// ---------------------- Send helpers ----------------------

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

// ---------------------- Receive ----------------------

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

// Protocol server callback type
typedef void (*protocol_message_cb)(SOCKET client_fd, const protocol_message_t* msg, void* user_data);

static void protocol_connection_handler(SOCKET client_fd, void* user_data) {
    protocol_server_t* pserver = (protocol_server_t*)user_data;
    if (!pserver || !pserver->on_message) {
        tcp_close(client_fd);
        return;
    }

    protocol_message_t msg;
    char last_device[PROTOCOL_MAX_DEVICE_ID + 1] = {0};

    while (1) {
        if (protocol_recv_message(client_fd, &msg) < 0) {
            break;
        }

        // Preserve device_id learned from login for subsequent frames
        if (msg.device_id[0] == '\0' && last_device[0] != '\0') {
            strncpy(msg.device_id, last_device, sizeof(msg.device_id) - 1);
            msg.device_id[sizeof(msg.device_id) - 1] = '\0';
        } else if (msg.device_id[0] != '\0') {
            strncpy(last_device, msg.device_id, sizeof(last_device) - 1);
            last_device[sizeof(last_device) - 1] = '\0';
            if (msg.type == MSG_LOGIN) {
                registry_set(last_device, client_fd);
            }
            }

            pserver->on_message(client_fd, &msg, pserver->user_data);
            protocol_message_free(&msg);
    }

    protocol_message_free(&msg);
    registry_remove_fd(client_fd);
    tcp_close(client_fd);
}

// Protocol server API
int protocol_server_create(const char* host, int port, protocol_message_cb on_message, void* user_data, protocol_server_t** out_server) {
    if (!out_server || !on_message || port <= 0) {
        return -1;
    }

    protocol_server_t* pserver = calloc(1, sizeof(protocol_server_t));
    if (!pserver) {
        return -1;
    }

    pserver->on_message = on_message;
    pserver->user_data = user_data;

    int rc = tcp_server_create(host, port, protocol_connection_handler, pserver, &pserver->tcp_server);
    if (rc != 0) {
        free(pserver);
        return -1;
    }

    *out_server = pserver;
    return 0;
}

int protocol_server_stop(protocol_server_t* server) {
    if (!server || !server->tcp_server) {
        return -1;
    }
    return tcp_server_stop(server->tcp_server);
}

void protocol_server_destroy(protocol_server_t* server) {
    if (!server) {
        return;
    }
    if (server->tcp_server) {
        tcp_server_destroy(server->tcp_server);
    }
    free(server);
}

