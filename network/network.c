#include "network.h"

// --- Thay đổi cho Windows ---
// Tắt các cảnh báo bảo mật của Microsoft (ví dụ: cho hàm fopen, snprintf)
#define _CRT_SECURE_NO_WARNINGS 
// Tắt cảnh báo về việc sử dụng các hàm Winsock cũ đã bị "deprecated" (không khuyến khích dùng)
#define _WINSOCK_DEPRECATED_NO_WARNINGS 

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

// Chỉ thị cho linker của Visual Studio tự động liên kết (link)
// với thư viện Winsock 2 (ws2_32.lib). Đây là thư viện cốt lõi cho networking.
#pragma comment(lib, "ws2_32.lib")

#include <windows.h> // Header chính của Windows, chứa winsock2.h

// Các include của POSIX đã bị xóa (unistd.h, arpa/inet.h, v.v.)
// vì winsock2.h và ws2tcpip.h đã thay thế chúng.
// --- Kết thúc thay đổi cho Windows ---


#define BACKLOG 16                  // Số lượng kết nối tối đa chờ trong hàng đợi 'listen'
#define DEFAULT_HTTP_PORT 80   // Cổng HTTP mặc định
#define MULTIPART_BOUNDARY "----CGoNetworkBoundary" // Ranh giới cho form multipart (ít dùng)

// --- Hàm mới cho Winsock ---

/**
 * @brief Khởi tạo thư viện Winsock. (BẮT BUỘC cho Windows)
 * @details Phải gọi hàm này một lần trước khi sử dụng bất kỳ hàm socket nào 
 * trên Windows. Nó yêu cầu phiên bản Winsock 2.2 (MAKEWORD(2, 2)).
 * @return 0 nếu khởi tạo thành công, -1 nếu có lỗi.
 */
int network_init(void) {
    WSADATA wsaData; // Cấu trúc để lưu thông tin về Winsock
    // WSAStartup khởi tạo thư viện Winsock
    int result = WSAStartup(MAKEWORD(2, 2), &wsaData);
    if (result != 0) {
        return -1; // Lỗi: Không thể khởi tạo Winsock
    }
    return 0; // Thành công
}

/**
 * @brief Dọn dẹp và giải phóng thư viện Winsock. (BẮT BUỘC cho Windows)
 * @details Phải gọi hàm này khi chương trình kết thúc và không cần 
 * dùng đến socket nữa, để giải phóng tài nguyên mà Winsock đã cấp.
 */
void network_cleanup(void) {
    WSACleanup(); // Hàm dọn dẹp của Winsock
}
// --- Kết thúc hàm mới ---

/**
 * @brief Thiết lập tùy chọn SO_REUSEADDR cho một socket. (Phiên bản Windows)
 * @details Tùy chọn này cho phép socket bind (gắn) vào một địa chỉ/cổng
 * vừa mới được sử dụng xong, tránh lỗi "Address already in use"
 * khi khởi động lại server nhanh.
 * @param fd Socket (kiểu SOCKET của Windows) cần thiết lập.
 * @return 0 nếu thành công, giá trị khác 0 (SOCKET_ERROR) nếu thất bại.
 */
static int set_reuseaddr(SOCKET fd) {
    int opt = 1; // Bật tùy chọn (giá trị 1 là true)
    // Thay đổi: tham số thứ 4 là (const char*) trên Winsock
    return setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, (const char*)&opt, sizeof(opt));
}

/**
 * @brief Lấy thông tin địa chỉ (addrinfo) từ host và port.
 * @details Đây là hàm trợ giúp (wrapper) cho hàm 'getaddrinfo' chuẩn.
 * Nó phân giải tên miền (host) và cổng (port) thành một danh sách 
 * các cấu trúc địa chỉ (struct addrinfo) mà socket có thể sử dụng
 * để 'bind' hoặc 'connect'.
 * @param host Tên host (ví dụ: "example.com") hoặc NULL (cho AI_PASSIVE).
 * @param port Số cổng.
 * @param socktype Loại socket (ví dụ: SOCK_STREAM cho TCP).
 * @param flags Cờ cho 'getaddrinfo' (ví dụ: AI_PASSIVE cho server).
 * @param out Con trỏ (output) để lưu trữ danh sách kết quả addrinfo.
 * @return 0 nếu thành công, mã lỗi (khác 0) nếu thất bại.
 */
static int get_addrinfo_for(const char* host, int port, int socktype, int flags, struct addrinfo** out) {
    struct addrinfo hints; // Cấu trúc "gợi ý" để lọc kết quả
    char port_str[16];
    memset(&hints, 0, sizeof(hints)); 
    hints.ai_family = AF_UNSPEC; // Chấp nhận cả IPv4 (AF_INET) và IPv6 (AF_INET6)
    hints.ai_socktype = socktype; // Loại socket (SOCK_STREAM hoặc SOCK_DGRAM)
    hints.ai_flags = flags; // Cờ (ví dụ: AI_PASSIVE cho server bind)
    snprintf(port_str, sizeof(port_str), "%d", port); // Chuyển cổng (int) sang chuỗi
    // Gọi hàm 'getaddrinfo' chuẩn để thực hiện phân giải
    return getaddrinfo(host, port_str, &hints, out);
}

/**
 * @brief Tạo và bind (gắn) một socket vào host và port cụ thể. (Dùng cho Server)
 * @details Hàm này sẽ:
 * 1. Lấy danh sách địa chỉ (dùng get_addrinfo_for với cờ AI_PASSIVE).
 * 2. Lặp qua danh sách, tạo (socket) và thử bind.
 * 3. Thiết lập SO_REUSEADDR.
 * 4. Trả về socket đầu tiên bind thành công.
 * @param host Host để bind (thường là NULL hoặc "0.0.0.0" để chấp nhận 
 * kết nối từ mọi giao diện mạng).
 * @param port Cổng để bind.
 * @param socktype Loại socket (SOCK_STREAM).
 * @return Một SOCKET hợp lệ nếu thành công, INVALID_SOCKET nếu thất bại.
 */
static SOCKET bind_socket(const char* host, int port, int socktype) {
    struct addrinfo* res = NULL;
    // Lấy thông tin địa chỉ. AI_PASSIVE cho biết ta muốn bind socket này (làm server)
    int rc = get_addrinfo_for(host, port, socktype, AI_PASSIVE, &res);
    if (rc != 0) {
        return INVALID_SOCKET; // Thay -1 bằng INVALID_SOCKET (chuẩn Windows)
    }

    SOCKET fd = INVALID_SOCKET; // Thay int bằng SOCKET, -1 bằng INVALID_SOCKET
    // Lặp qua tất cả các địa chỉ 'getaddrinfo' trả về
    for (struct addrinfo* p = res; p; p = p->ai_next) {
        // Tạo socket
        fd = socket(p->ai_family, p->ai_socktype, p->ai_protocol);
        if (fd == INVALID_SOCKET) { // Thay < 0 bằng == INVALID_SOCKET
            continue; // Thử địa chỉ tiếp theo
        }
        set_reuseaddr(fd); // Cho phép tái sử dụng địa chỉ
        // Thử bind socket vào địa chỉ
        if (bind(fd, p->ai_addr, (int)p->ai_addrlen) == 0) {
            break; // Bind thành công, thoát khỏi vòng lặp
        }
        closesocket(fd); // Thay close() bằng closesocket()
        fd = INVALID_SOCKET; // Đặt lại là INVALID_SOCKET để thử vòng lặp tiếp
    }

    freeaddrinfo(res); // Giải phóng bộ nhớ của danh sách địa chỉ
    return fd; // Trả về socket đã bind thành công, hoặc INVALID_SOCKET
}

/**
 * @brief Tạo và kết nối một socket đến server. (Dùng cho Client)
 * @details Hàm này sẽ:
 * 1. Lấy danh sách địa chỉ của server (dùng get_addrinfo_for).
 * 2. Lặp qua danh sách, tạo (socket) và thử connect.
 * 3. Trả về socket đầu tiên connect thành công.
 * @param host Host (tên miền hoặc IP) của server cần kết nối.
 * @param port Cổng của server.
 * @param socktype Loại socket (SOCK_STREAM).
 * @return Một SOCKET hợp lệ nếu thành công, INVALID_SOCKET nếu thất bại.
 */
static SOCKET connect_socket(const char* host, int port, int socktype) {
    struct addrinfo* res = NULL;
    // Lấy thông tin địa chỉ của server (không có AI_PASSIVE)
    int rc = get_addrinfo_for(host, port, socktype, 0, &res);
    if (rc != 0) {
        return INVALID_SOCKET;
    }

    SOCKET fd = INVALID_SOCKET;
    // Lặp qua các địa chỉ (ví dụ: IPv6 trước, IPv4 sau)
    for (struct addrinfo* p = res; p; p = p->ai_next) {
        fd = socket(p->ai_family, p->ai_socktype, p->ai_protocol);
        if (fd == INVALID_SOCKET) {
            continue;
        }
        // Thử kết nối đến server
        if (connect(fd, p->ai_addr, (int)p->ai_addrlen) == 0) {
            break; // Kết nối thành công
        }
        closesocket(fd); // Kết nối thất bại, đóng socket và thử địa chỉ tiếp
        fd = INVALID_SOCKET;
    }

    freeaddrinfo(res);
    return fd; // Trả về socket đã kết nối, hoặc INVALID_SOCKET
}

/**
 * @brief Gửi TOÀN BỘ dữ liệu trong buffer qua socket.
 * @details Hàm 'send' chuẩn có thể không gửi hết dữ liệu trong một lần gọi
 * (ví dụ: nếu buffer mạng bị đầy). Hàm này đảm bảo gửi hết 'len' 
 * byte bằng cách gọi 'send' trong một vòng lặp cho đến khi tất cả 
 * dữ liệu được gửi hoặc có lỗi xảy ra.
 * @param fd Socket để gửi.
 * @param data Con trỏ tới buffer chứa dữ liệu.
 * @param len Số byte cần gửi.
 * @return 0 nếu gửi thành công toàn bộ, -1 nếu có lỗi.
 */
static int send_all(SOCKET fd, const char* data, size_t len) {
    size_t total = 0; // Tổng số byte đã gửi
    while (total < len) {
        // Thay đổi: send() trên Winsock trả về 'int', không phải 'ssize_t'
        // và tham số độ dài cũng là 'int'.
        int sent = send(fd, data + total, (int)(len - total), 0);
        if (sent < 0) { // Trên Winsock, lỗi trả về SOCKET_ERROR (thường là -1)
            // Thay đổi: dùng WSAGetLastError() thay vì errno để lấy mã lỗi
            if (WSAGetLastError() == WSAEINTR) { // Lỗi bị ngắt (interrupted)
                continue; // Thử lại
            }
            return -1; // Lỗi thực sự
        }
        if (sent == 0) { // Gửi 0 byte (ít khi xảy ra)
            break;
        }
        total += (size_t)sent; // Cộng dồn số byte đã gửi
    }
    return (total == len) ? 0 : -1; // Trả về 0 nếu gửi đủ, -1 nếu không
}

/**
 * @brief Nhận dữ liệu vào buffer cho đến khi buffer gần đầy hoặc kết nối đóng.
 * @details Hàm này liên tục gọi 'recv' để đọc dữ liệu vào 'buffer'.
 * Nó sẽ dừng khi đã đọc (buffer_len - 1) byte (để chừa 1 byte
 * cho ký tự null '\0') hoặc khi 'recv' trả về 0 (kết nối đóng)
 * hoặc lỗi.
 * Hàm này *sẽ* tự động thêm ký tự '\0' vào cuối dữ liệu nhận được.
 * @param fd Socket để nhận.
 * @param buffer Buffer để lưu dữ liệu.
 * @param buffer_len Kích thước tối đa của buffer.
 * @return Số byte thực sự nhận được (kiểu ssize_t), hoặc -1 nếu có lỗi.
 */
static ssize_t recv_into_buffer(SOCKET fd, char* buffer, size_t buffer_len) {
    size_t total = 0; // Tổng số byte đã nhận
    // Vòng lặp cho đến khi buffer gần đầy (chừa 1 byte cho '\0')
    while (total + 1 < buffer_len) {
        // Thay đổi: recv() trên Winsock trả về 'int'
        int received = recv(fd, buffer + total, (int)(buffer_len - 1 - total), 0);
        if (received < 0) { // Lỗi (SOCKET_ERROR)
            // Thay đổi: dùng WSAGetLastError()
            if (WSAGetLastError() == WSAEINTR) {
                continue; // Bị ngắt, thử lại
            }
            return -1; // Lỗi thực sự
        }
        if (received == 0) { // Kết nối đã bị đóng bởi phía bên kia
            break;
        }
        total += (size_t)received; // Cộng dồn
    }
    buffer[total] = '\0'; // Đảm bảo chuỗi kết thúc bằng null
    return (ssize_t)total; // Trả về tổng số byte đã nhận
}

/**
 * @brief Trích xuất tên file (basename) từ một đường dẫn đầy đủ.
 * @details Hàm này xử lý cả hai loại dấu phân cách thư mục:
 * '/' (Linux/POSIX) và '\' (Windows).
 * Nó tìm dấu phân cách cuối cùng và trả về con trỏ đến
 * ký tự ngay sau nó.
 * @param path Đường dẫn đầy đủ (ví dụ: "C:\temp\file.txt" hoặc "/var/log/file.log").
 * @return Con trỏ trỏ đến phần tên file (ví dụ: "file.txt").
 */
static const char* basename_ptr(const char* path) {
    const char* slash = strrchr(path, '/'); // Tìm dấu '/' cuối cùng
    const char* backslash = strrchr(path, '\\'); // Tìm dấu '\' cuối cùng
    
    if (!slash && !backslash) return path; // Không có dấu nào, trả về toàn bộ chuỗi
    if (slash && !backslash) return slash + 1; // Chỉ có '/', trả về sau nó
    if (!slash && backslash) return backslash + 1; // Chỉ có '\', trả về sau nó

    // Cả hai đều tồn tại, trả về cái cuối cùng
    return (slash > backslash ? slash : backslash) + 1;
}

/**
 * @brief Khởi tạo một server TCP (tạo, bind, và listen).
 * @details Đây là hàm tiện ích, kết hợp 'bind_socket' và 'listen'
 * để nhanh chóng tạo một socket server sẵn sàng chấp nhận kết nối.
 * @param host Host để bind (thường là NULL).
 * @param port Cổng để lắng nghe.
 * @return Socket server (đang listen) nếu thành công, INVALID_SOCKET nếu thất bại.
 */
SOCKET tcp_server_start(const char* host, int port) {
    if (port <= 0) {
        return INVALID_SOCKET;
    }
    // Tạo và bind socket
    SOCKET fd = bind_socket(host, port, SOCK_STREAM);
    if (fd == INVALID_SOCKET) {
        return INVALID_SOCKET;
    }
    // Bắt đầu lắng nghe kết nối
    if (listen(fd, BACKLOG) != 0) { // Lỗi (trên Winsock là SOCKET_ERROR)
        closesocket(fd);
        return INVALID_SOCKET;
    }
    return fd; // Trả về socket server
}

/**
 * @brief Chấp nhận một kết nối mới từ client.
 * @details Hàm này sẽ gọi 'accept' trên socket server.
 * Nó là một hàm *blocking* (sẽ tạm dừng chương trình)
 * cho đến khi có một client mới kết nối tới.
 * @param server_fd Socket server (đã gọi 'listen' trước đó).
 * @return Socket của client mới nếu thành công, INVALID_SOCKET nếu thất bại.
 */
SOCKET tcp_accept(SOCKET server_fd) {
    if (server_fd == INVALID_SOCKET) {
        return INVALID_SOCKET;
    }
    // Chấp nhận kết nối (blocking)
    SOCKET client_fd = accept(server_fd, NULL, NULL);
    if (client_fd == INVALID_SOCKET) {
        return INVALID_SOCKET;
    }
    return client_fd; // Trả về socket của client
}

/**
 * @brief Kết nối tới một server TCP.
 * @details Đây là hàm tiện ích cho client, gọi 'connect_socket'.
 * @param host Host (IP/tên miền) của server.
 * @param port Cổng của server.
 * @return Socket đã kết nối tới server nếu thành công, INVALID_SOCKET nếu thất bại.
 */
SOCKET tcp_client_connect(const char* host, int port) {
    if (!host || port <= 0) {
        return INVALID_SOCKET;
    }
    // Gọi hàm trợ giúp để tạo và kết nối
    return connect_socket(host, port, SOCK_STREAM);
}

/**
 * @brief Gửi dữ liệu qua kết nối TCP (đảm bảo gửi hết).
 * @details Hàm này gọi 'send_all' để đảm bảo toàn bộ 'len' byte
 * trong 'buf' được gửi đi.
 * @param fd Socket (client hoặc server) đã kết nối.
 * @param buf Buffer chứa dữ liệu cần gửi.
 * @param len Số byte cần gửi.
 * @return Số byte đã gửi (bằng 'len') nếu thành công, -1 nếu thất bại.
 */
ssize_t tcp_send(SOCKET fd, const char* buf, size_t len) {
    if (fd == INVALID_SOCKET || !buf) {
        return -1;
  }
    // Gọi hàm trợ giúp 'send_all'
    if (send_all(fd, buf, len) != 0) {
        return -1; // Lỗi
    }
    // Trả về số byte đã gửi
    return (ssize_t)len;
}

/**
 * @brief Nhận dữ liệu từ kết nối TCP.
 * @details Hàm này gọi 'recv' một lần. Nó có thể trả về ít hơn 'len'
 * byte, tùy thuộc vào lượng dữ liệu có sẵn trên mạng tại
 * thời điểm gọi.
 * @param fd Socket đã kết nối.
 * @param buf Buffer để lưu dữ liệu nhận được.
 * @param len Kích thước tối đa của 'buf'.
 * @return Số byte thực sự nhận được, 0 nếu client đóng kết nối, -1 nếu có lỗi.
 */
ssize_t tcp_recv(SOCKET fd, char* buf, size_t len) {
    if (fd == INVALID_SOCKET || !buf || len == 0) {
        return -1;
    }
    while (1) {
        // Thay đổi: recv() trả về 'int' và nhận độ dài là 'int'
        int received = recv(fd, buf, (int)len, 0);
        if (received < 0) { // Lỗi (SOCKET_ERROR)
            if (WSAGetLastError() == WSAEINTR) {
                continue; // Bị ngắt, thử lại
            }
            return -1; // Lỗi thực sự
        }
        return (ssize_t)received; // Trả về số byte nhận được (cast sang ssize_t)
    }
}

/**
 * @brief Đóng một kết nối TCP. (Phiên bản Windows)
 * @param fd Socket cần đóng.
 * @return 0 nếu thành công, SOCKET_ERROR (-1) nếu lỗi.
 */
int tcp_close(SOCKET fd) {
    if (fd == INVALID_SOCKET) {
        return -1;
    }
    return closesocket(fd); // Dùng closesocket() thay vì close()
}

// UDP functions removed (unused)

/**
 * @brief Thực hiện một yêu cầu HTTP (GET, POST, v.v.) cơ bản bằng socket.
 * @warning Hàm này không xử lý HTTPS (SSL/TLS).
 * @details Hàm này thực hiện một giao dịch HTTP đầy đủ bằng cách:
 * 1. Kết nối socket TCP đến server.
 * 2. Tự xây dựng (bằng tay) chuỗi header HTTP (Request line, Host, 
 * Connection: close, Content-Type, Content-Length...).
 * 3. Gửi header (dùng send_all).
 * 4. Gửi body (nếu có) (dùng send_all).
 * 5. Nhận toàn bộ phản hồi (header + body) vào 'response' (dùng recv_into_buffer).
 * 6. Đóng socket.
 * @param host Host của server.
 * @param port Cổng (mặc định 80 nếu <= 0).
 * @param method Phương thức HTTP (ví dụ: "GET", "POST").
 * @param path Đường dẫn trên server (ví dụ: "/index.html").
 * @param content_type Loại nội dung (ví dụ: "application/json").
 * @param body Dữ liệu (body) của request.
 * @param body_len Độ dài của body.
 * @param extra_headers Các header bổ sung (ví dụ: "Authorization: Bearer ...\r\n").
 * @param response Buffer (output) để lưu trữ toàn bộ phản hồi.
 * @param response_len Kích thước của buffer 'response'.
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
        port = DEFAULT_HTTP_PORT; // Dùng cổng 80 nếu không chỉ định
    }

    SOCKET fd = connect_socket(host, port, SOCK_STREAM);
    if (fd == INVALID_SOCKET) {
        return -1;
    }

    // Chuẩn bị dòng "Host: host:port"
    char host_line[512];
    if (port == 80 || port == 0) {
        snprintf(host_line, sizeof(host_line), "%s", host);
    } else {
        snprintf(host_line, sizeof(host_line), "%s:%d", host, port);
    }

    char header[4096]; // Buffer để xây dựng header
    size_t offset = 0; // Vị trí hiện tại trong buffer header

    int written = snprintf(header + offset, sizeof(header) - offset,
                           "%s %s HTTP/1.1\r\n", method, path);
    if (written < 0 || (size_t)written >= sizeof(header) - offset) {
        closesocket(fd); return -1;
    }
    offset += (size_t)written;

    // Thêm header "Host" (bắt buộc trong HTTP/1.1)
    written = snprintf(header + offset, sizeof(header) - offset,
                       "Host: %s\r\n", host_line);
    if (written < 0 || (size_t)written >= sizeof(header) - offset) {
        closesocket(fd); return -1;
    }
    offset += (size_t)written;

    // Thêm header "Connection: close" để server tự đóng kết nối sau khi trả lời
    // (vì client socket cơ bản này không xử lý keep-alive)
    written = snprintf(header + offset, sizeof(header) - offset,
                       "Connection: close\r\n");
    if (written < 0 || (size_t)written >= sizeof(header) - offset) {
        closesocket(fd); return -1;
    }
    offset += (size_t)written;

    // Thêm "Content-Type" nếu có body
    if (content_type && body_len > 0) {
        written = snprintf(header + offset, sizeof(header) - offset,
                           "Content-Type: %s\r\n", content_type);
        if (written < 0 || (size_t)written >= sizeof(header) - offset) {
            closesocket(fd); return -1;
        }
        offset += (size_t)written;
    }

    // Thêm "Content-Length" nếu có body
    if (body_len > 0) {
        written = snprintf(header + offset, sizeof(header) - offset,
                           "Content-Length: %zu\r\n", body_len);
        if (written < 0 || (size_t)written >= sizeof(header) - offset) {
            closesocket(fd); return -1;
        }
        offset += (size_t)written;
    }

    // Thêm các header tùy chọn (extra_headers)
    if (extra_headers && extra_headers[0] != '\0') {
        size_t eh_len = strlen(extra_headers);
        if (eh_len >= sizeof(header) - offset) {
            closesocket(fd); return -1; // Không đủ chỗ
        }
        memcpy(header + offset, extra_headers, eh_len);
        offset += eh_len;
        // Đảm bảo extra_headers kết thúc bằng \r\n
        if (eh_len < 2 || strncmp(extra_headers + eh_len - 2, "\r\n", 2) != 0) {
            if (offset + 2 >= sizeof(header)) {
                closesocket(fd); return -1;
            }
            header[offset++] = '\r';
            header[offset++] = '\n';
        }
    }

    // Thêm dòng trống (\r\n) cuối cùng để kết thúc header
    if (offset + 2 >= sizeof(header)) {
        closesocket(fd); return -1;
    }
    header[offset++] = '\r';
    header[offset++] = '\n';

    // --- Gửi Request ---
    // 1. Gửi toàn bộ header
    if (send_all(fd, header, offset) != 0) {
        closesocket(fd); return -1;
    }

    // 2. Gửi body (nếu có)
    if (body_len > 0 && send_all(fd, body, body_len) != 0) {
        closesocket(fd); return -1;
    }

    // --- Nhận Response ---
    // Nhận toàn bộ phản hồi vào buffer 'response'
    ssize_t received = recv_into_buffer(fd, response, response_len);
    closesocket(fd); // Đóng kết nối

    return (received < 0) ? -1 : 0; // Trả về 0 nếu nhận thành công (dù chỉ là 0 byte)
}

