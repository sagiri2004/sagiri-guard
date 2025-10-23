# Hướng dẫn build lib trên Windows

## Các bước thực hiện:

1. **Compile source code thành object file:**
   ```bash
   gcc -c -o network.o network.c -I.
   ```

2. **Tạo static library từ object file:**
   ```bash
   ar rcs libnetwork.a network.o
   ```