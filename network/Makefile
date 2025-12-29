# network/Makefile
CC = gcc
CFLAGS = -c -fPIC -Wall -Wextra -O2 -I.
LDFLAGS = -shared -fPIC -lpthread

SOURCES = tcp_core.c tcp_server.c protocol_message.c protocol_server.c
OBJECTS = $(SOURCES:.c=.o)

all: libnetwork.a libnetwork.so

%.o: %.c
	$(CC) $(CFLAGS) -o $@ $<

libnetwork.a: $(OBJECTS)
	ar rcs $@ $(OBJECTS)

libnetwork.so: $(SOURCES)
	$(CC) $(LDFLAGS) -o $@ $(SOURCES)

clean:
	rm -f *.o libnetwork.a libnetwork.so

.PHONY: all clean
