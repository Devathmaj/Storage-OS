# Makefile for integrated OSCore storage system

CC = gcc
CFLAGS = -Wall -Wextra -O3 -march=native -flto -std=c11 -D_GNU_SOURCE -I..
LDFLAGS = -lrt -laio

# Directories
QUERY_DIR = ../query
FS_DIR = ../fs

# Source files
INTEGRATED_SRC = oscore_integrated.c
INTEGRATED_OBJ = $(INTEGRATED_SRC:.c=.o)

# Dependencies
QUERY_OBJ = $(QUERY_DIR)/query.o $(QUERY_DIR)/advanced_query.o
FS_OBJ = $(FS_DIR)/fs.o

# Targets
all: liboscore.a test_integrated

# Static library
liboscore.a: $(INTEGRATED_OBJ) $(QUERY_OBJ) $(FS_OBJ)
	ar rcs $@ $^

# Test program
test_integrated: test_integrated.o liboscore.a
	$(CC) $(CFLAGS) -o $@ $< -L. -loscore $(LDFLAGS)

# Compile integrated module
%.o: %.c oscore_integrated.h
	$(CC) $(CFLAGS) -c $< -o $@

# Compile dependencies if needed
$(QUERY_OBJ):
	$(MAKE) -C $(QUERY_DIR)

$(FS_OBJ):
	$(MAKE) -C $(FS_DIR)

clean:
	rm -f *.o liboscore.a test_integrated

test: test_integrated
	./test_integrated

install: liboscore.a
	mkdir -p ../../lib ../../include
	cp liboscore.a ../../lib/
	cp oscore_integrated.h ../../include/
	cp $(QUERY_DIR)/query.h ../../include/
	cp $(FS_DIR)/fs.h ../../include/

.PHONY: all clean test install
