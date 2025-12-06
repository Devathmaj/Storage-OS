#!/bin/bash
#
# Diagnose HTTP server in QEMU VM
#

echo "======================================"
echo "HTTP Server Diagnostics"
echo "======================================"
echo ""

echo "[1] Checking if server binary exists..."
if [ -f /usr/sbin/storageos-httpd ]; then
    echo "✓ Binary found: /usr/sbin/storageos-httpd"
    ls -lh /usr/sbin/storageos-httpd
else
    echo "✗ Binary NOT found!"
    exit 1
fi

echo ""
echo "[2] Checking init script..."
if [ -f /etc/init.d/S95httpd ]; then
    echo "✓ Init script found: /etc/init.d/S95httpd"
    ls -lh /etc/init.d/S95httpd
else
    echo "✗ Init script NOT found!"
fi

echo ""
echo "[3] Checking if server is running..."
if pgrep -f storageos-httpd > /dev/null; then
    echo "✓ Server process found:"
    ps aux | grep storageos-httpd | grep -v grep
else
    echo "✗ Server NOT running!"
fi

echo ""
echo "[4] Checking network interface..."
echo "Network interfaces:"
ifconfig 2>/dev/null || ip addr show

echo ""
echo "[5] Checking listening ports..."
echo "Listening on port 8080:"
netstat -tuln 2>/dev/null | grep 8080 || ss -tuln 2>/dev/null | grep 8080

echo ""
echo "[6] Testing local connection..."
if command -v wget > /dev/null; then
    echo "Using wget:"
    wget -O- http://localhost:8080/health 2>&1 | head -5
elif command -v curl > /dev/null; then
    echo "Using curl:"
    curl -v http://localhost:8080/health 2>&1 | head -10
else
    echo "⚠ No wget or curl available"
fi

echo ""
echo "[7] Checking PID file..."
if [ -f /var/run/storageos-httpd.pid ]; then
    echo "PID file exists:"
    cat /var/run/storageos-httpd.pid
else
    echo "No PID file found"
fi

echo ""
echo "======================================"
echo "Diagnostics Complete"
echo "======================================"
