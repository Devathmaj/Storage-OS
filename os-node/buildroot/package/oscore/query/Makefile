# Makefile for optimized query system

CC = gcc
CFLAGS = -Wall -Wextra -O3 -march=native -flto -std=c11 -D_GNU_SOURCE
CFLAGS_SHARED = -fPIC $(CFLAGS)
LDFLAGS = -lrt -lm

# Build directory
BUILD_DIR = build

# Source files
QUERY_SRC = query.c advanced_query.c
QUERY_OBJ = $(QUERY_SRC:.c=.o)
QUERY_OBJ_SHARED = $(BUILD_DIR)/query_shared.o $(BUILD_DIR)/advanced_query_shared.o

# Targets
all: test_query benchmark libquery.a libquery.so

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

# Shared library (for Go CGO)
libquery.so: $(BUILD_DIR) $(QUERY_OBJ_SHARED)
	$(CC) -shared $(QUERY_OBJ_SHARED) -o $(BUILD_DIR)/$@ $(LDFLAGS)

$(BUILD_DIR)/%_shared.o: %.c query.h khash.h
	$(CC) $(CFLAGS_SHARED) -c $< -o $@

# Static library
libquery.a: $(QUERY_OBJ)
	ar rcs $@ $^

test_query: test_query.o $(QUERY_OBJ)
	$(CC) $(CFLAGS) -o $@ $^ $(LDFLAGS)

benchmark: benchmark.o $(QUERY_OBJ)
	$(CC) $(CFLAGS) -o $@ $^ $(LDFLAGS)

%.o: %.c query.h khash.h
	$(CC) $(CFLAGS) -c $< -o $@

clean:
	rm -f *.o test_query benchmark libquery.a
	rm -rf $(BUILD_DIR)

run_test: test_query
	./test_query

run_benchmark: benchmark
	./benchmark

install: libquery.a libquery.so
	mkdir -p ../../lib ../../include
	cp libquery.a ../../lib/
	cp $(BUILD_DIR)/libquery.so ../../lib/
	cp query.h ../../include/
	cp khash.h ../../include/

# Help for Go CGO integration
go-help:
	@echo "To use libquery in Go with CGO:"
	@echo ""
	@echo "export CGO_CFLAGS=\"-I$(PWD)\""
	@echo "export CGO_LDFLAGS=\"-L$(PWD)/$(BUILD_DIR) -lquery -lm\""
	@echo "export LD_LIBRARY_PATH=\"$(PWD)/$(BUILD_DIR):\$$LD_LIBRARY_PATH\""
	@echo ""
	@echo "Then in your Go file:"
	@echo "  /*"
	@echo "  #cgo CFLAGS: -I$(PWD)"
	@echo "  #cgo LDFLAGS: -L$(PWD)/$(BUILD_DIR) -lquery -lm"
	@echo "  #include \"query.h\""
	@echo "  */"
	@echo "  import \"C\""

.PHONY: all clean run_test run_benchmark install go-help
