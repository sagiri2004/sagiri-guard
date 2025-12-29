#include "network.h"
#include <pthread.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

// Protocol server structure
struct protocol_server {
    tcp_server_t* tcp_server;
    protocol_message_cb on_message;
    protocol_disconnect_cb on_disconnect;
    void* user_data;
};

// Device registry (server side)
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
    return protocol_send_command(fd, json, json_len);
}

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
    
    // Remove from registry first so DeviceIsOnline returns false during cleanup
    registry_remove_fd(client_fd);

    // Call disconnect callback if registered and we have a device_id
    if (pserver->on_disconnect && last_device[0] != '\0') {
        pserver->on_disconnect(client_fd, last_device, pserver->user_data);
    }
    
    tcp_close(client_fd);
}

// Helper function to get Go bridge function pointers
// Note: Go export functions have non-const signatures
extern void goProtocolMessageBridge(SOCKET client_fd, protocol_message_t* msg, void* user_data);
extern void goProtocolDisconnectBridge(SOCKET client_fd, char* device_id, void* user_data);

protocol_message_cb protocol_get_message_bridge(void) {
    return (protocol_message_cb)goProtocolMessageBridge;
}

protocol_disconnect_cb protocol_get_disconnect_bridge(void) {
    return (protocol_disconnect_cb)goProtocolDisconnectBridge;
}

int protocol_server_create(const char* host, int port, protocol_message_cb on_message, protocol_disconnect_cb on_disconnect, void* user_data, protocol_server_t** out_server) {
    if (!out_server || !on_message || port <= 0) {
        return -1;
    }

    protocol_server_t* pserver = calloc(1, sizeof(protocol_server_t));
    if (!pserver) {
        return -1;
    }

    pserver->on_message = on_message;
    pserver->on_disconnect = on_disconnect;
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
