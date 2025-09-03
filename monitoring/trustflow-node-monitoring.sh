#!/bin/bash

# Improved comprehensive monitoring script
# Usage: ./trustflow-node-monitoring.sh [app_name] [interval] [max_log_size_mb]

set -euo pipefail  # Exit on error, undefined vars, pipe failures

APP_NAME=${1:-"trustflow-node"}
INTERVAL=${2:-30}  # seconds
MAX_LOG_SIZE_MB=${3:-100}  # MB, rotate logs when exceeded
LOG_FILE="trustflow-node-metrics-$(date +%Y%m%d_%H%M%S).csv"
ERROR_LOG="trustflow-node-monitoring-errors.log"
SCRIPT_PID=$$
MAX_RETRIES=3
CURL_TIMEOUT=3

# Logging functions
log_info() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] INFO: $1" >> "$ERROR_LOG"
    # Only show startup messages on console
    case "$1" in
        *"Starting enhanced monitoring"*|*"Logging to:"*|*"Error log:"*|*"Interval:"*|*"Max log size:"*|*"Script PID:"*|*"Press Ctrl+C"*|*"pprof_port"*|*"Monitoring stopped"*|*"Total runtime"*)
            echo "[$(date '+%Y-%m-%d %H:%M:%S')] INFO: $1"
            ;;
    esac
}

log_error() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] ERROR: $1" >> "$ERROR_LOG"
}

log_warn() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] WARN: $1" >> "$ERROR_LOG"
}

# Check dependencies
check_dependencies() {
    local missing_deps=()
    for cmd in ps curl awk; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            missing_deps+=("$cmd")
        fi
    done
    
    # netstat or ss (one is sufficient)
    if ! command -v netstat >/dev/null 2>&1 && ! command -v ss >/dev/null 2>&1; then
        missing_deps+=("netstat or ss")
    fi
    
    if [ ${#missing_deps[@]} -ne 0 ]; then
        log_error "Missing dependencies: ${missing_deps[*]}"
        log_info "Please install missing dependencies and retry"
        exit 1
    fi
}

# Create CSV header
create_csv_header() {
    echo "timestamp,pid,cpu_percent,mem_percent,rss_mb,vsz_mb,threads,file_descriptors,tcp_connections,goroutines,heap_allocs,heap_inuse_mb,active_streams,service_cache,relay_circuits,script_memory_mb,errors" > "$LOG_FILE"
}

# Rotate log if too large
rotate_log_if_needed() {
    if [ -f "$LOG_FILE" ]; then
        local size_mb=$(( $(stat -c%s "$LOG_FILE" 2>/dev/null || stat -f%z "$LOG_FILE" 2>/dev/null || echo 0) / 1048576 ))
        if [ "$size_mb" -gt "$MAX_LOG_SIZE_MB" ]; then
            local backup_file="${LOG_FILE%.csv}_$(date +%H%M%S).csv"
            mv "$LOG_FILE" "$backup_file"
            log_info "Log rotated: $backup_file (${size_mb}MB)"
            create_csv_header
        fi
    fi
}

log_info "Starting enhanced monitoring for $APP_NAME"
log_info "Logging to: $LOG_FILE"
log_info "Error log: $ERROR_LOG" 
log_info "Interval: $INTERVAL seconds"
log_info "Max log size: $MAX_LOG_SIZE_MB MB"
log_info "Script PID: $SCRIPT_PID"
log_info "Press Ctrl+C to stop"

# Check dependencies before starting
check_dependencies

# Check if pprof should be enabled
check_pprof_config() {
    local config_files=("./internal/utils/configs" "/etc/trustflow/configs" "~/.trustflow/configs")
    
    for config_file in "${config_files[@]}"; do
        if [ -f "$config_file" ]; then
            local pprof_port=$(grep -i "pprof_port" "$config_file" 2>/dev/null | cut -d= -f2 | tr -d ' ')
            if [ -n "$pprof_port" ]; then
                log_info "Found pprof_port = $pprof_port in $config_file"
                return 0
            fi
        fi
    done
    
    log_info "No pprof_port configured, using default 6060"
    return 0
}

# Create initial CSV header
create_csv_header

# Check pprof configuration
check_pprof_config

# Enhanced trap to handle cleanup
cleanup() {
    log_info "Monitoring stopped. Logs saved to $LOG_FILE"
    log_info "Total runtime: $SECONDS seconds"
    # Clean up temp files
    rm -f /tmp/pprof_*_logged_* 2>/dev/null
    exit 0
}

trap cleanup INT TERM

# Enhanced functions with retry logic and error handling
safe_curl() {
    local url=$1
    local retries=0
    local result=""
    
    while [ $retries -lt $MAX_RETRIES ]; do
        if result=$(timeout $CURL_TIMEOUT curl -s --connect-timeout $CURL_TIMEOUT "$url" 2>/dev/null); then
            echo "$result"
            return 0
        fi
        retries=$((retries + 1))
        [ $retries -lt $MAX_RETRIES ] && sleep 1
    done
    return 1
}

get_process_stats() {
    local pid=$1
    local errors=""
    
    # Basic process stats with error handling
    if ! PS_STATS=$(ps -p "$pid" -o pcpu,pmem,rss,vsz --no-headers 2>/dev/null); then
        errors="ps_failed"
        echo "0,0,0,0,0,0,0,0,0,0,0,0,0,0,${errors}"
        return 1
    fi
    
    CPU=$(echo "$PS_STATS" | awk '{print $1}')
    MEM=$(echo "$PS_STATS" | awk '{print $2}')
    RSS=$(echo "$PS_STATS" | awk '{printf "%.2f", $3/1024}')
    VSZ=$(echo "$PS_STATS" | awk '{printf "%.2f", $4/1024}')
    
    # Thread count with fallback
    if ! THREADS=$(grep "^Threads:" "/proc/$pid/status" 2>/dev/null | awk '{print $2}'); then
        THREADS=0
        errors="${errors}:threads_failed"
    fi
    
    # File descriptor count with fallback  
    if ! FD_COUNT=$(find "/proc/$pid/fd" -type l 2>/dev/null | wc -l); then
        FD_COUNT=0
        errors="${errors}:fd_failed"
    fi
    
    # TCP connections with fallback
    if command -v ss >/dev/null 2>&1; then
        TCP_CONN=$(ss -tulpn 2>/dev/null | grep -c "$pid" || echo "0")
    else
        TCP_CONN=$(netstat -antp 2>/dev/null | grep -c "$pid" || echo "0")
    fi
    
    # Enhanced pprof data collection with retry logic
    GOROUTINES=0
    HEAP_ALLOCS=0  
    HEAP_INUSE=0
    local pprof_success=false
    local pprof_port_found=""
    
    # Try to find actual pprof port by checking what the process is listening on
    local listening_ports=$(netstat -tulpn 2>/dev/null | grep "$pid" | grep LISTEN | awk '{print $4}' | cut -d: -f2 | tr '\n' ' ' || ss -tulpn 2>/dev/null | grep "$pid" | grep LISTEN | awk '{print $5}' | cut -d: -f2 | tr '\n' ' ' || echo "")
    
    # Clean up listening ports (remove duplicates, ensure single line)
    listening_ports=$(echo "$listening_ports" | tr '\n' ' ' | tr -s ' ' | sed 's/^ *//;s/ *$//')
    
    # Add common pprof ports to the list to check (including fallback ports)
    local ports_to_check="6060 6061 6062 8080 9090"
    if [ -n "$listening_ports" ]; then
        ports_to_check="$listening_ports $ports_to_check"
    fi
    
    # Remove duplicates from port list
    ports_to_check=$(echo "$ports_to_check" | tr ' ' '\n' | sort -nu | tr '\n' ' ' | sed 's/ $//')
    
    for port in $ports_to_check; do
        # Skip if not a valid port number
        [[ "$port" =~ ^[0-9]+$ ]] || continue
        
        # Test both IPv4 and IPv6 endpoints
        hosts_to_try="localhost [::1]"
        pprof_host_found=""
        test_successful=false
        
        # DEBUG: Log which port we're testing
        # echo "DEBUG: Testing port $port" >> "$ERROR_LOG"
        
        for host in $hosts_to_try; do
            # First test if pprof is available at all
            if safe_curl "http://$host:$port/debug/pprof/" >/dev/null; then
                pprof_host_found="$host"
                test_successful=true
                # DEBUG: Log successful host
                # echo "DEBUG: Found pprof on $host:$port" >> "$ERROR_LOG"
                break
            fi
        done
        
        if [ "$test_successful" = false ]; then
            continue
        fi
        
        # Try goroutine endpoint with the working host
        if goroutine_data=$(safe_curl "http://$pprof_host_found:$port/debug/pprof/goroutine?debug=1"); then
            # Extract just the number after "total" using awk
            GOROUTINES=$(echo "$goroutine_data" | head -1 | awk '{print $4}')
            # Validate it's a proper integer
            if [[ "$GOROUTINES" =~ ^[0-9]+$ ]] && [ "$GOROUTINES" -gt 0 ]; then
                pprof_success=true
                pprof_port_found="$port"
                
                # Try heap endpoint with the working host
                if heap_data=$(safe_curl "http://$pprof_host_found:$port/debug/pprof/heap?debug=1"); then
                    HEAP_ALLOCS=$(echo "$heap_data" | grep "# heap profile:" | grep -o '[0-9]\+ objects' | grep -o '[0-9]\+' | head -1 || echo "0")
                    HEAP_INUSE=$(echo "$heap_data" | grep -E "# HeapInuse = [0-9]+" | grep -o '[0-9]\+' | head -1 || echo "0")
                    
                    # Fallback heap parsing
                    if [ "$HEAP_INUSE" = "0" ] || ! [[ "$HEAP_INUSE" =~ ^[0-9]+$ ]]; then
                        HEAP_INUSE=$(echo "$heap_data" | grep "# heap profile:" | grep -o '[0-9]\+ \[.*MB\]' | grep -o '[0-9]\+' | tail -1 || echo "0")
                    fi
                    
                    # Ensure we have valid integers
                    [[ "$HEAP_ALLOCS" =~ ^[0-9]+$ ]] || HEAP_ALLOCS=0
                    [[ "$HEAP_INUSE" =~ ^[0-9]+$ ]] || HEAP_INUSE=0
                fi
                break
            fi
        fi
    done
    
    # Fallback if pprof unavailable
    if [ "$pprof_success" = false ]; then
        GOROUTINES=$THREADS
        errors="${errors}:pprof_failed"
        
        # Log pprof diagnostic info on first failure and periodically (avoid spam)
        diag_file="/tmp/pprof_diag_logged_$1"
        if [ ! -f "$diag_file" ] || [ $(($(date +%s) - $(stat -c %Y "$diag_file" 2>/dev/null || echo 0))) -gt 300 ]; then
            log_warn "pprof unavailable for PID $1. Checked ports: $ports_to_check. Process listening on: ${listening_ports:-none}"
            touch "$diag_file"
        fi
    else
        # Log successful pprof discovery
        if [ ! -f "/tmp/pprof_success_logged_$1" ]; then
            log_info "pprof found on port $pprof_port_found for PID $1"
            touch "/tmp/pprof_success_logged_$1"
        fi
    fi
    
    # Convert heap to MB
    HEAP_INUSE_MB=0
    if [[ "$HEAP_INUSE" =~ ^[0-9]+$ ]] && [ "$HEAP_INUSE" -gt 0 ]; then
        # Use awk for floating point arithmetic instead of bc
        HEAP_INUSE_MB=$(awk -v heap="$HEAP_INUSE" 'BEGIN { printf "%.2f", heap / 1048576 }')
    fi
    
    # Placeholder for future enhancements
    ACTIVE_STREAMS=0
    SERVICE_CACHE=0  
    RELAY_CIRCUITS=0
    
    # Script memory monitoring
    SCRIPT_MEMORY_MB=0
    if SCRIPT_RSS=$(ps -p $SCRIPT_PID -o rss --no-headers 2>/dev/null); then
        SCRIPT_MEMORY_MB=$(echo "$SCRIPT_RSS" | awk '{printf "%.2f", $1/1024}')
    fi
    
    # Clean up error string
    errors=${errors#:}
    
    echo "$CPU,$MEM,$RSS,$VSZ,$THREADS,$FD_COUNT,$TCP_CONN,$GOROUTINES,$HEAP_ALLOCS,$HEAP_INUSE_MB,$ACTIVE_STREAMS,$SERVICE_CACHE,$RELAY_CIRCUITS,$SCRIPT_MEMORY_MB,${errors:-none}"
}

# Resource monitoring and alerting
check_resource_alerts() {
    local pid=$1
    local cpu=$2
    local mem=$3
    local rss=$4
    local goroutines=$5
    
    # CPU alert (>80%) - using awk for floating point comparison
    if awk -v cpu="$cpu" 'BEGIN { exit !(cpu > 80) }'; then
        log_warn "High CPU usage: ${cpu}% (PID: $pid)"
    fi
    
    # Memory alert (>90%) - using awk for floating point comparison  
    if awk -v mem="$mem" 'BEGIN { exit !(mem > 90) }'; then
        log_warn "High memory usage: ${mem}% (${rss}MB RSS) (PID: $pid)"
    fi
    
    # Goroutine alert (>1000)
    if [[ "$goroutines" =~ ^[0-9]+$ ]] && [ "$goroutines" -gt 1000 ]; then
        log_warn "High goroutine count: $goroutines (PID: $pid)"
    fi
}

# Main monitoring loop with enhanced error handling
while true; do
    rotate_log_if_needed
    
    TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
    
    # Find process with better error handling
    if ! PID=$(pgrep -f "$APP_NAME" | head -1); then
        echo "$TIMESTAMP,not_found,0,0,0,0,0,0,0,0,0,0,0,0,0,0,process_not_found" >> "$LOG_FILE"
        printf "\r%s | Process not found" "$(date '+%H:%M:%S')"
        sleep "$INTERVAL"
        continue
    fi
    
    if [ -z "$PID" ]; then
        echo "$TIMESTAMP,not_found,0,0,0,0,0,0,0,0,0,0,0,0,0,0,pid_empty" >> "$LOG_FILE" 
        printf "\r%s | Process not found" "$(date '+%H:%M:%S')"
        sleep "$INTERVAL"
        continue
    fi
    
    # Check if process still exists
    if ! kill -0 "$PID" 2>/dev/null; then
        echo "$TIMESTAMP,process_died,0,0,0,0,0,0,0,0,0,0,0,0,0,0,process_died" >> "$LOG_FILE"
        printf "\r%s | Process died (PID: %s)" "$(date '+%H:%M:%S')" "$PID"
        sleep "$INTERVAL"
        continue
    fi
    
    # Get process statistics
    if STATS=$(get_process_stats "$PID"); then
        echo "$TIMESTAMP,$PID,$STATS" >> "$LOG_FILE"
        
        # Parse stats for display and alerts
        CPU=$(echo "$STATS" | cut -d, -f1)
        MEM=$(echo "$STATS" | cut -d, -f2) 
        RSS=$(echo "$STATS" | cut -d, -f3)
        THREADS=$(echo "$STATS" | cut -d, -f5)
        FD_COUNT=$(echo "$STATS" | cut -d, -f6)
        TCP_CONN=$(echo "$STATS" | cut -d, -f7)
        GOROUTINES=$(echo "$STATS" | cut -d, -f8)
        ERRORS=$(echo "$STATS" | cut -d, -f15)
        
        # Resource alerts
        check_resource_alerts "$PID" "$CPU" "$MEM" "$RSS" "$GOROUTINES"
        
        # Enhanced display
        printf "\r%s | PID:%s | CPU:%s%% | MEM:%s%% | RSS:%sMB | T:%s | FD:%s | TCP:%s | GR:%s" \
            "$(date '+%H:%M:%S')" "$PID" "$CPU" "$MEM" "$RSS" "$THREADS" "$FD_COUNT" "$TCP_CONN" "$GOROUTINES"
        
        if [ "$ERRORS" != "none" ]; then
            printf " | ERR:%s" "$ERRORS"
        fi
    else
        log_error "Failed to get process stats for PID $PID"
        echo "$TIMESTAMP,$PID,error_getting_stats,0,0,0,0,0,0,0,0,0,0,0,0,0,stats_failed" >> "$LOG_FILE"
    fi
    
    sleep "$INTERVAL"
done
