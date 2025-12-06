# Metadata loader Makefile
CC ?= gcc
CFLAGS = -Wall -O2 -I../query
LDFLAGS = -lrt -lpthread

QUERY_DIR = ../query
QUERY_OBJS = $(QUERY_DIR)/query.o $(QUERY_DIR)/advanced_query.o

TARGET = metadata_loader

all: $(TARGET)

# Build query objects if they don't exist or are out of date
$(QUERY_DIR)/query.o: $(QUERY_DIR)/query.c $(QUERY_DIR)/query.h
	$(CC) $(CFLAGS) -c -o $@ $<

$(QUERY_DIR)/advanced_query.o: $(QUERY_DIR)/advanced_query.c $(QUERY_DIR)/query.h
	$(CC) $(CFLAGS) -c -o $@ $<

$(TARGET): metadata_loader.c $(QUERY_OBJS)
	$(CC) $(CFLAGS) -o $(TARGET) metadata_loader.c $(QUERY_OBJS) $(LDFLAGS)

clean:
	rm -f $(TARGET) *.o $(QUERY_OBJS)

.PHONY: all clean
