# Makefile for optimized filesystem operations

CC ?= gcc
CFLAGS = -Wall -Wextra -O3 -march=x86-64 -flto -std=c11 -D_GNU_SOURCE -fPIC
LDFLAGS = -lrt
# Optional: add -laio if libaio is available for async I/O support

# Source files
FS_SRC = fs.c
FS_OBJ = $(FS_SRC:.c=.o)

# Targets
all: libfs.a libfs.so libfs_preload.so fastcp test_preload

# Static library for linking with other components
libfs.a: $(FS_OBJ)
	ar rcs $@ $^

# Shared library for CGO/dynamic linking
libfs.so: $(FS_OBJ)
	$(CC) -shared -o $@ $^ $(LDFLAGS)

# LD_PRELOAD wrapper library for intercepting standard file operations
libfs_preload.so: fs_preload.o fs.o
	$(CC) -shared -o $@ $^ $(LDFLAGS) -ldl

# Fast copy command-line tool
fastcp: fastcp.o libfs.a
	$(CC) $(CFLAGS) -o $@ $^ $(LDFLAGS)

# Test program
test_fs: test_fs.o libfs.a
	$(CC) $(CFLAGS) -o $@ $^ $(LDFLAGS)

# Preload test program - tests LD_PRELOAD interception
test_preload: test_preload.o
	$(CC) $(CFLAGS) -o $@ $^

%.o: %.c fs.h
	$(CC) $(CFLAGS) -c $< -o $@

clean:
	rm -f *.o libfs.a libfs.so libfs_preload.so test_fs fastcp test_preload

install: libfs.a libfs.so libfs_preload.so fastcp test_preload
	mkdir -p ../../lib ../../include ../../bin
	cp libfs.a libfs.so libfs_preload.so ../../lib/
	cp fs.h ../../include/
	cp fastcp test_preload ../../bin/

.PHONY: all clean install
