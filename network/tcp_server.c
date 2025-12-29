#include "tcp.h"
#include <errno.h>
#include <pthread.h>
#include <stdbool.h>
#include <stdatomic.h>
#include <stdlib.h>
#include <unistd.h>

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
