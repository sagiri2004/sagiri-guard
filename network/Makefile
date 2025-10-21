CC = gcc
CFLAGS = -O2 -Wall -fPIC
LDFLAGS = -shared
TARGET = libnetwork.so
SRCS = network.c
OBJS = $(SRCS:.c=.o)

.PHONY: all clean

all: $(TARGET)

$(TARGET): $(OBJS)
	$(CC) $(LDFLAGS) -o $@ $^

%.o: %.c network.h
	$(CC) $(CFLAGS) -c $< -o $@

clean:
	rm -f $(OBJS) $(TARGET)
