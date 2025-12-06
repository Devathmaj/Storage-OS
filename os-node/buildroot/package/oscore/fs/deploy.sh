#!/bin/bash
# Quick deployment script for testing on target system

TARGET_IP="${1:-localhost}"
TARGET_USER="${2:-root}"

if [ "$TARGET_IP" = "localhost" ]; then
    echo "Installing locally..."
    
    # Build
    make clean && make all
    
    # Install
    sudo cp libfs.so /usr/lib/
    sudo cp libfs_preload.so /usr/lib/
    sudo cp test_preload /usr/bin/
    sudo cp S99fsoptimize /etc/init.d/
    sudo chmod +x /etc/init.d/S99fsoptimize
    
    # Enable
    sudo /etc/init.d/S99fsoptimize start
    
    echo "Installation complete. Run 'test_preload' to verify."
else
    echo "Deploying to $TARGET_USER@$TARGET_IP..."
    
    # Build locally
    make clean && make all
    
    # Copy to target
    scp libfs.so libfs_preload.so test_preload "$TARGET_USER@$TARGET_IP:/tmp/"
    scp S99fsoptimize "$TARGET_USER@$TARGET_IP:/tmp/"
    
    # Install on target
    ssh "$TARGET_USER@$TARGET_IP" << 'EOF'
        sudo cp /tmp/libfs.so /usr/lib/
        sudo cp /tmp/libfs_preload.so /usr/lib/
        sudo cp /tmp/test_preload /usr/bin/
        sudo cp /tmp/S99fsoptimize /etc/init.d/
        sudo chmod +x /etc/init.d/S99fsoptimize
        sudo /etc/init.d/S99fsoptimize start
        echo "Installation complete. Run 'test_preload' to verify."
EOF
fi
