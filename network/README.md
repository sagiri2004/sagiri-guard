# Network module
Thư viện C thuần + binding Go để truyền TCP protocol nhị phân. Dùng cho backend/agent giao tiếp.

## Build library
### Linux
1) Compile object (cgo cần -fPIC):
```bash
gcc -c -fPIC -o network.o network.c -I.
```
2) Static lib:
```bash
ar rcs libnetwork.a network.o
```
Hoặc shared lib:
```bash
gcc -shared -fPIC -o libnetwork.so network.c -I.
```

### Windows (MinGW)
```bash
gcc -c -o network.o network.c -I.
ar rcs libnetwork.a network.o
```

## Protocol format (protocol.h)
Header: 1 byte `type`, 4 byte `length_be` (payload). Giới hạn payload ≤ 1MB.

| Type | Mã hex | Ý nghĩa | Payload format |
| --- | --- | --- | --- |
| MSG_LOGIN | 0x01 | Agent→Backend đăng nhập kèm token | `[dev_len:u8][token_len:u16][device_id][token]` |
| MSG_COMMAND | 0x02 | Sub-command JSON | `raw JSON bytes` |
| MSG_FILE_META | 0x03 | Thông tin file | `[name_len:u16][file_size:u64][file_name]` |
| MSG_FILE_CHUNK | 0x04 | Chunk dữ liệu file | `[sid_len:u8][tok_len:u8][session_id][token][offset:u32][len:u32][chunk]` |
| MSG_FILE_DONE | 0x05 | Kết thúc truyền file | `[sid_len:u8][tok_len:u8][session_id][token]` |
| MSG_ACK | 0x06 | ACK/ERROR | `[status_code:u16][msg_len:u16][msg]` |
| MSG_ERROR | 0x7F | Lỗi | Như ACK |

Ràng buộc: `device_id ≤255`, `token ≤1024`, `session_id ≤128`, `filename ≤512`, `status_msg ≤1024`.

### Cách dựng payload chi tiết
- **LOGIN (0x01)**  
  - `dev_len`: độ dài device_id (u8)  
  - `token_len`: độ dài token (u16 big-endian)  
  - Tiếp theo `device_id` (ASCII/UTF-8 raw), rồi `token`.  
  - Ví dụ device="dev", token="tok": payload hex `03 00 03 64 65 76 74 6f 6b`.

- **COMMAND (0x02)**  
  - Payload là JSON thuần (UTF-8).  
  - Ví dụ: `{"action":"login","data":{"username":"u","password":"p"}}` → marshal JSON → frame.

- **FILE_META (0x03)**  
  - `name_len:u16 BE`, `file_size:u64 BE`, `file_name` bytes.  
  - Dùng để thông báo tên & kích thước trước khi gửi chunk.

- **FILE_CHUNK (0x04)**  
  - `sid_len:u8`, `tok_len:u8`, `session_id`, `token`, `offset:u32 BE`, `len:u32 BE`, `chunk data`.  
  - `offset` là byte offset trong file; `len` là độ dài chunk.

- **FILE_DONE (0x05)**  
  - `sid_len:u8`, `tok_len:u8`, `session_id`, `token`.  
  - Không có dữ liệu bổ sung.

- **ACK / ERROR (0x06 / 0x7F)**  
  - `status_code:u16 BE`, `msg_len:u16 BE`, `msg` (UTF-8).  
  - `msg` có thể chứa JSON (ví dụ payload response) hoặc chuỗi lỗi.

### Cách đóng gói frame
1) Xây payload theo type (ở trên).  
2) Header: `type` (1 byte), `length_be` = payload length (u32 BE).  
3) Gửi header trước, rồi payload.

### Cách parse frame trả về
1) Đọc 5 byte đầu: type, length.  
2) Đọc đủ `length` byte payload.  
3) Giải mã theo type:  
   - LOGIN: tách `dev_len`, `token_len`, đọc chuỗi.  
   - COMMAND: payload JSON thô.  
   - FILE_*: tách độ dài, offset, len, chunk...  
   - ACK/ERROR: lấy `status_code`, `status_msg`.  

### JSON sub-command (MsgCommand) thường dùng
- `login`: `{"action":"login","data":{...}}` (payload tùy backend).  
- `device_register`: device info.  
- `filetree_sync`, `agent_log`, `backup_init_upload`, `backup_init_download`, `backup_download_start`, ...
Payload của `MSG_COMMAND` luôn là JSON, còn ACK trả về có thể là JSON chuỗi trong `status_msg`.

## C API chính (network.h / network.c)
### TCP helpers
- `tcp_server_start`, `tcp_client_connect`, `tcp_accept`, `tcp_send`, `tcp_recv`, `tcp_close`.
- Threaded server: `tcp_server_create(host, port, handler, user_data, &server)`, `tcp_server_stop/destroy`.

### Protocol send
- `protocol_send_login(fd, device_id, token)`
- `protocol_send_command(fd, json, json_len)`
- File: `protocol_send_file_meta`, `protocol_send_file_chunk`, `protocol_send_file_done`
- `protocol_send_ack(fd, status_code, msg)`

### Protocol receive
- `protocol_recv_message(fd, protocol_message_t* msg)`:
  - Parse theo type, điền sẵn các trường: `device_id`, `token`, `session_id`, `file_name`, `file_size`, `chunk_offset`, `chunk_len`, `status_code`, `status_msg`. Với `MSG_COMMAND`, dữ liệu giữ ở `data`.
- `protocol_message_free` giải phóng `data`.
- `protocol_message_get_type` trả về type.

### C server (multi-client)
- `protocol_server_create(host, port, on_message, user_data, &srv)`: chạy TCP server, mỗi client thread đọc frame, giữ `last_device` từ login để gán cho frame sau nếu thiếu device_id, rồi gọi `on_message`.
- `protocol_server_stop`, `protocol_server_destroy`.

## Go binding (network.go)
### Khởi tạo
- `network.Init()` / `network.Cleanup()` bao cgo init/cleanup.

### Client
- `DialTCP(host, port) (*TCPClient)`; methods:
  - `SendLogin(deviceID, token)`
  - `SendCommand(jsonPayload)`
  - `SendFileMeta`, `SendFileChunk`, `SendFileChunkWithSession`, `SendFileDone`, `SendFileDoneWithSession`
  - `SendAck`
  - `RecvProtocolMessage()` (gọi C, decode sang `ProtocolMessage`)
  - `Close`, `Write`, `Read`, `ReadFull`

### ProtocolMessage (Go)
- Trường: `Type`, `Raw`, `DeviceID`, `Token`, `SessionID`, `CommandJSON`, `FileName`, `FileSize`, `ChunkOffset`, `ChunkLen`, `ChunkData`, `StatusCode`, `StatusMsg`.
- `convertProtocolMessage` map từ struct C; với `MSG_COMMAND` gán `CommandJSON = Raw`, `MSG_FILE_CHUNK` trích `ChunkData`.

### Server
- `ListenProtocol(host, port, handler)` dựng C server, bridge callback `handler(client *TCPClient, msg *ProtocolMessage)` từ thread C.

## Luồng sử dụng (backend/agent)
1) Agent mở TCP, có thể gửi `MSG_LOGIN` (device_id + token) và/hoặc `MSG_COMMAND` chứa JSON sub-command (login, device_register, ...).
2) Backend Go nhận qua `ListenProtocol` → `ProtocolController.HandleMessage`.
3) File transfer dùng `MSG_FILE_META` → `MSG_FILE_CHUNK` → `MSG_FILE_DONE`.
4) Phản hồi dùng `MSG_ACK` (status_code + message JSON hoặc chuỗi).

## Phản hồi từ backend (định dạng theo type)
- Với các sub-command (gửi bằng `MSG_COMMAND`):
  - Backend trả `MSG_ACK`:
    - `status_code`: mã HTTP-like (200, 400, 401, 500...).
    - `status_msg`: chuỗi UTF-8. Nếu có payload JSON, backend sẽ marshal JSON vào đây (chuỗi JSON), ví dụ:  
      - Login thành công: `{"token":"...","device_id":"..."}`.  
      - Backup init: `{"session_id":"...","token":"...","file_size":...}`.  
      - Trường hợp lỗi: chuỗi mô tả lỗi.
- Với upload file (agent→backend):
  - Sau `backup_init_upload` (ACK 200 kèm JSON), agent gửi `MSG_FILE_CHUNK`/`MSG_FILE_DONE`. Backend thường không trả từng chunk; chỉ lỗi nếu có (có thể bằng `MSG_ERROR`/`MSG_ACK` tùy triển khai). Hiện tại controller không gửi phản hồi chunk.
- Với download file (backend→agent):
  - Agent gửi `backup_download_start` (COMMAND), backend trả `MSG_ACK` (200 hoặc lỗi). Sau đó backend gửi `MSG_FILE_META` + nhiều `MSG_FILE_CHUNK` + `MSG_FILE_DONE`. Không có payload JSON trong các frame này; dữ liệu nhị phân nằm trong chunk.
- Ping: `action:"ping"` → backend trả `MSG_ACK` code 200, msg "pong".

## Ví dụ nhị phân (tóm lược)
- LOGIN với device_id="dev", token="tok":
  - dev_len=3 (0x03), token_len=3 (0x0003)
  - Payload hex: `03 00 03 64 65 76 74 6f 6b` (dev bytes `64 65 76`, token bytes `74 6f 6b`)
  - Frame = header `01 00 00 00 09` + payload trên.

## Ghi chú build & cgo
- Thư viện cần `libnetwork.a` (hoặc .so) trong thư mục `network/` cho cgo flag: `#cgo LDFLAGS: -L${SRCDIR} -lnetwork`.
- Compile với `-fPIC` trên Linux để dùng static lib với cgo.***