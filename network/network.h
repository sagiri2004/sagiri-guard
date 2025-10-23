#ifndef NETWORK_H
#define NETWORK_H

#include <stddef.h>

// --- Thay đổi cho Windows ---
#include <winsock2.h>
#include <ws2tcpip.h>

#if defined(_MSC_VER) && !defined(_SSIZE_T_DEFINED)
#ifdef _WIN64
typedef __int64 ssize_t;
#else
typedef int ssize_t;
#endif
#define _SSIZE_T_DEFINED
#endif
// --- Kết thúc thay đổi cho Windows ---

// --- Thêm hàm khởi tạo và dọn dẹp cho Winsock ---
/**
 * @brief Khởi tạo thư viện Winsock.
 * Phải được gọi một lần khi bắt đầu chương trình.
 * @return 0 nếu thành công, -1 nếu thất bại.
 */
int network_init(void);

/**
 * @brief Dọn dẹp thư viện Winsock.
 * Phải được gọi một lần khi kết thúc chương trình.
 */
void network_cleanup(void);
// --- Kết thúc thêm hàm ---


// TCP
// Thay đổi: kiểu trả về và tham số file descriptor (fd) là SOCKET
SOCKET tcp_server_start(const char* host, int port);
SOCKET tcp_client_connect(const char* host, int port);
SOCKET tcp_accept(SOCKET server_fd);
ssize_t tcp_send(SOCKET fd, const char* buf, size_t len);
ssize_t tcp_recv(SOCKET fd, char* buf, size_t len);
int tcp_close(SOCKET fd);

// UDP removed (unused)

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
// http_post_file removed (unused)

#endif
