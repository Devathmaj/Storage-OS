#!/bin/sh
################################################################################
# quotactl - User-facing quota control utility
################################################################################

QUOTA_FILE="/etc/storage-quota.conf"

get_quota_limit_mb() {
    if [ -f "$QUOTA_FILE" ]; then
        grep "^QUOTA_LIMIT_MB=" "$QUOTA_FILE" | cut -d= -f2
    else
        echo "0"
    fi
}

get_data_usage_mb() {
    du -sm /data 2>/dev/null | awk '{print $1}'
}

show_status() {
    local limit_mb=$(get_quota_limit_mb)
    local used_mb=$(get_data_usage_mb)
    local free_mb=$((limit_mb - used_mb))
    
    echo "Storage Quota Status:"
    echo "  Limit:     ${limit_mb} MB"
    echo "  Used:      ${used_mb} MB"
    echo "  Available: ${free_mb} MB"
    
    if [ -f /data/.quota-exceeded ]; then
        echo "  WARNING: Quota exceeded!"
    fi
}

case "$1" in
    status)
        show_status
        ;;
    *)
        echo "Usage: $0 status"
        exit 1
esac

exit 0
