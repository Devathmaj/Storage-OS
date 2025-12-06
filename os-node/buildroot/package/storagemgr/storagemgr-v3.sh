#!/bin/sh
################################################################################
# storagemgr - Interactive Storage Manager v3.0 (Quota-Based)
################################################################################

set -e

# UI colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

QUOTA_FILE="/etc/storage-quota.conf"
HOST_MOUNT="/host"
DATA_MOUNT="/data"

# Pending changes (commit model)
PENDING_QUOTA_MB=""
HAS_PENDING=false

require_root() {
    if [ "$(id -u)" -ne 0 ]; then
        echo -e "${RED}ERROR:${NC} storagemgr must be run as root"
        exit 1
    fi
}

info() { echo -e "${BLUE}ℹ${NC} $1"; }
success() { echo -e "${GREEN}✓${NC} $1"; }
warn() { echo -e "${YELLOW}⚠${NC} $1"; }
error() { echo -e "${RED}✗${NC} $1"; }

load_quota_config() {
    if [ ! -f "$QUOTA_FILE" ]; then
        error "No quota configuration found. Run 'setup-storage' first."
        exit 1
    fi
    . "$QUOTA_FILE"
}

get_host_available_mb() {
    local avail=0
    if [ -d "$HOST_MOUNT" ] && [ -f "$HOST_MOUNT/.storage-info" ]; then
        avail=$(grep "^AVAILABLE_MB=" "$HOST_MOUNT/.storage-info" | cut -d= -f2)
    elif [ -d "$HOST_MOUNT" ]; then
        avail=$(df -m "$HOST_MOUNT" 2>/dev/null | tail -1 | awk '{print $4}')
    fi
    echo "${avail:-0}"
}

get_data_usage_mb() {
    du -sm "$DATA_MOUNT" 2>/dev/null | awk '{print $1}'
}

get_current_quota_mb() {
    if [ "$HAS_PENDING" = true ]; then
        echo "$PENDING_QUOTA_MB"
    else
        echo "$QUOTA_LIMIT_MB"
    fi
}

print_header() {
    clear
    echo -e "${CYAN}╔═══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║         📦 Interactive Storage Manager v3.0                  ║${NC}"
    echo -e "${CYAN}║         Quota-Based Dynamic Storage                           ║${NC}"
    echo -e "${CYAN}╚═══════════════════════════════════════════════════════════════╝${NC}"
    echo ""
}

show_status() {
    local current_quota=$(get_current_quota_mb)
    local used_mb=$(get_data_usage_mb)
    local free_mb=$((current_quota - used_mb))
    local host_avail=$(get_host_available_mb)
    
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}  Storage Quota Status${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "  ${BLUE}Data Directory:${NC}        $DATA_MOUNT"
    echo ""
    
    if [ "$HAS_PENDING" = true ]; then
        echo -e "  ${YELLOW}⚠ PENDING CHANGES (not committed)${NC}"
        echo -e "  ${BLUE}Current Quota:${NC}         ${QUOTA_LIMIT_MB} MB"
        echo -e "  ${YELLOW}→ New Quota (pending):${NC} ${PENDING_QUOTA_MB} MB"
        echo ""
    fi
    
    echo -e "  ${GREEN}Quota Limit:${NC}           ${current_quota} MB"
    echo -e "  ${YELLOW}Used Space:${NC}            ${used_mb} MB"
    echo -e "  ${GREEN}Available:${NC}             ${free_mb} MB"
    
    if [ "$host_avail" -gt 0 ]; then
        echo ""
        echo -e "  ${BLUE}Host Available:${NC}        ${host_avail} MB"
        if [ -f "$QUOTA_FILE" ]; then
            . "$QUOTA_FILE"
            if [ -n "$HOST_PATH" ]; then
                echo -e "  ${BLUE}Host Path:${NC}             ${HOST_PATH}"
            fi
        fi
    fi
    
    if [ -f "$DATA_MOUNT/.quota-exceeded" ]; then
        echo ""
        echo -e "  ${RED}⚠ WARNING: Quota limit exceeded!${NC}"
    fi
    
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
    echo ""
}

menu_view() {
    show_status
    echo "Press Enter to return to menu..."
    read
}

menu_increase() {
    print_header
    show_status
    
    local current_quota=$(get_current_quota_mb)
    local host_avail=$(get_host_available_mb)
    local max_increase=$QCOW_MAX_MB
    
    if [ "$host_avail" -gt 0 ] && [ "$host_avail" -lt "$max_increase" ]; then
        max_increase=$host_avail
    fi
    
    echo -e "${CYAN}Increase Storage Quota${NC}"
    echo "─────────────────────────────────────────────────────────────────"
    echo "  Current quota: ${current_quota} MB"
    echo "  Maximum:       ${max_increase} MB"
    echo ""
    echo "Enter new quota size (in MB):"
    printf "> "
    read new_quota
    
    # Validate
    if ! echo "$new_quota" | grep -qE '^[0-9]+$'; then
        error "Invalid input. Must be a number."
        sleep 2
        return
    fi
    
    if [ "$new_quota" -le "$current_quota" ]; then
        error "New quota must be larger than current (${current_quota} MB)"
        sleep 2
        return
    fi
    
    if [ "$new_quota" -gt "$max_increase" ]; then
        error "Quota exceeds maximum (${max_increase} MB)"
        sleep 2
        return
    fi
    
    # Set pending change
    PENDING_QUOTA_MB="$new_quota"
    HAS_PENDING=true
    
    success "Quota increase to ${new_quota} MB is pending (not committed)"
    sleep 2
}

menu_decrease() {
    print_header
    show_status
    
    local current_quota=$(get_current_quota_mb)
    local used_mb=$(get_data_usage_mb)
    
    echo -e "${CYAN}Decrease Storage Quota${NC}"
    echo "─────────────────────────────────────────────────────────────────"
    echo "  Current quota: ${current_quota} MB"
    echo "  Current usage: ${used_mb} MB"
    echo "  Minimum:       ${used_mb} MB (cannot go below current usage)"
    echo ""
    
    if [ "$used_mb" -ge "$current_quota" ]; then
        error "Cannot decrease: usage equals or exceeds quota"
        echo "Delete some files first, then try again."
        sleep 3
        return
    fi
    
    echo "Enter new quota size (in MB):"
    printf "> "
    read new_quota
    
    # Validate
    if ! echo "$new_quota" | grep -qE '^[0-9]+$'; then
        error "Invalid input. Must be a number."
        sleep 2
        return
    fi
    
    if [ "$new_quota" -ge "$current_quota" ]; then
        error "New quota must be smaller than current (${current_quota} MB)"
        sleep 2
        return
    fi
    
    if [ "$new_quota" -lt "$used_mb" ]; then
        error "New quota (${new_quota} MB) is less than current usage (${used_mb} MB)"
        echo "Delete some files to free up space first."
        sleep 3
        return
    fi
    
    # Set pending change
    PENDING_QUOTA_MB="$new_quota"
    HAS_PENDING=true
    
    success "Quota decrease to ${new_quota} MB is pending (not committed)"
    sleep 2
}

menu_commit() {
    if [ "$HAS_PENDING" = false ]; then
        warn "No pending changes to commit"
        sleep 2
        return
    fi
    
    print_header
    echo -e "${YELLOW}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${YELLOW}  Commit Pending Changes${NC}"
    echo -e "${YELLOW}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "  Current Quota:  ${QUOTA_LIMIT_MB} MB"
    echo -e "  New Quota:      ${PENDING_QUOTA_MB} MB"
    echo ""
    echo "This will update the quota configuration and restart the quota daemon."
    echo ""
    printf "Commit changes? (yes/no): "
    read confirm
    
    case "$confirm" in
        yes|y|YES|Y)
            # Update config file
            sed -i "s/^QUOTA_LIMIT_MB=.*/QUOTA_LIMIT_MB=${PENDING_QUOTA_MB}/" "$QUOTA_FILE"
            
            # Restart quota daemon
            /usr/sbin/quotad stop 2>/dev/null || true
            sleep 1
            /usr/sbin/quotad start 2>/dev/null || true
            
            success "Changes committed successfully!"
            
            # Clear pending
            PENDING_QUOTA_MB=""
            HAS_PENDING=false
            QUOTA_LIMIT_MB="$PENDING_QUOTA_MB"
            
            sleep 2
            ;;
        *)
            info "Commit cancelled"
            sleep 1
            ;;
    esac
}

menu_discard() {
    if [ "$HAS_PENDING" = false ]; then
        warn "No pending changes to discard"
        sleep 2
        return
    fi
    
    info "Discarding pending changes..."
    PENDING_QUOTA_MB=""
    HAS_PENDING=false
    sleep 1
}

show_menu() {
    print_header
    show_status
    
    echo -e "${CYAN}Menu Options:${NC}"
    echo "─────────────────────────────────────────────────────────────────"
    echo "  [1] View Status"
    echo "  [2] Increase Quota"
    echo "  [3] Decrease Quota"
    
    if [ "$HAS_PENDING" = true ]; then
        echo -e "  ${YELLOW}[4] Commit Changes${NC}"
        echo "  [5] Discard Changes"
    fi
    
    echo "  [q] Quit"
    echo "─────────────────────────────────────────────────────────────────"
    printf "Select option: "
}

main_loop() {
    while true; do
        show_menu
        read choice
        
        case "$choice" in
            1) menu_view ;;
            2) menu_increase ;;
            3) menu_decrease ;;
            4)
                if [ "$HAS_PENDING" = true ]; then
                    menu_commit
                else
                    error "Invalid option"
                    sleep 1
                fi
                ;;
            5)
                if [ "$HAS_PENDING" = true ]; then
                    menu_discard
                else
                    error "Invalid option"
                    sleep 1
                fi
                ;;
            q|Q|quit|exit)
                if [ "$HAS_PENDING" = true ]; then
                    warn "You have uncommitted changes. Discard them? (yes/no)"
                    printf "> "
                    read confirm
                    case "$confirm" in
                        yes|y|YES|Y) break ;;
                        *) continue ;;
                    esac
                else
                    break
                fi
                ;;
            *)
                error "Invalid option"
                sleep 1
                ;;
        esac
    done
    
    echo ""
    info "Exiting storagemgr"
}

main() {
    require_root
    load_quota_config
    main_loop
}

main
