#include "network.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <arpa/inet.h>
#include <sys/socket.h>
#include <netdb.h>
#include <errno.h>
#include <sys/stat.h>
#include <netinet/in.h>

#define BACKLOG 16                  // Số lượng kết nối tối đa có thể chờ trong hàng đợi
#define DEFAULT_HTTP_PORT 80        // Cổng HTTP mặc định
#define MULTIPART_BOUNDARY "----CGoNetworkBoundary" // Ranh giới cho dữ liệu multipart/form-data

/**
 * @brief Thiết lập tùy chọn SO_REUSEADDR cho một socket.
 * * Tùy chọn này cho phép server khởi động lại và bind vào một địa chỉ/cổng đã được sử dụng gần đây
 * mà không cần chờ hết thời gian timeout (trạng thái TIME_WAIT).
 * * @param fd File descriptor của socket.
 * @return 0 nếu thành công, -1 nếu thất bại.
 */
static int set_reuseaddr(int fd) {
    int opt = 1;
    return setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt));
}

/**
 * @brief Lấy thông tin địa chỉ (addrinfo) cho một host và port cụ thể.
 * * Đây là hàm trợ giúp để gói gọn việc gọi `getaddrinfo`, giúp chuyển đổi tên miền (host)
 * và số hiệu cổng (port) thành một tập hợp các cấu trúc địa chỉ socket.
 * * @param host Tên miền hoặc địa chỉ IP.
 * @param port Số hiệu cổng.
 * @param socktype Loại socket (ví dụ: SOCK_STREAM cho TCP, SOCK_DGRAM cho UDP).
 * @param flags Các cờ bổ sung cho `getaddrinfo` (ví dụ: AI_PASSIVE cho server).
 * @param out Con trỏ để lưu trữ kết quả danh sách các addrinfo.
 * @return 0 nếu thành công, khác 0 nếu có lỗi.
 */
static int get_addrinfo_for(const char* host, int port, int socktype, int flags, struct addrinfo** out) {
    struct addrinfo hints;
    char port_str[16];
    memset(&hints, 0, sizeof(hints)); // Khởi tạo hints với giá trị 0
    hints.ai_family = AF_UNSPEC;      // Chấp nhận cả IPv4 và IPv6
    hints.ai_socktype = socktype;     // Loại socket được chỉ định
    hints.ai_flags = flags;           // Cờ cho biết mục đích sử dụng (server/client)
    snprintf(port_str, sizeof(port_str), "%d", port); // Chuyển đổi port từ int sang chuỗi
    return getaddrinfo(host, port_str, &hints, out); // Lấy thông tin địa chỉ
}

/**
 * @brief Tạo và bind một socket tới một host và port.
 * * Hàm này dùng cho phía server. Nó lặp qua danh sách các địa chỉ có thể có
 * từ `getaddrinfo` và cố gắng tạo và bind socket vào địa chỉ đầu tiên thành công.
 * * @param host Tên miền hoặc địa chỉ IP để bind. Nếu là NULL, sẽ bind vào tất cả các interface.
 * @param port Số hiệu cổng để bind.
 * @param socktype Loại socket (SOCK_STREAM hoặc SOCK_DGRAM).
 * @return File descriptor của socket đã được bind nếu thành công, -1 nếu thất bại.
 */
static int bind_socket(const char* host, int port, int socktype) {
    struct addrinfo* res = NULL;
    // Lấy danh sách địa chỉ để lắng nghe (AI_PASSIVE)
    int rc = get_addrinfo_for(host, port, socktype, AI_PASSIVE, &res);
    if (rc != 0) {
        return -1; // Không thể phân giải địa chỉ
    }

    int fd = -1;
    // Lặp qua từng địa chỉ trong danh sách liên kết
    for (struct addrinfo* p = res; p; p = p->ai_next) {
        // Tạo socket với thông tin từ addrinfo
        fd = socket(p->ai_family, p->ai_socktype, p->ai_protocol);
        if (fd < 0) {
            continue; // Thất bại, thử địa chỉ tiếp theo
        }
        set_reuseaddr(fd); // Cho phép tái sử dụng địa chỉ
        // Bind socket vào địa chỉ
        if (bind(fd, p->ai_addr, p->ai_addrlen) == 0) {
            break; // Bind thành công, thoát khỏi vòng lặp
        }
        close(fd); // Bind thất bại, đóng socket và thử lại
        fd = -1;
    }

    freeaddrinfo(res); // Giải phóng bộ nhớ của danh sách addrinfo
    return fd;
}

/**
 * @brief Tạo và kết nối một socket tới một host và port.
 * * Hàm này dùng cho phía client. Nó lặp qua danh sách các địa chỉ có thể có
 * và cố gắng kết nối tới địa chỉ đầu tiên thành công.
 * * @param host Tên miền hoặc địa chỉ IP của server.
 * @param port Số hiệu cổng của server.
 * @param socktype Loại socket (SOCK_STREAM hoặc SOCK_DGRAM).
 * @return File descriptor của socket đã kết nối nếu thành công, -1 nếu thất bại.
 */
static int connect_socket(const char* host, int port, int socktype) {
    struct addrinfo* res = NULL;
    // Lấy danh sách địa chỉ của server
    int rc = get_addrinfo_for(host, port, socktype, 0, &res);
    if (rc != 0) {
        return -1; // Không thể phân giải địa chỉ
    }

    int fd = -1;
    // Lặp qua từng địa chỉ trong danh sách
    for (struct addrinfo* p = res; p; p = p->ai_next) {
        // Tạo socket
        fd = socket(p->ai_family, p->ai_socktype, p->ai_protocol);
        if (fd < 0) {
            continue; // Thất bại, thử địa chỉ tiếp theo
        }
        // Kết nối tới server
        if (connect(fd, p->ai_addr, p->ai_addrlen) == 0) {
            break; // Kết nối thành công, thoát vòng lặp
        }
        close(fd); // Kết nối thất bại, đóng socket và thử lại
        fd = -1;
    }

    freeaddrinfo(res); // Giải phóng bộ nhớ
    return fd;
}

/**
 * @brief Gửi toàn bộ dữ liệu trong buffer qua socket.
 * * Hàm `send()` có thể không gửi hết dữ liệu trong một lần gọi. Hàm này đảm bảo
 * tất cả dữ liệu được gửi đi bằng cách lặp lại việc gọi `send()` cho đến khi
 * toàn bộ buffer được gửi hoặc có lỗi xảy ra.
 * * @param fd File descriptor của socket.
 * @param data Con trỏ tới buffer chứa dữ liệu cần gửi.
 * @param len Độ dài của dữ liệu.
 * @return 0 nếu gửi thành công toàn bộ, -1 nếu có lỗi.
 */
static int send_all(int fd, const char* data, size_t len) {
    size_t total = 0;
    while (total < len) {
        // Gửi phần dữ liệu còn lại
        ssize_t sent = send(fd, data + total, len - total, 0);
        if (sent < 0) {
            if (errno == EINTR) { // Bị ngắt bởi một tín hiệu, thử lại
                continue;
            }
            return -1; // Lỗi thực sự
        }
        if (sent == 0) { // Socket đã đóng
            break;
        }
        total += (size_t)sent; // Cộng dồn số byte đã gửi
    }
    return (total == len) ? 0 : -1; // Trả về 0 chỉ khi gửi đủ
}

/**
 * @brief Nhận dữ liệu từ socket và lưu vào buffer cho đến khi đầy hoặc kết nối đóng.
 * * Hàm này đọc dữ liệu từ socket và đảm bảo kết quả là một chuỗi được kết thúc bằng null.
 * * @param fd File descriptor của socket.
 * @param buffer Buffer để lưu dữ liệu nhận được.
 * @param buffer_len Kích thước của buffer.
 * @return Số byte đã nhận nếu thành công, -1 nếu có lỗi.
 */
static ssize_t recv_into_buffer(int fd, char* buffer, size_t buffer_len) {
    size_t total = 0;
    // Vòng lặp nhận dữ liệu, chừa lại 1 byte cho ký tự null
    while (total + 1 < buffer_len) {
        ssize_t received = recv(fd, buffer + total, buffer_len - 1 - total, 0);
        if (received < 0) {
            if (errno == EINTR) { // Bị ngắt, thử lại
                continue;
            }
            return -1; // Lỗi thực sự
        }
        if (received == 0) { // Phía bên kia đã đóng kết nối
            break;
        }
        total += (size_t)received; // Cộng dồn số byte đã nhận
    }
    buffer[total] = '\0'; // Thêm ký tự null để tạo thành chuỗi hợp lệ
    return (ssize_t)total;
}

/**
 * @brief Trích xuất tên file từ một đường dẫn đầy đủ.
 * * @param path Đường dẫn file.
 * @return Con trỏ tới phần tên file trong chuỗi đường dẫn.
 */
static const char* basename_ptr(const char* path) {
    const char* slash = strrchr(path, '/'); // Tìm dấu '/' cuối cùng
    if (!slash) {
        return path; // Không có '/', toàn bộ chuỗi là tên file
    }
    return slash + 1; // Trả về con trỏ ngay sau dấu '/'
}

/**
 * @brief Khởi tạo một server TCP.
 * * Tạo, bind và lắng nghe trên một socket TCP.
 * * @param host Địa chỉ IP để bind (hoặc NULL cho tất cả).
 * @param port Cổng để lắng nghe.
 * @return File descriptor của socket server nếu thành công, -1 nếu thất bại.
 */
int tcp_server_start(const char* host, int port) {
    if (port <= 0) {
        return -1;
    }
    int fd = bind_socket(host, port, SOCK_STREAM); // Bind socket TCP
    if (fd < 0) {
        return -1;
    }
    // Bắt đầu lắng nghe kết nối
    if (listen(fd, BACKLOG) != 0) {
        close(fd);
        return -1;
    }
    return fd;
}

int tcp_accept(int server_fd) {
    if (server_fd < 0) {
        return -1;
    }
    int client_fd = accept(server_fd, NULL, NULL);
    if (client_fd < 0) {
        return -1;
    }
    return client_fd;
}

/**
 * @brief Kết nối tới một server TCP.
 * * @param host Địa chỉ IP hoặc tên miền của server.
 * @param port Cổng của server.
 * @return File descriptor của socket client nếu thành công, -1 nếu thất bại.
 */
int tcp_client_connect(const char* host, int port) {
    if (!host || port <= 0) {
        return -1;
    }
    return connect_socket(host, port, SOCK_STREAM); // Kết nối socket TCP
}

/**
 * @brief Gửi dữ liệu qua một kết nối TCP.
 * * @param fd File descriptor của socket.
 * @param buf Buffer chứa dữ liệu.
 * @param len Độ dài dữ liệu.
 * @return Số byte đã gửi nếu thành công, -1 nếu thất bại.
 */
ssize_t tcp_send(int fd, const char* buf, size_t len) {
    if (fd < 0 || !buf) {
        return -1;
    }
    if (send_all(fd, buf, len) != 0) { // Đảm bảo gửi hết
        return -1;
    }
    return (ssize_t)len;
}

/**
 * @brief Nhận dữ liệu từ một kết nối TCP.
 * * @param fd File descriptor của socket.
 * @param buf Buffer để lưu dữ liệu.
 * @param len Kích thước tối đa của buffer.
 * @return Số byte đã nhận, 0 nếu kết nối đóng, -1 nếu có lỗi.
 */
ssize_t tcp_recv(int fd, char* buf, size_t len) {
    if (fd < 0 || !buf || len == 0) {
        return -1;
    }
    while (1) {
        ssize_t received = recv(fd, buf, len, 0);
        if (received < 0) {
            if (errno == EINTR) { // Bị ngắt, thử lại
                continue;
            }
            return -1; // Lỗi
        }
        return received; // Trả về số byte nhận được
    }
}

/**
 * @brief Đóng một kết nối TCP.
 * * @param fd File descriptor của socket.
 * @return 0 nếu thành công, -1 nếu thất bại.
 */
int tcp_close(int fd) {
    if (fd < 0) {
        return -1;
    }
    return close(fd);
}

/**
 * @brief Khởi tạo một server UDP.
 * * @param host Địa chỉ IP để bind (hoặc NULL).
 * @param port Cổng để lắng nghe.
 * @return File descriptor của socket server nếu thành công, -1 nếu thất bại.
 */
int udp_server_start(const char* host, int port) {
    if (port <= 0) {
        return -1;
    }
    int fd = bind_socket(host, port, SOCK_DGRAM); // Bind socket UDP
    return fd;
}

/**
 * @brief Tạo một socket UDP client và kết nối (thiết lập địa chỉ mặc định).
 * * @param host Địa chỉ IP hoặc tên miền của server.
 * @param port Cổng của server.
 * @return File descriptor của socket client nếu thành công, -1 nếu thất bại.
 */
int udp_client_connect(const char* host, int port) {
    if (!host || port <= 0) {
        return -1;
    }
    return connect_socket(host, port, SOCK_DGRAM); // Kết nối socket UDP
}

/**
 * @brief Gửi dữ liệu qua UDP.
 * * Nếu host và port được cung cấp, sử dụng `sendto`. Nếu không, sử dụng `send` (cho socket đã connect).
 * * @param fd File descriptor của socket.
 * @param buf Buffer dữ liệu.
 * @param len Độ dài dữ liệu.
 * @param host Địa chỉ IP đích (tùy chọn).
 * @param port Cổng đích (tùy chọn).
 * @return Số byte đã gửi nếu thành công, -1 nếu thất bại.
 */
ssize_t udp_send(int fd, const char* buf, size_t len, const char* host, int port) {
    if (fd < 0 || !buf) {
        return -1;
    }
    if (host && port > 0) { // Nếu có địa chỉ đích cụ thể
        struct addrinfo* res = NULL;
        if (get_addrinfo_for(host, port, SOCK_DGRAM, 0, &res) != 0) {
            return -1;
        }
        ssize_t result = sendto(fd, buf, len, 0, res->ai_addr, res->ai_addrlen);
        freeaddrinfo(res);
        return result;
    }
    // Nếu không, socket phải đã được "connect"
    return send(fd, buf, len, 0);
}

/**
 * @brief Nhận dữ liệu qua UDP và lấy thông tin người gửi.
 * * @param fd File descriptor của socket.
 * @param buf Buffer để lưu dữ liệu.
 * @param maxlen Kích thước tối đa của buffer.
 * @param out_ip Buffer để lưu địa chỉ IP của người gửi (tùy chọn).
 * @param out_port Con trỏ để lưu cổng của người gửi (tùy chọn).
 * @return Số byte đã nhận nếu thành công, -1 nếu thất bại.
 */
ssize_t udp_recv(int fd, char* buf, size_t maxlen, char* out_ip, int* out_port) {
    if (fd < 0 || !buf || maxlen == 0) {
        return -1;
    }
    struct sockaddr_storage addr; // Lưu trữ địa chỉ người gửi (IPv4/IPv6)
    socklen_t addrlen = sizeof(addr);
    ssize_t received = recvfrom(fd, buf, maxlen, 0, (struct sockaddr*)&addr, &addrlen);
    if (received < 0) {
        return -1;
    }

    // Trích xuất IP nếu được yêu cầu
    if (out_ip) {
        void* src = NULL;
        if (addr.ss_family == AF_INET) { // IPv4
            src = &((struct sockaddr_in*)&addr)->sin_addr;
        } else if (addr.ss_family == AF_INET6) { // IPv6
            src = &((struct sockaddr_in6*)&addr)->sin6_addr;
        }
        if (src) {
            // Chuyển đổi địa chỉ từ dạng nhị phân sang chuỗi
            inet_ntop(addr.ss_family, src, out_ip, INET6_ADDRSTRLEN);
        }
    }
    // Trích xuất port nếu được yêu cầu
    if (out_port) {
        if (addr.ss_family == AF_INET) {
            *out_port = ntohs(((struct sockaddr_in*)&addr)->sin_port);
        } else if (addr.ss_family == AF_INET6) {
            *out_port = ntohs(((struct sockaddr_in6*)&addr)->sin6_port);
        }
    }

    return received;
}

/**
 * @brief Đóng một socket UDP.
 * * @param fd File descriptor của socket.
 * @return 0 nếu thành công, -1 nếu thất bại.
 */
int udp_close(int fd) {
    if (fd < 0) {
        return -1;
    }
    return close(fd);
}

/**
 * @brief Thực hiện một yêu cầu HTTP chung.
 * * Đây là hàm cốt lõi để tạo và gửi các yêu cầu GET, POST, PUT, DELETE.
 * * @param host Tên miền hoặc IP của server.
 * @param port Cổng của server.
 * @param method Phương thức HTTP (ví dụ: "GET", "POST").
 * @param path Đường dẫn tài nguyên (ví dụ: "/index.html").
 * @param content_type Loại nội dung của body (ví dụ: "application/json").
 * @param body Nội dung của yêu cầu (request body).
 * @param body_len Độ dài của body.
 * @param extra_headers Các header bổ sung, mỗi header kết thúc bằng "\r\n".
 * @param response Buffer để lưu trữ phản hồi từ server.
 * @param response_len Kích thước của buffer phản hồi.
 * @return 0 nếu thành công, -1 nếu thất bại.
 */
int http_request(const char* host, int port, const char* method, const char* path,
                 const char* content_type, const char* body, size_t body_len,
                 const char* extra_headers, char* response, size_t response_len) {
    // Kiểm tra các tham số đầu vào cơ bản
    if (!host || !method || !path || !response || response_len == 0) {
        return -1;
    }
    if (body_len > 0 && !body) {
        return -1;
    }

    if (port <= 0) {
        port = DEFAULT_HTTP_PORT; // Sử dụng cổng mặc định nếu không được chỉ định
    }

    // Kết nối tới server
    int fd = connect_socket(host, port, SOCK_STREAM);
    if (fd < 0) {
        return -1;
    }

    // Tạo dòng Host header, bao gồm cả port nếu không phải là 80
    char host_line[512];
    if (port == 80 || port == 0) {
        snprintf(host_line, sizeof(host_line), "%s", host);
    } else {
        snprintf(host_line, sizeof(host_line), "%s:%d", host, port);
    }

    char header[4096]; // Buffer để xây dựng toàn bộ HTTP header
    size_t offset = 0;

    // Xây dựng dòng yêu cầu đầu tiên: "METHOD /path HTTP/1.1\r\n"
    int written = snprintf(header + offset, sizeof(header) - offset,
                           "%s %s HTTP/1.1\r\n", method, path);
    if (written < 0 || (size_t)written >= sizeof(header) - offset) {
        close(fd); return -1;
    }
    offset += (size_t)written;

    // Thêm Host header
    written = snprintf(header + offset, sizeof(header) - offset,
                       "Host: %s\r\n", host_line);
    if (written < 0 || (size_t)written >= sizeof(header) - offset) {
        close(fd); return -1;
    }
    offset += (size_t)written;

    // Thêm Connection header, báo cho server đóng kết nối sau khi phản hồi
    written = snprintf(header + offset, sizeof(header) - offset,
                       "Connection: close\r\n");
    if (written < 0 || (size_t)written >= sizeof(header) - offset) {
        close(fd); return -1;
    }
    offset += (size_t)written;

    // Thêm Content-Type header nếu có
    if (content_type && body_len > 0) {
        written = snprintf(header + offset, sizeof(header) - offset,
                           "Content-Type: %s\r\n", content_type);
        if (written < 0 || (size_t)written >= sizeof(header) - offset) {
            close(fd); return -1;
        }
        offset += (size_t)written;
    }

    // Thêm Content-Length header nếu có body
    if (body_len > 0) {
        written = snprintf(header + offset, sizeof(header) - offset,
                           "Content-Length: %zu\r\n", body_len);
        if (written < 0 || (size_t)written >= sizeof(header) - offset) {
            close(fd); return -1;
        }
        offset += (size_t)written;
    }

    // Thêm các header tùy chỉnh khác
    if (extra_headers && extra_headers[0] != '\0') {
        size_t eh_len = strlen(extra_headers);
        if (eh_len >= sizeof(header) - offset) {
            close(fd); return -1;
        }
        memcpy(header + offset, extra_headers, eh_len);
        offset += eh_len;
        // Đảm bảo extra_headers kết thúc bằng \r\n
        if (eh_len < 2 || strncmp(extra_headers + eh_len - 2, "\r\n", 2) != 0) {
            if (offset + 2 >= sizeof(header)) {
                close(fd); return -1;
            }
            header[offset++] = '\r';
            header[offset++] = '\n';
        }
    }

    // Kết thúc phần header bằng một dòng trống
    if (offset + 2 >= sizeof(header)) {
        close(fd); return -1;
    }
    header[offset++] = '\r';
    header[offset++] = '\n';

    // Gửi toàn bộ header
    if (send_all(fd, header, offset) != 0) {
        close(fd); return -1;
    }

    // Gửi body nếu có
    if (body_len > 0 && send_all(fd, body, body_len) != 0) {
        close(fd); return -1;
    }

    // Nhận phản hồi từ server
    ssize_t received = recv_into_buffer(fd, response, response_len);
    close(fd); // Đóng kết nối

    return (received < 0) ? -1 : 0;
}

/**
 * @brief Thực hiện yêu cầu HTTP GET.
 */
int http_get(const char* host, int port, const char* path,
             const char* extra_headers, char* response, size_t response_len) {
    return http_request(host, port, "GET", path, NULL, NULL, 0, extra_headers, response, response_len);
}

/**
 * @brief Thực hiện yêu cầu HTTP POST.
 */
int http_post(const char* host, int port, const char* path,
              const char* content_type, const char* body, size_t body_len,
              const char* extra_headers, char* response, size_t response_len) {
    return http_request(host, port, "POST", path, content_type, body, body_len,
                        extra_headers, response, response_len);
}

/**
 * @brief Thực hiện yêu cầu HTTP PUT.
 */
int http_put(const char* host, int port, const char* path,
             const char* content_type, const char* body, size_t body_len,
             const char* extra_headers, char* response, size_t response_len) {
    return http_request(host, port, "PUT", path, content_type, body, body_len,
                        extra_headers, response, response_len);
}

/**
 * @brief Thực hiện yêu cầu HTTP DELETE.
 */
int http_delete(const char* host, int port, const char* path,
                const char* extra_headers, char* response, size_t response_len) {
    return http_request(host, port, "DELETE", path, NULL, NULL, 0,
                        extra_headers, response, response_len);
}

/**
 * @brief Gửi một file lên server bằng HTTP POST (multipart/form-data).
 * * @param host Tên miền hoặc IP của server.
 * @param port Cổng của server.
 * @param path Đường dẫn API trên server.
 * @param filepath Đường dẫn tới file cần upload.
 * @param field_name Tên của trường form-data chứa file (mặc định là "file").
 * @param file_name Tên file sẽ được gửi đi (mặc định là tên file gốc).
 * @param extra_headers Các header bổ sung.
 * @param response Buffer để lưu phản hồi.
 * @param response_len Kích thước buffer phản hồi.
 * @return 0 nếu thành công, -1 nếu thất bại.
 */
int http_post_file(const char* host, int port, const char* path, const char* filepath,
                   const char* field_name, const char* file_name,
                   const char* extra_headers, char* response, size_t response_len) {
    if (!filepath || !host || !path) {
        return -1;
    }

    if (!field_name || field_name[0] == '\0') {
        field_name = "file"; // Tên trường mặc định
    }

    // Lấy tên file để upload, ưu tiên file_name, nếu không thì lấy từ filepath
    const char* upload_name = file_name && file_name[0] != '\0' ? file_name : basename_ptr(filepath);

    // Lấy thông tin file (kích thước)
    struct stat st;
    if (stat(filepath, &st) != 0) {
        return -1; // Không thể lấy thông tin file
    }
    if (!S_ISREG(st.st_mode)) {
        return -1; // Chỉ hỗ trợ file thông thường
    }

    size_t file_size = (size_t)st.st_size;
    FILE* fp = fopen(filepath, "rb"); // Mở file ở chế độ đọc nhị phân
    if (!fp) {
        return -1;
    }

    // Xây dựng phần đầu của multipart body
    char preamble[1024];
    int preamble_len = snprintf(preamble, sizeof(preamble),
                                "--%s\r\n"
                                "Content-Disposition: form-data; name=\"%s\"; filename=\"%s\"\r\n"
                                "Content-Type: application/octet-stream\r\n\r\n",
                                MULTIPART_BOUNDARY, field_name, upload_name);
    if (preamble_len < 0 || (size_t)preamble_len >= sizeof(preamble)) {
        fclose(fp); return -1;
    }

    // Xây dựng phần cuối của multipart body
    char closing[128];
    int closing_len = snprintf(closing, sizeof(closing),
                               "\r\n--%s--\r\n", MULTIPART_BOUNDARY);
    if (closing_len < 0 || (size_t)closing_len >= sizeof(closing)) {
        fclose(fp); return -1;
    }

    // Cấp phát bộ nhớ cho toàn bộ body (phần đầu + nội dung file + phần cuối)
    size_t total_len = (size_t)preamble_len + file_size + (size_t)closing_len;
    char* body = (char*)malloc(total_len);
    if (!body) {
        fclose(fp); return -1;
    }

    // Sao chép phần đầu vào body
    memcpy(body, preamble, (size_t)preamble_len);
    size_t offset = (size_t)preamble_len;

    // Đọc toàn bộ nội dung file vào body
    size_t read_total = fread(body + offset, 1, file_size, fp);
    fclose(fp); // Đóng file sau khi đọc xong

    if (read_total != file_size) { // Kiểm tra xem có đọc đủ file không
        free(body);
        return -1;
    }

    // Sao chép phần cuối vào body
    memcpy(body + offset + read_total, closing, (size_t)closing_len);

    // Tạo Content-Type header cho yêu cầu multipart
    char content_type[128];
    snprintf(content_type, sizeof(content_type),
             "multipart/form-data; boundary=%s",
             MULTIPART_BOUNDARY);

    // Gọi hàm http_request để gửi đi
    int rc = http_request(host, port, "POST", path, content_type,
                          body, total_len, extra_headers, response, response_len);
    free(body); // Giải phóng bộ nhớ đã cấp phát
    return rc;
}