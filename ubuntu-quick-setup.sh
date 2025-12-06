#!/bin/bash

#############################################################################
# StorageOS Node - Ubuntu Quick Setup Script
#############################################################################
# This script automates the entire OS storage node setup process:
# 1. Installs dependencies (buildroot, qemu, etc.)
# 2. Builds the custom OS from source using Buildroot
# 3. Packages the OS node distribution
# 4. Installs and starts the systemd service
#
# Run as root or with sudo privileges
#############################################################################

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILDROOT_DIR="$SCRIPT_DIR/os-node/buildroot"
PACKAGE_SCRIPT="$SCRIPT_DIR/package_os_node.sh"

#############################################################################
# Helper Functions
#############################################################################

print_header() {
    echo -e "\n${BLUE}============================================================${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}============================================================${NC}\n"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠ $1${NC}"
}

print_info() {
    echo -e "${BLUE}ℹ $1${NC}"
}

check_root() {
    if [[ $EUID -ne 0 ]]; then
        print_error "This script must be run as root or with sudo"
        echo "Usage: sudo $0"
        exit 1
    fi
}

#############################################################################
# Step 1: Install Dependencies
#############################################################################

install_dependencies() {
    print_header "Step 1: Installing Dependencies"
    
    print_info "Updating package lists..."
    apt-get update -qq
    
    print_info "Installing build tools..."
    apt-get install -y -qq \
        build-essential \
        gcc \
        g++ \
        make \
        binutils \
        patch \
        gzip \
        bzip2 \
        perl \
        tar \
        cpio \
        unzip \
        rsync \
        file \
        bc \
        wget \
        git \
        libncurses5-dev \
        libssl-dev \
        python3 \
        qemu-system-x86 \
        qemu-utils
    
    print_success "Dependencies installed"
}

#############################################################################
# Step 2: Build OS Node with Buildroot
#############################################################################

build_os_node() {
    print_header "Step 2: Building OS Storage Node"
    
    if [[ ! -d "$BUILDROOT_DIR" ]]; then
        print_error "Buildroot directory not found: $BUILDROOT_DIR"
        print_info "Expected structure: os-node/buildroot/"
        exit 1
    fi
    
    cd "$BUILDROOT_DIR"
    
    # Check if config exists
    if [[ ! -f ".config" ]] && [[ ! -f "configs/storage_os_defconfig" ]]; then
        print_error "No Buildroot configuration found"
        print_info "Please configure Buildroot first:"
        print_info "  cd $BUILDROOT_DIR"
        print_info "  make menuconfig"
        exit 1
    fi
    
    # Load config if it exists
    if [[ -f "configs/storage_os_defconfig" ]]; then
        print_info "Loading storage_os_defconfig..."
        make storage_os_defconfig
    fi
    
    print_info "Starting Buildroot compilation (this may take 30-60 minutes)..."
    print_warning "Go grab a coffee ☕ - this will take a while!"
    
    # Build with all CPU cores
    NPROC=$(nproc)
    print_info "Building with $NPROC parallel jobs..."
    
    make -j"$NPROC" 2>&1 | tee "$SCRIPT_DIR/buildroot-build.log"
    
    # Check if build succeeded
    if [[ -f "output/images/bzImage" ]] && [[ -f "output/images/rootfs.ext2" ]]; then
        print_success "OS node built successfully!"
        print_info "  Kernel: output/images/bzImage"
        print_info "  Rootfs: output/images/rootfs.ext2"
    else
        print_error "Build failed - missing output files"
        print_info "Check build log: $SCRIPT_DIR/buildroot-build.log"
        exit 1
    fi
}

#############################################################################
# Step 3: Package OS Node Distribution
#############################################################################

package_os_node() {
    print_header "Step 3: Packaging OS Node Distribution"
    
    cd "$SCRIPT_DIR"
    
    if [[ ! -f "$PACKAGE_SCRIPT" ]]; then
        print_error "Package script not found: $PACKAGE_SCRIPT"
        exit 1
    fi
    
    # Make executable
    chmod +x "$PACKAGE_SCRIPT"
    
    print_info "Creating distribution package..."
    bash "$PACKAGE_SCRIPT"
    
    if [[ -f "dist/os-storage-node-1.0.0.tar.gz" ]]; then
        print_success "Distribution package created: dist/os-storage-node-1.0.0.tar.gz"
    else
        print_error "Packaging failed"
        exit 1
    fi
}

#############################################################################
# Step 4: Install OS Node Service
#############################################################################

install_os_node() {
    print_header "Step 4: Installing OS Node Service"
    
    cd "$SCRIPT_DIR"
    
    # Extract package
    print_info "Extracting package..."
    tar -xzf dist/os-storage-node-1.0.0.tar.gz -C /tmp/
    
    cd /tmp/os-storage-node
    
    # Run installer
    print_info "Running installer..."
    bash install.sh
    
    # Clean up
    rm -rf /tmp/os-storage-node
    
    print_success "OS node installed to /opt/os-storage-node"
}

#############################################################################
# Step 5: Start Service
#############################################################################

start_service() {
    print_header "Step 5: Starting OS Node Service"
    
    print_info "Enabling service for auto-start..."
    systemctl enable os-storage-node
    
    print_info "Starting service..."
    systemctl start os-storage-node
    
    sleep 3
    
    # Check status
    if systemctl is-active --quiet os-storage-node; then
        print_success "OS storage node is running!"
    else
        print_warning "Service may not be running - check status:"
        systemctl status os-storage-node --no-pager
    fi
}

#############################################################################
# Summary
#############################################################################

print_summary() {
    print_header "Installation Complete! 🎉"
    
    echo -e "The OS storage node has been built and installed.\n"
    
    echo -e "${GREEN}Management Commands:${NC}"
    echo -e "  sudo systemctl start os-storage-node   ${BLUE}# Start service${NC}"
    echo -e "  sudo systemctl stop os-storage-node    ${BLUE}# Stop service${NC}"
    echo -e "  sudo systemctl restart os-storage-node ${BLUE}# Restart service${NC}"
    echo -e "  sudo systemctl status os-storage-node  ${BLUE}# Check status${NC}"
    echo -e "  sudo journalctl -u os-storage-node -f  ${BLUE}# View logs${NC}"
    
    echo -e "\n${GREEN}Quick Management:${NC}"
    echo -e "  sudo os-storage-node start    ${BLUE}# Start${NC}"
    echo -e "  sudo os-storage-node stop     ${BLUE}# Stop${NC}"
    echo -e "  sudo os-storage-node status   ${BLUE}# Status${NC}"
    echo -e "  sudo os-storage-node logs     ${BLUE}# Logs${NC}"
    
    echo -e "\n${GREEN}Next Steps:${NC}"
    echo -e "  1. Access OS node console:"
    echo -e "     ${YELLOW}sudo os-storage-node attach${NC}"
    echo -e ""
    echo -e "  2. Inside OS node, enroll with controller:"
    echo -e "     ${YELLOW}os-storage-enroll${NC}"
    echo -e ""
    echo -e "  3. Use controller dashboard to generate OTP and approve enrollment"
    
    echo -e "\n${GREEN}Configuration:${NC}"
    echo -e "  Edit: ${YELLOW}/opt/os-storage-node/config/os-storage.conf${NC}"
    echo -e "  Then: ${YELLOW}sudo systemctl restart os-storage-node${NC}"
    
    echo -e "\n${BLUE}Build artifacts preserved at:${NC}"
    echo -e "  Kernel:  $BUILDROOT_DIR/output/images/bzImage"
    echo -e "  Rootfs:  $BUILDROOT_DIR/output/images/rootfs.ext2"
    echo -e "  Package: $SCRIPT_DIR/dist/os-storage-node-1.0.0.tar.gz"
    echo -e "  Log:     $SCRIPT_DIR/buildroot-build.log"
    
    echo ""
}

#############################################################################
# Main Execution
#############################################################################

main() {
    print_header "StorageOS Node - Ubuntu Quick Setup"
    
    check_root
    
    # Prompt user
    echo -e "${YELLOW}This script will:${NC}"
    echo "  1. Install build dependencies (buildroot, qemu, gcc, etc.)"
    echo "  2. Build custom OS from source (30-60 minutes)"
    echo "  3. Package distribution tarball"
    echo "  4. Install systemd service"
    echo "  5. Start OS storage node"
    echo ""
    echo -e "${YELLOW}Estimated time: 30-60 minutes${NC}"
    echo ""
    read -p "Continue? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        print_info "Aborted by user"
        exit 0
    fi
    
    # Execute steps
    install_dependencies
    build_os_node
    package_os_node
    install_os_node
    start_service
    print_summary
}

# Run main function
main "$@"
