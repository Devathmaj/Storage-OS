#!/bin/sh
#
# Initialize storage services after login
# This script is designed to run once after the first root login
#

INIT_FLAG="/var/lib/storage-services-initialized"
LOG_FILE="/var/log/storage-init.log"

# Redirect output to log file
exec >> "$LOG_FILE" 2>&1

echo "=== Storage Services Initialization ==="
echo "Started at: $(date)"

# Check if already initialized
if [ -f "$INIT_FLAG" ]; then
    echo "Storage services already initialized"
    exit 0
fi

# Ensure we're running as root
if [ "$(id -u)" -ne 0 ]; then
    echo "ERROR: This script must be run as root"
    exit 1
fi

# Wait for storage to be mounted
echo "Waiting for storage to be mounted..."
RETRIES=30
while [ $RETRIES -gt 0 ]; do
    if mountpoint -q /data; then
        echo "Storage mounted successfully"
        break
    fi
    sleep 1
    RETRIES=$((RETRIES - 1))
done

if ! mountpoint -q /data; then
    echo "ERROR: Storage not mounted at /data"
    exit 1
fi

# Start OS Node server
echo ""
echo "Starting OS Node server..."
if [ -x /usr/local/bin/start-osnode ]; then
    /usr/local/bin/start-osnode start
    if [ $? -eq 0 ]; then
        echo "✓ OS Node server started"
    else
        echo "✗ Failed to start OS Node server"
    fi
else
    echo "⚠ OS Node start script not found"
fi

# Wait a moment for OS Node to initialize
sleep 2

# Start metadata loader
echo ""
echo "Starting metadata loader..."
if [ -x /usr/local/bin/start-metadata ]; then
    /usr/local/bin/start-metadata start
    if [ $? -eq 0 ]; then
        echo "✓ Metadata loader started"
    else
        echo "✗ Failed to start metadata loader"
    fi
else
    echo "⚠ Metadata loader start script not found"
fi

# Wait a moment for metadata loader to initialize
sleep 2

# Enable filesystem optimizations
echo ""
echo "Enabling filesystem optimizations..."
if [ -x /usr/local/bin/start-fsoptimize ]; then
    /usr/local/bin/start-fsoptimize start
    if [ $? -eq 0 ]; then
        echo "✓ Filesystem optimizations enabled"
    else
        echo "✗ Failed to enable filesystem optimizations"
    fi
else
    echo "⚠ Filesystem optimization start script not found"
fi

# Create initialization flag
mkdir -p /var/lib
touch "$INIT_FLAG"

echo ""
echo "=== Storage Services Initialization Complete ==="
echo "Completed at: $(date)"
echo ""
echo "Service Status:"
echo "---------------"

# Show status of all services
if [ -x /usr/local/bin/start-osnode ]; then
    /usr/local/bin/start-osnode status
fi

echo ""
if [ -x /usr/local/bin/start-metadata ]; then
    /usr/local/bin/start-metadata status
fi

echo ""
echo "Log file: $LOG_FILE"
echo ""

exit 0
