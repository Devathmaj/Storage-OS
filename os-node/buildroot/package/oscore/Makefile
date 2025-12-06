# Master Makefile for OSCore - Optimized Storage & Query System

.PHONY: all clean test install fs query integrated

all: fs query integrated

# Build filesystem layer
fs:
	@echo "Building filesystem layer..."
	$(MAKE) -C fs

# Build query engine
query:
	@echo "Building query engine..."
	$(MAKE) -C query

# Build integrated system (depends on fs and query)
integrated: fs query
	@echo "Building integrated storage system..."
	$(MAKE) -C integrated

# Run all tests
test: all
	@echo ""
	@echo "╔═══════════════════════════════════════════════════════════╗"
	@echo "║           Running OSCore Test Suite                      ║"
	@echo "╚═══════════════════════════════════════════════════════════╝"
	@echo ""
	@echo "=== Filesystem Tests ==="
	cd fs && ./test_fs || true
	@echo ""
	@echo "=== Query Engine Tests ==="
	cd query && ./test_query || true
	@echo ""
	@echo "=== Integrated System Tests ==="
	cd integrated && ./test_integrated || true
	@echo ""
	@echo "╔═══════════════════════════════════════════════════════════╗"
	@echo "║           All Tests Completed                             ║"
	@echo "╚═══════════════════════════════════════════════════════════╝"

# Run benchmarks
benchmark: all
	@echo ""
	@echo "╔═══════════════════════════════════════════════════════════╗"
	@echo "║           Running OSCore Benchmarks                       ║"
	@echo "╚═══════════════════════════════════════════════════════════╝"
	@echo ""
	@echo "=== Filesystem Benchmarks ==="
	cd fs && ./test_fs
	@echo ""
	@echo "=== Query Engine Benchmarks ==="
	cd query && ./benchmark
	@echo ""
	@echo "=== Integrated System Benchmarks ==="
	cd integrated && ./test_integrated

# Clean all
clean:
	@echo "Cleaning all components..."
	$(MAKE) -C fs clean
	$(MAKE) -C query clean
	$(MAKE) -C integrated clean
	@echo "✓ Clean complete"

# Install libraries and headers
install: all
	@echo "Installing OSCore libraries and headers..."
	mkdir -p lib include
	$(MAKE) -C fs install
	$(MAKE) -C query install
	$(MAKE) -C integrated install
	@echo "✓ Installation complete"
	@echo ""
	@echo "Libraries installed in: ./lib/"
	@echo "Headers installed in: ./include/"

# Help target
help:
	@echo "OSCore Build System"
	@echo ""
	@echo "Targets:"
	@echo "  all        - Build all components (default)"
	@echo "  fs         - Build filesystem layer only"
	@echo "  query      - Build query engine only"
	@echo "  integrated - Build integrated system"
	@echo "  test       - Build and run all tests"
	@echo "  benchmark  - Run performance benchmarks"
	@echo "  clean      - Remove all build artifacts"
	@echo "  install    - Install libraries and headers"
	@echo "  help       - Show this help message"
	@echo ""
	@echo "Usage:"
	@echo "  make          # Build everything"
	@echo "  make test     # Run tests"
	@echo "  make clean    # Clean build"
	@echo "  make install  # Install to lib/ and include/"
