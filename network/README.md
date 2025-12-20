# Hướng dẫn build lib network

## Các bước thực hiện:

### Trên Linux

1. **Compile source code thành object file (dùng -fPIC để hỗ trợ cgo):**
   ```bash
   gcc -c -fPIC -o network.o network.c -I.
   ```

2. **Tạo static library từ object file:**
   ```bash
   ar rcs libnetwork.a network.o
   ```

> Hoặc tạo shared library:
> ```bash
> gcc -shared -fPIC -o libnetwork.so network.c -I.
> ```

### Trên Windows

1. **Compile source code thành object file:**
   ```bash
   gcc -c -o network.o network.c -I.
   ```

2. **Tạo static library từ object file:**
   ```bash
   ar rcs libnetwork.a network.o
   ```