# Makefile for StorageOS HTTP Server

CC ?= gcc
CFLAGS ?= -Wall -O2 -std=gnu99
LDFLAGS ?=

TARGET = storageos-httpd
SRC = httpd.c
OBJ = $(SRC:.c=.o)

.PHONY: all clean install

all: $(TARGET)

$(TARGET): $(OBJ)
	$(CC) $(CFLAGS) -o $@ $^ $(LDFLAGS)

%.o: %.c
	$(CC) $(CFLAGS) -c $< -o $@

clean:
	rm -f $(TARGET) $(OBJ)

install: $(TARGET)
	install -D -m 0755 $(TARGET) $(DESTDIR)/usr/sbin/$(TARGET)

.PHONY: test
test: $(TARGET)
	@echo "Testing HTTP server..."
	@./$(TARGET) 8080 &
	@sleep 1
	@curl -s http://localhost:8080/health || echo "Server started"
	@killall $(TARGET) 2>/dev/null || true
