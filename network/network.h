#ifndef NETWORK_H
#define NETWORK_H

#include <stddef.h>
#include <sys/types.h>

// TCP
int tcp_server_start(const char* host, int port);
int tcp_client_connect(const char* host, int port);
int tcp_accept(int server_fd);
ssize_t tcp_send(int fd, const char* buf, size_t len);
ssize_t tcp_recv(int fd, char* buf, size_t len);
int tcp_close(int fd);

// UDP
int udp_server_start(const char* host, int port);
int udp_client_connect(const char* host, int port);
ssize_t udp_send(int fd, const char* buf, size_t len, const char* host, int port);
ssize_t udp_recv(int fd, char* buf, size_t maxlen, char* out_ip, int* out_port);
int udp_close(int fd);

// HTTP Helpers (simple implementation over TCP sockets)
int http_request(const char* host, int port, const char* method, const char* path,
                 const char* content_type, const char* body, size_t body_len,
                 const char* extra_headers, char* response, size_t response_len);
int http_get(const char* host, int port, const char* path,
             const char* extra_headers, char* response, size_t response_len);
int http_post(const char* host, int port, const char* path,
              const char* content_type, const char* body, size_t body_len,
              const char* extra_headers, char* response, size_t response_len);
int http_put(const char* host, int port, const char* path,
             const char* content_type, const char* body, size_t body_len,
             const char* extra_headers, char* response, size_t response_len);
int http_delete(const char* host, int port, const char* path,
                const char* extra_headers, char* response, size_t response_len);
int http_post_file(const char* host, int port, const char* path, const char* filepath,
                   const char* field_name, const char* file_name,
                   const char* extra_headers, char* response, size_t response_len);

#endif

