#include "network.h"

// --- Thay đổi cho Windows ---
#define _CRT_SECURE_NO_WARNINGS // Tắt cảnh báo cho fopen, snprintf
#define _WINSOCK_DEPRECATED_NO_WARNINGS // Tắt cảnh báo cho một số hàm cũ

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h> // Dùng cho _stat

// Link với thư viện ws2_32.lib
#pragma comment(lib, "ws2_32.lib")

// Các include của POSIX đã bị xóa (unistd.h, arpa/inet.h, v.v.)
// vì winsock2.h và ws2tcpip.h đã thay thế chúng.
// --- Kết thúc thay đổi cho Windows ---


#define BACKLOG 16                  
#define DEFAULT_HTTP_PORT 80
#define MULTIPART_BOUNDARY "----CGoNetworkBoundary" 

// --- Hàm mới cho Winsock ---
int network_init(void) {
    WSADATA wsaData;
    int result = WSAStartup(MAKEWORD(2, 2), &wsaData);
    if (result != 0) {
        return -1; // Lỗi: Không thể khởi tạo Winsock
    }
    return 0;
}

void network_cleanup(void) {
    WSACleanup();
}
// --- Kết thúc hàm mới ---

/**
 * @brief Thiết lập tùy chọn SO_REUSEADDR cho một socket. (Phiên bản Windows)
 */
static int set_reuseaddr(SOCKET fd) {
    int opt = 1;
    // Thay đổi: tham số thứ 4 là (const char*)
    return setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, (const char*)&opt, sizeof(opt));
}

/**
 * @brief Lấy thông tin địa chỉ (addrinfo).
 * (Hàm này giữ nguyên vì getaddrinfo là chuẩn)
 */
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

/**
 * @brief Tạo và bind một socket. (Phiên bản Windows)
 * @return SOCKET nếu thành công, INVALID_SOCKET nếu thất bại.
 */
static SOCKET bind_socket(const char* host, int port, int socktype) {
    struct addrinfo* res = NULL;
    int rc = get_addrinfo_for(host, port, socktype, AI_PASSIVE, &res);
    if (rc != 0) {
        return INVALID_SOCKET; // Thay -1 bằng INVALID_SOCKET
    }

    SOCKET fd = INVALID_SOCKET; // Thay int bằng SOCKET, -1 bằng INVALID_SOCKET
    for (struct addrinfo* p = res; p; p = p->ai_next) {
        fd = socket(p->ai_family, p->ai_socktype, p->ai_protocol);
        if (fd == INVALID_SOCKET) { // Thay < 0 bằng == INVALID_SOCKET
            continue;
        }
        set_reuseaddr(fd); 
        if (bind(fd, p->ai_addr, (int)p->ai_addrlen) == 0) {
            break;
        }
        closesocket(fd); // Thay close() bằng closesocket()
        fd = INVALID_SOCKET;
    }

    freeaddrinfo(res);
    return fd;
}

/**
 * @brief Tạo và kết nối một socket. (Phiên bản Windows)
 * @return SOCKET nếu thành công, INVALID_SOCKET nếu thất bại.
 */
static SOCKET connect_socket(const char* host, int port, int socktype) {
    struct addrinfo* res = NULL;
    int rc = get_addrinfo_for(host, port, socktype, 0, &res);
    if (rc != 0) {
        return INVALID_SOCKET;
    }

    SOCKET fd = INVALID_SOCKET;
    for (struct addrinfo* p = res; p; p = p->ai_next) {
        fd = socket(p->ai_family, p->ai_socktype, p->ai_protocol);
        if (fd == INVALID_SOCKET) {
            continue;
        }
        if (connect(fd, p->ai_addr, (int)p->ai_addrlen) == 0) {
            break;
        }
        closesocket(fd);
        fd = INVALID_SOCKET;
    }

    freeaddrinfo(res);
    return fd;
}

/**
 * @brief Gửi toàn bộ dữ liệu. (Phiên bản Windows)
 */
static int send_all(SOCKET fd, const char* data, size_t len) {
    size_t total = 0;
    while (total < len) {
        // Thay đổi: send() trả về 'int', không phải 'ssize_t'
        int sent = send(fd, data + total, (int)(len - total), 0);
        if (sent < 0) {
            // Thay đổi: dùng WSAGetLastError() thay vì errno
            if (WSAGetLastError() == WSAEINTR) { 
                continue;
            }
            return -1; // Lỗi
        }
        if (sent == 0) { // SOCKET_ERROR (thường là -1) mới là lỗi
            break;
        }
        total += (size_t)sent;
    }
    return (total == len) ? 0 : -1;
}

/**
 * @brief Nhận dữ liệu vào buffer. (Phiên bản Windows)
 */
static ssize_t recv_into_buffer(SOCKET fd, char* buffer, size_t buffer_len) {
    size_t total = 0;
    while (total + 1 < buffer_len) {
        // Thay đổi: recv() trả về 'int'
        int received = recv(fd, buffer + total, (int)(buffer_len - 1 - total), 0);
        if (received < 0) {
            // Thay đổi: dùng WSAGetLastError() thay vì errno
            if (WSAGetLastError() == WSAEINTR) {
                continue;
            }
            return -1; // Lỗi
        }
        if (received == 0) { // Kết nối đã đóng
            break;
        }
        total += (size_t)received;
    }
    buffer[total] = '\0';
    return (ssize_t)total;
}

/**
 * @brief Trích xuất tên file từ đường dẫn. (Phiên bản Windows)
 * Hỗ trợ cả hai dấu '/' và '\'
 */
static const char* basename_ptr(const char* path) {
    const char* slash = strrchr(path, '/');
    const char* backslash = strrchr(path, '\\');
    
    if (!slash && !backslash) return path;
    if (slash && !backslash) return slash + 1;
    if (!slash && backslash) return backslash + 1;

    // Cả hai đều tồn tại, trả về cái cuối cùng
    return (slash > backslash ? slash : backslash) + 1;
}

/**
 * @brief Khởi tạo server TCP. (Phiên bản Windows)
 */
SOCKET tcp_server_start(const char* host, int port) {
    if (port <= 0) {
        return INVALID_SOCKET;
    }
    SOCKET fd = bind_socket(host, port, SOCK_STREAM);
    if (fd == INVALID_SOCKET) {
        return INVALID_SOCKET;
    }
    if (listen(fd, BACKLOG) != 0) {
        closesocket(fd);
        return INVALID_SOCKET;
    }
    return fd;
}

/**
 * @brief Chấp nhận kết nối TCP. (Phiên bản Windows)
 */
SOCKET tcp_accept(SOCKET server_fd) {
    if (server_fd == INVALID_SOCKET) {
        return INVALID_SOCKET;
    }
    SOCKET client_fd = accept(server_fd, NULL, NULL);
    if (client_fd == INVALID_SOCKET) {
        return INVALID_SOCKET;
    }
    return client_fd;
}

/**
 * @brief Kết nối tới server TCP. (Phiên bản Windows)
 */
SOCKET tcp_client_connect(const char* host, int port) {
    if (!host || port <= 0) {
        return INVALID_SOCKET;
    }
    return connect_socket(host, port, SOCK_STREAM);
}

/**
 * @brief Gửi dữ liệu TCP. (Phiên bản Windows)
 */
ssize_t tcp_send(SOCKET fd, const char* buf, size_t len) {
    if (fd == INVALID_SOCKET || !buf) {
        return -1;
    }
    if (send_all(fd, buf, len) != 0) {
        return -1;
    }
    // Trả về ssize_t (có thể là -1 nếu lỗi, hoặc len nếu thành công)
    return (ssize_t)len;
}

/**
 * @brief Nhận dữ liệu TCP. (Phiên bản Windows)
 */
ssize_t tcp_recv(SOCKET fd, char* buf, size_t len) {
    if (fd == INVALID_SOCKET || !buf || len == 0) {
        return -1;
    }
    while (1) {
        // Thay đổi: recv() trả về 'int'
        int received = recv(fd, buf, (int)len, 0);
        if (received < 0) {
            if (WSAGetLastError() == WSAEINTR) {
                continue;
            }
            return -1; // Lỗi
        }
        return (ssize_t)received; // Trả về số byte nhận được (cast sang ssize_t)
    }
}

/**
 * @brief Đóng kết nối TCP. (Phiên bản Windows)
 */
int tcp_close(SOCKET fd) {
    if (fd == INVALID_SOCKET) {
        return -1;
    }
    return closesocket(fd); // Dùng closesocket()
}

/**
 * @brief Khởi tạo server UDP. (Phiên bản Windows)
 */
SOCKET udp_server_start(const char* host, int port) {
    if (port <= 0) {
        return INVALID_SOCKET;
    }
    SOCKET fd = bind_socket(host, port, SOCK_DGRAM);
    return fd;
}

/**
 * @brief Tạo socket UDP client. (Phiên bản Windows)
 */
SOCKET udp_client_connect(const char* host, int port) {
    if (!host || port <= 0) {
        return INVALID_SOCKET;
    }
    return connect_socket(host, port, SOCK_DGRAM);
}

/**
 * @brief Gửi dữ liệu UDP. (Phiên bản Windows)
 */
ssize_t udp_send(SOCKET fd, const char* buf, size_t len, const char* host, int port) {
    if (fd == INVALID_SOCKET || !buf) {
        return -1;
    }
    if (host && port > 0) {
        struct addrinfo* res = NULL;
        if (get_addrinfo_for(host, port, SOCK_DGRAM, 0, &res) != 0) {
            return -1;
        }
        // Thay đổi: sendto() trả về 'int'
        int result = sendto(fd, buf, (int)len, 0, res->ai_addr, (int)res->ai_addrlen);
        freeaddrinfo(res);
        return (ssize_t)result;
    }
    // Thay đổi: send() trả về 'int'
    int result = send(fd, buf, (int)len, 0);
    return (ssize_t)result;
}

/**
 * @brief Nhận dữ liệu UDP. (Phiên bản Windows)
 */
ssize_t udp_recv(SOCKET fd, char* buf, size_t maxlen, char* out_ip, int* out_port) {
    if (fd == INVALID_SOCKET || !buf || maxlen == 0) {
        return -1;
    }
    struct sockaddr_storage addr; 
    int addrlen = sizeof(addr); // Dùng int cho Windows
    
    // Thay đổi: recvfrom() trả về 'int'
    int received = recvfrom(fd, buf, (int)maxlen, 0, (struct sockaddr*)&addr, &addrlen);
    if (received < 0) {
        return -1;
    }

    if (out_ip) {
        void* src = NULL;
        if (addr.ss_family == AF_INET) {
            src = &((struct sockaddr_in*)&addr)->sin_addr;
        } else if (addr.ss_family == AF_INET6) {
            src = &((struct sockaddr_in6*)&addr)->sin6_addr;
        }
        if (src) {
            // inet_ntop có sẵn trong ws2tcpip.h
            inet_ntop(addr.ss_family, src, out_ip, INET6_ADDRSTRLEN);
        }
    }
    if (out_port) {
        if (addr.ss_family == AF_INET) {
            *out_port = ntohs(((struct sockaddr_in*)&addr)->sin_port);
        } else if (addr.ss_family == AF_INET6) {
            *out_port = ntohs(((struct sockaddr_in6*)&addr)->sin6_port);
        }
    }

    return (ssize_t)received;
}

/**
 * @brief Đóng socket UDP. (Phiên bản Windows)
 */
int udp_close(SOCKET fd) {
    if (fd == INVALID_SOCKET) {
        return -1;
    }
    return closesocket(fd);
}

/**
 * @brief Thực hiện yêu cầu HTTP chung. (Phiên bản Windows)
 */
int http_request(const char* host, int port, const char* method, const char* path,
                 const char* content_type, const char* body, size_t body_len,
                 const char* extra_headers, char* response, size_t response_len) {
    if (!host || !method || !path || !response || response_len == 0) {
        return -1;
    }
    if (body_len > 0 && !body) {
        return -1;
    }

    if (port <= 0) {
        port = DEFAULT_HTTP_PORT;
    }

    // Thay đổi: dùng SOCKET và kiểm tra INVALID_SOCKET
    SOCKET fd = connect_socket(host, port, SOCK_STREAM);
    if (fd == INVALID_SOCKET) {
        return -1;
    }

    char host_line[512];
    if (port == 80 || port == 0) {
        snprintf(host_line, sizeof(host_line), "%s", host);
    } else {
        snprintf(host_line, sizeof(host_line), "%s:%d", host, port);
    }

    char header[4096]; 
    size_t offset = 0;

    // snprintf là chuẩn C99, MSVC (Visual Studio) hỗ trợ
    int written = snprintf(header + offset, sizeof(header) - offset,
                           "%s %s HTTP/1.1\r\n", method, path);
    if (written < 0 || (size_t)written >= sizeof(header) - offset) {
        closesocket(fd); return -1;
    }
    offset += (size_t)written;

    written = snprintf(header + offset, sizeof(header) - offset,
                       "Host: %s\r\n", host_line);
    if (written < 0 || (size_t)written >= sizeof(header) - offset) {
        closesocket(fd); return -1;
    }
    offset += (size_t)written;

    written = snprintf(header + offset, sizeof(header) - offset,
                       "Connection: close\r\n");
    if (written < 0 || (size_t)written >= sizeof(header) - offset) {
        closesocket(fd); return -1;
    }
    offset += (size_t)written;

    if (content_type && body_len > 0) {
        written = snprintf(header + offset, sizeof(header) - offset,
                           "Content-Type: %s\r\n", content_type);
        if (written < 0 || (size_t)written >= sizeof(header) - offset) {
            closesocket(fd); return -1;
        }
        offset += (size_t)written;
    }

    if (body_len > 0) {
        written = snprintf(header + offset, sizeof(header) - offset,
                           "Content-Length: %zu\r\n", body_len);
        if (written < 0 || (size_t)written >= sizeof(header) - offset) {
            closesocket(fd); return -1;
        }
        offset += (size_t)written;
    }

    if (extra_headers && extra_headers[0] != '\0') {
        size_t eh_len = strlen(extra_headers);
        if (eh_len >= sizeof(header) - offset) {
            closesocket(fd); return -1;
        }
        memcpy(header + offset, extra_headers, eh_len);
        offset += eh_len;
        if (eh_len < 2 || strncmp(extra_headers + eh_len - 2, "\r\n", 2) != 0) {
            if (offset + 2 >= sizeof(header)) {
                closesocket(fd); return -1;
            }
            header[offset++] = '\r';
            header[offset++] = '\n';
        }
    }

    if (offset + 2 >= sizeof(header)) {
        closesocket(fd); return -1;
    }
    header[offset++] = '\r';
    header[offset++] = '\n';

    if (send_all(fd, header, offset) != 0) {
        closesocket(fd); return -1;
    }

    if (body_len > 0 && send_all(fd, body, body_len) != 0) {
        closesocket(fd); return -1;
    }

    ssize_t received = recv_into_buffer(fd, response, response_len);
    closesocket(fd); 

    return (received < 0) ? -1 : 0;
}

// Các hàm http_get, http_post, http_put, http_delete
// không thay đổi vì chúng chỉ gọi http_request

int http_get(const char* host, int port, const char* path,
             const char* extra_headers, char* response, size_t response_len) {
    return http_request(host, port, "GET", path, NULL, NULL, 0, extra_headers, response, response_len);
}

int http_post(const char* host, int port, const char* path,
              const char* content_type, const char* body, size_t body_len,
              const char* extra_headers, char* response, size_t response_len) {
    return http_request(host, port, "POST", path, content_type, body, body_len,
                        extra_headers, response, response_len);
}

int http_put(const char* host, int port, const char* path,
             const char* content_type, const char* body, size_t body_len,
             const char* extra_headers, char* response, size_t response_len) {
    return http_request(host, port, "PUT", path, content_type, body, body_len,
                        extra_headers, response, response_len);
}

int http_delete(const char* host, int port, const char* path,
                const char* extra_headers, char* response, size_t response_len) {
    return http_request(host, port, "DELETE", path, NULL, NULL, 0,
                        extra_headers, response, response_len);
}

/**
 * @brief Gửi file qua HTTP POST. (Phiên bản Windows)
 */
int http_post_file(const char* host, int port, const char* path, const char* filepath,
                   const char* field_name, const char* file_name,
                   const char* extra_headers, char* response, size_t response_len) {
    if (!filepath || !host || !path) {
        return -1;
    }

    if (!field_name || field_name[0] == '\0') {
        field_name = "file";
    }

    const char* upload_name = file_name && file_name[0] != '\0' ? file_name : basename_ptr(filepath);

    // Thay đổi: sử dụng struct _stat và hàm _stat
    struct _stat st;
    if (_stat(filepath, &st) != 0) {
        return -1; 
    }
    // Thay đổi: sử dụng cờ _S_IFREG
    if (!(st.st_mode & _S_IFREG)) {
        return -1; 
    }

    size_t file_size = (size_t)st.st_size;
    FILE* fp = fopen(filepath, "rb"); 
    if (!fp) {
        return -1;
    }

    char preamble[1024];
    int preamble_len = snprintf(preamble, sizeof(preamble),
                                "--%s\r\n"
                                "Content-Disposition: form-data; name=\"%s\"; filename=\"%s\"\r\n"
                                "Content-Type: application/octet-stream\r\n\r\n",
                                MULTIPART_BOUNDARY, field_name, upload_name);
    if (preamble_len < 0 || (size_t)preamble_len >= sizeof(preamble)) {
        fclose(fp); return -1;
    }

    char closing[128];
    int closing_len = snprintf(closing, sizeof(closing),
                               "\r\n--%s--\r\n", MULTIPART_BOUNDARY);
    if (closing_len < 0 || (size_t)closing_len >= sizeof(closing)) {
        fclose(fp); return -1;
    }

    size_t total_len = (size_t)preamble_len + file_size + (size_t)closing_len;
    char* body = (char*)malloc(total_len);
    if (!body) {
        fclose(fp); return -1;
    }

    memcpy(body, preamble, (size_t)preamble_len);
    size_t offset = (size_t)preamble_len;

    size_t read_total = fread(body + offset, 1, file_size, fp);
    fclose(fp);

    if (read_total != file_size) { 
        free(body);
        return -1;
    }

    memcpy(body + offset + read_total, closing, (size_t)closing_len);

    char content_type[128];
    snprintf(content_type, sizeof(content_type),
             "multipart/form-data; boundary=%s",
             MULTIPART_BOUNDARY);

    int rc = http_request(host, port, "POST", path, content_type,
                          body, total_len, extra_headers, response, response_len);
    free(body);
    return rc;
}
