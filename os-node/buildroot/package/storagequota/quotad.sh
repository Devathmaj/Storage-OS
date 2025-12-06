#!/bin/sh
################################################################################
# quotad - Storage Quota Enforcement Daemon
################################################################################

QUOTA_FILE="/etc/storage-quota.conf"
CHECK_INTERVAL=60
LOG_FILE="/var/log/quotad.log"

log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') - $1" >> "$LOG_FILE"
}

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

check_quota() {
    local limit_mb=$(get_quota_limit_mb)
    local used_mb=$(get_data_usage_mb)
    
    if [ "$limit_mb" -eq 0 ]; then
        return 0
    fi
    
    if [ "$used_mb" -ge "$limit_mb" ]; then
        log "QUOTA EXCEEDED: ${used_mb}MB used / ${limit_mb}MB limit"
        
        # Touch warning file
        touch /data/.quota-exceeded 2>/dev/null || true
        
        # Find and mark files as immutable (simple enforcement)
        # In production, you'd integrate with ext4 project quotas
        return 1
    else
        rm -f /data/.quota-exceeded 2>/dev/null || true
    fi
    
    return 0
}

daemon_loop() {
    log "Quota daemon started (limit: $(get_quota_limit_mb)MB)"
    
    while true; do
        check_quota
        sleep "$CHECK_INTERVAL"
    done
}

case "$1" in
    start)
        echo "Starting quota daemon..."
        daemon_loop &
        echo $! > /var/run/quotad.pid
        ;;
    stop)
        echo "Stopping quota daemon..."
        if [ -f /var/run/quotad.pid ]; then
            kill $(cat /var/run/quotad.pid) 2>/dev/null || true
            rm -f /var/run/quotad.pid
        fi
        ;;
    check)
        check_quota
        ;;
    *)
        echo "Usage: $0 {start|stop|check}"
        exit 1
esac

exit 0
