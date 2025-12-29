#ifndef TCP_H
#define TCP_H

#include <stddef.h>
#include <sys/socket.h>
#include <sys/types.h>
#include <netdb.h>

// Socket type definition
typedef int SOCKET;

#ifndef INVALID_SOCKET
#define INVALID_SOCKET (-1)
#endif

#define BACKLOG 16

// Opaque server type
typedef struct tcp_server tcp_server_t;

// Connection callback typedef
typedef void (*tcp_connection_cb)(SOCKET client_fd, void* user_data);

// Platform initialization
int network_init(void);
void network_cleanup(void);

// Core TCP functions
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

// Internal helpers (exposed for other modules)
int set_reuseaddr(SOCKET fd);
int get_addrinfo_for(const char* host, int port, int socktype, int flags, struct addrinfo** out);
SOCKET bind_socket(const char* host, int port, int socktype);
SOCKET connect_socket(const char* host, int port, int socktype);
int send_all(SOCKET fd, const char* data, size_t len);

#endif
