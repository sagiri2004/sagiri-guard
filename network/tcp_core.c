#include "tcp.h"
#include <errno.h>
#include <signal.h>
#include <stdbool.h>
#include <stdatomic.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <arpa/inet.h>
#include <netinet/in.h>

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

int set_reuseaddr(SOCKET fd) {
    int opt = 1;
    return setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt));
}

int get_addrinfo_for(const char* host, int port, int socktype, int flags, struct addrinfo** out) {
    struct addrinfo hints;
    char port_str[16];
    memset(&hints, 0, sizeof(hints));
    hints.ai_family = AF_UNSPEC;
    hints.ai_socktype = socktype;
    hints.ai_flags = flags;
    snprintf(port_str, sizeof(port_str), "%d", port);
    return getaddrinfo(host, port_str, &hints, out);
}

SOCKET bind_socket(const char* host, int port, int socktype) {
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

SOCKET connect_socket(const char* host, int port, int socktype) {
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

int send_all(SOCKET fd, const char* data, size_t len) {
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
