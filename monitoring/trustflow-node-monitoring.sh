#!/bin/bash

# Improved comprehensive monitoring script
# Usage: ./trustflow-node-monitoring.sh [app_name] [interval] [max_log_size_mb] [pid]
# Special commands:
#   ./trustflow-node-monitoring.sh --leak-report     - Generate immediate leak report and exit
#   ./trustflow-node-monitoring.sh --resource-stats  - Query enhanced resource stats from HTTP endpoints
# Environment Variables:
#   ENABLE_GOROUTINE_ANALYSIS=true/false (default: true) - Enable detailed goroutine analysis
#   ENABLE_PERIODIC_REPORTS=true/false (default: true) - Generate stuck goroutine reports every ~5 minutes

set -euo pipefail  # Exit on error, undefined vars, pipe failures

# Handle special commands first before parameter assignment
if [ "${1:-}" = "--leak-report" ]; then
    SPECIAL_COMMAND="leak-report"
    APP_NAME="trustflow-node"  # Use default for special commands
elif [ "${1:-}" = "--resource-stats" ]; then
    SPECIAL_COMMAND="resource-stats"  
    APP_NAME="trustflow-node"  # Use default for special commands
else
    SPECIAL_COMMAND=""
    # Default values for normal monitoring
    APP_NAME=${1:-"trustflow-node"}
fi

# Handle special commands immediately before any file creation
if [ "$SPECIAL_COMMAND" = "leak-report" ]; then
    echo "Generating goroutine leak report for $APP_NAME..."
    # Find the process
    if PID=$(pgrep -x "$APP_NAME" || pgrep -f "/$APP_NAME$" || pgrep -f "$APP_NAME" | grep -v "monitoring" | grep -v "$$" | head -1); then
        echo "Found process PID: $PID"
        # Include required functions inline to avoid dependencies
        show_leak_report() {
            local pid=$1
            echo
            echo "=== GOROUTINE LEAK ANALYSIS ==="
            echo "=== CURRENT GOROUTINE ANALYSIS ==="
            echo "Analyzing current goroutine patterns..."
            
            # Try to get goroutine data from pprof
            if goroutine_data=$(timeout 5 curl -s "http://localhost:6060/debug/pprof/goroutine?debug=1" 2>/dev/null); then
                # Extract total from the profile header
                local total=$(echo "$goroutine_data" | grep "^goroutine profile:" | sed 's/.*total \([0-9]*\).*/\1/')
                echo "Retrieved goroutine profile with total: $total goroutines"
                echo
                
                # Show top goroutine patterns by counting occurrences
                echo "Top goroutine functions:"
                echo "$goroutine_data" | grep -E "^\s*[a-zA-Z]" | awk '{print $1}' | sort | uniq -c | sort -nr | head -10 | while read count func; do
                    printf "  %-30s: %3d\n" "$func" "$count"
                done
                
                echo
                echo "Sample goroutine details:"
                echo "$goroutine_data" | head -20
            else
                echo "Could not retrieve goroutine data from pprof endpoint"
            fi
        }
        show_leak_report "$PID"
        exit 0
    else
        echo "Error: $APP_NAME process not found"
        exit 1
    fi
elif [ "$SPECIAL_COMMAND" = "resource-stats" ]; then
    echo "Querying enhanced resource stats for $APP_NAME..."
    # Find the process
    if PID=$(pgrep -x "$APP_NAME" || pgrep -f "/$APP_NAME$" || pgrep -f "$APP_NAME" | grep -v "monitoring" | grep -v "$$" | head -1); then
        echo "Found process PID: $PID"
        
        # Try different ports to find HTTP server
        for port in 6060 6061 6062 6063 6064 6065; do
            if curl -s --connect-timeout 1 "http://localhost:$port/health" >/dev/null 2>&1; then
                echo "HTTP server found on port: $port"
                
                echo "=== RESOURCE STATS ==="
                curl -s "http://localhost:$port/stats/resources" 2>/dev/null | jq . 2>/dev/null || curl -s "http://localhost:$port/stats/resources"
                
                echo ""
                echo "=== HEALTH STATUS ==="  
                curl -s "http://localhost:$port/health" 2>/dev/null | jq . 2>/dev/null || curl -s "http://localhost:$port/health"
                
                exit 0
            fi
        done
        echo "Error: HTTP server not found for process PID $PID"
        exit 1
    else
        echo "Error: $APP_NAME process not found"
        exit 1
    fi
fi

# Continue with normal monitoring setup only if not a special command
INTERVAL=${2:-30}  # seconds
MAX_LOG_SIZE_MB=${3:-100}  # MB, rotate logs when exceeded
ENABLE_GOROUTINE_ANALYSIS=${ENABLE_GOROUTINE_ANALYSIS:-"true"}  # Set to "false" to disable
ENABLE_PERIODIC_REPORTS=${ENABLE_PERIODIC_REPORTS:-"true"}  # Set to "false" to disable periodic stuck goroutine reports
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
        *"Starting enhanced monitoring"*|*"Logging to:"*|*"Error log:"*|*"Interval:"*|*"Max log size:"*|*"Goroutine analysis:"*|*"Periodic reports:"*|*"Script PID:"*|*"Press Ctrl+C"*|*"pprof_port"*|*"Monitoring stopped"*|*"Total runtime"*)
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
    echo "timestamp,pid,cpu_percent,mem_percent,rss_mb,vsz_mb,threads,file_descriptors,tcp_connections,goroutines,external_goroutines,app_goroutines,libp2p_goroutines,quic_goroutines,dht_goroutines,pubsub_goroutines,net_goroutines,relay_goroutines,heap_allocs,heap_inuse_mb,active_streams,service_cache,relay_circuits,script_memory_mb,goroutine_issues,errors" > "$LOG_FILE"
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
log_info "Goroutine analysis: $ENABLE_GOROUTINE_ANALYSIS"
log_info "Periodic reports: $ENABLE_PERIODIC_REPORTS"
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

# Check for potentially stuck goroutines using pprof data
check_stuck_goroutines() {
    local host=$1
    local port=$2
    local pid=$3
    
    # Only check every 5th cycle to avoid overhead (every ~2.5 minutes with 30s interval)
    local cycle_file="/tmp/goroutine_check_cycle_$pid"
    local cycle_count=0
    
    if [ -f "$cycle_file" ]; then
        cycle_count=$(cat "$cycle_file" 2>/dev/null || echo 0)
    fi
    
    cycle_count=$((cycle_count + 1))
    echo "$cycle_count" > "$cycle_file"
    
    # Get detailed goroutine stack dump every cycle for external analysis
    local goroutine_stacks
    if ! goroutine_stacks=$(safe_curl "http://$host:$port/debug/pprof/goroutine?debug=2" 2>/dev/null); then
        return 1
    fi
    
    # Always analyze external package goroutines for CSV output
    analyze_external_goroutines "$goroutine_stacks" "$pid"
    
    # Run detailed stuck goroutine analysis every 5th cycle to avoid overhead
    if [ $((cycle_count % 5)) -eq 0 ]; then
        # Analyze goroutine patterns for potential issues
        analyze_goroutine_patterns "$goroutine_stacks" "$pid"
        
        # Identify specific stuck goroutines
        identify_stuck_goroutines "$goroutine_stacks" "$pid"
    fi
}

# Analyze goroutine stack patterns for stuck/problematic goroutines
analyze_goroutine_patterns() {
    local stacks="$1"
    local pid="$2"
    local analysis_file="/tmp/goroutine_analysis_$pid"
    
    # Count different types of goroutines
    local chan_recv_count=$(echo "$stacks" | grep -c "chan receive" || echo 0)
    local chan_send_count=$(echo "$stacks" | grep -c "chan send" || echo 0)
    local network_io_count=$(echo "$stacks" | grep -c -E "(net\..*Read|net\..*Write)" || echo 0)
    local select_count=$(echo "$stacks" | grep -c "select" || echo 0)
    local sleep_count=$(echo "$stacks" | grep -c "time\.Sleep" || echo 0)
    local mutex_count=$(echo "$stacks" | grep -c -E "(sync\..*Lock|sync\..*Wait)" || echo 0)
    
    # Look for potentially problematic patterns
    local stuck_patterns=""
    
    # Ensure all counts are integers
    [[ "$chan_recv_count" =~ ^[0-9]+$ ]] || chan_recv_count=0
    [[ "$chan_send_count" =~ ^[0-9]+$ ]] || chan_send_count=0
    [[ "$network_io_count" =~ ^[0-9]+$ ]] || network_io_count=0
    [[ "$select_count" =~ ^[0-9]+$ ]] || select_count=0
    [[ "$sleep_count" =~ ^[0-9]+$ ]] || sleep_count=0
    [[ "$mutex_count" =~ ^[0-9]+$ ]] || mutex_count=0
    
    # High number of goroutines waiting on channels might indicate deadlock
    if [ "$chan_recv_count" -gt 50 ] || [ "$chan_send_count" -gt 50 ]; then
        stuck_patterns="${stuck_patterns}high_channel_wait:$chan_recv_count/$chan_send_count "
    fi
    
    # Many goroutines sleeping might indicate polling instead of event-driven
    if [ "$sleep_count" -gt 20 ]; then
        stuck_patterns="${stuck_patterns}excessive_sleep:$sleep_count "
    fi
    
    # High mutex contention
    if [ "$mutex_count" -gt 30 ]; then
        stuck_patterns="${stuck_patterns}mutex_contention:$mutex_count "
    fi
    
    # Log analysis results with timestamps for trend detection
    local current_time=$(date '+%Y-%m-%d %H:%M:%S')
    local analysis_summary="$current_time|chan_recv:$chan_recv_count|chan_send:$chan_send_count|network_io:$network_io_count|select:$select_count|sleep:$sleep_count|mutex:$mutex_count"
    
    # Keep last 20 entries for trend analysis
    echo "$analysis_summary" >> "$analysis_file"
    tail -20 "$analysis_file" > "${analysis_file}.tmp" && mv "${analysis_file}.tmp" "$analysis_file"
    
    # Alert if problematic patterns detected
    if [ -n "$stuck_patterns" ]; then
        log_warn "Potential goroutine issues detected for PID $pid: $stuck_patterns"
        
        # Log detailed breakdown
        log_warn "Goroutine analysis - Chan recv: $chan_recv_count, Send: $chan_send_count, Network I/O: $network_io_count, Sleep: $sleep_count, Mutex: $mutex_count"
        
        # Set global variable for CSV output (remove trailing space)
        GOROUTINE_ISSUES=$(echo "$stuck_patterns" | sed 's/ *$//')
    fi
    
    # Check for trend: increasing goroutine counts over time
    if [ -f "$analysis_file" ]; then
        local line_count=$(wc -l < "$analysis_file")
        if [ "$line_count" -ge 5 ]; then
            # Get first and last entries to check trend
            local first_entry=$(head -1 "$analysis_file" | cut -d'|' -f2- | tr '|' ' ')
            local last_entry=$(tail -1 "$analysis_file" | cut -d'|' -f2- | tr '|' ' ')
            
            # Extract total goroutines from patterns (sum of major categories)
            local first_total=$(echo "$first_entry" | awk -F: '{sum=0; for(i=2;i<=NF;i+=2) sum+=$i} END {print sum}')
            local last_total=$(echo "$last_entry" | awk -F: '{sum=0; for(i=2;i<=NF;i+=2) sum+=$i} END {print sum}')
            
            # Alert if significant increase (>50% growth)
            if [ "$first_total" -gt 0 ] && [ "$last_total" -gt $((first_total * 150 / 100)) ]; then
                log_warn "Goroutine growth trend detected for PID $pid: $first_total -> $last_total over recent cycles"
            fi
        fi
    fi
}

# Analyze external package goroutines to identify third-party goroutine sources
analyze_external_goroutines() {
    local stacks="$1"
    local pid="$2"
    local external_file="/tmp/external_goroutines_$pid"
    local current_time=$(date '+%Y-%m-%d %H:%M:%S')
    
    # Count goroutines by counting unique "goroutine N [state]:" headers that have external packages in their stack
    local libp2p_count=$(echo "$stacks" | awk '/^goroutine [0-9]+ \[/ { gr=$0; getline; stack=$0; while(getline && !/^goroutine [0-9]+ \[/) stack=stack"\n"$0; if(stack ~ /github\.com\/libp2p|libp2p\/go-/) print gr }' | wc -l)
    local quic_count=$(echo "$stacks" | awk '/^goroutine [0-9]+ \[/ { gr=$0; getline; stack=$0; while(getline && !/^goroutine [0-9]+ \[/) stack=stack"\n"$0; if(stack ~ /quic-go|yamux/) print gr }' | wc -l)
    local dht_count=$(echo "$stacks" | awk '/^goroutine [0-9]+ \[/ { gr=$0; getline; stack=$0; while(getline && !/^goroutine [0-9]+ \[/) stack=stack"\n"$0; if(stack ~ /kad-dht|dht\./) print gr }' | wc -l)
    local pubsub_count=$(echo "$stacks" | awk '/^goroutine [0-9]+ \[/ { gr=$0; getline; stack=$0; while(getline && !/^goroutine [0-9]+ \[/) stack=stack"\n"$0; if(stack ~ /pubsub|go-libp2p-pubsub/) print gr }' | wc -l)
    local net_count=$(echo "$stacks" | awk '/^goroutine [0-9]+ \[/ { gr=$0; getline; stack=$0; while(getline && !/^goroutine [0-9]+ \[/) stack=stack"\n"$0; if(stack ~ /net\/http|crypto\/tls/) print gr }' | wc -l)
    local runtime_count=$(echo "$stacks" | awk '/^goroutine [0-9]+ \[/ { gr=$0; getline; stack=$0; while(getline && !/^goroutine [0-9]+ \[/) stack=stack"\n"$0; if(stack ~ /runtime\.|internal\/poll|sync\./) print gr }' | wc -l)
    
    # Calculate total external goroutines
    local total_external=$((libp2p_count + quic_count + dht_count + pubsub_count + net_count + runtime_count))
    
    # Use the already-parsed goroutine count from the main function
    # (The detailed pprof output doesn't contain the total in a reliable format)
    local total_goroutines=$GOROUTINES
    
    # Calculate app goroutines (estimated)
    local app_goroutines=$((total_goroutines - total_external))
    [ "$app_goroutines" -lt 0 ] && app_goroutines=0
    
    # Log detailed breakdown
    local breakdown="libp2p:$libp2p_count quic:$quic_count dht:$dht_count pubsub:$pubsub_count net:$net_count runtime:$runtime_count"
    log_info "Goroutine breakdown for PID $pid - Total:$total_goroutines External:$total_external App:$app_goroutines | $breakdown"
    
    # Store for trend analysis
    echo "$current_time|$total_goroutines|$total_external|$app_goroutines|$libp2p_count|$quic_count|$dht_count|$pubsub_count|$net_count|$runtime_count" >> "$external_file"
    
    # Keep last 20 entries for trend analysis
    tail -20 "$external_file" > "${external_file}.tmp" && mv "${external_file}.tmp" "$external_file"
    
    # Alert on high external goroutine counts
    if [ "$libp2p_count" -gt 200 ]; then
        log_warn "High libp2p goroutine count: $libp2p_count (PID: $pid)"
    fi
    
    if [ "$total_external" -gt 400 ]; then
        log_warn "High total external goroutines: $total_external (PID: $pid)"
    fi
    
    # Check for external goroutine growth trend
    if [ -f "$external_file" ]; then
        local line_count=$(wc -l < "$external_file")
        if [ "$line_count" -ge 5 ]; then
            local first_external=$(head -1 "$external_file" | cut -d'|' -f3)
            local last_external=$(tail -1 "$external_file" | cut -d'|' -f3)
            
            # Alert if significant growth (>30% increase)
            if [ "$first_external" -gt 0 ] && [ "$last_external" -gt $((first_external * 130 / 100)) ]; then
                local growth_pct=$(((last_external - first_external) * 100 / first_external))
                log_warn "External goroutine growth detected for PID $pid: $first_external -> $last_external (+$growth_pct%)"
            fi
        fi
    fi
    
    # Set global variables for CSV output
    EXTERNAL_GOROUTINES="$total_external"
    LIBP2P_GOROUTINES="$libp2p_count"
    QUIC_GOROUTINES="$quic_count"
    DHT_GOROUTINES="$dht_count"
    PUBSUB_GOROUTINES="$pubsub_count"
    NET_GOROUTINES="$net_count"
    RELAY_GOROUTINES="$runtime_count"  # Using runtime count for relay/misc category
    APP_GOROUTINES="$app_goroutines"
}

# Identify specific stuck goroutines with detailed analysis
identify_stuck_goroutines() {
    local stacks="$1"
    local pid="$2"
    local stuck_goroutines_file="/tmp/stuck_goroutines_$pid"
    local current_time=$(date '+%Y-%m-%d %H:%M:%S')
    
    # Parse goroutine stacks to identify individual goroutines
    # Each goroutine section starts with "goroutine N [status]: " 
    local goroutine_sections=""
    
    # Split stack dump into individual goroutine sections
    goroutine_sections=$(echo "$stacks" | awk '
        BEGIN { section = ""; goroutine_id = ""; status = "" }
        /^goroutine [0-9]+ \[.*\]:/ { 
            if (section != "") print section
            section = $0 "\n"
            goroutine_id = $2
            status = $3
            gsub(/\[|\]:/, "", status)
        }
        !/^goroutine [0-9]+ \[.*\]:/ && section != "" { 
            section = section $0 "\n" 
        }
        END { if (section != "") print section }
    ')
    
    # Analyze each goroutine section for stuck patterns
    local stuck_count=0
    local deadlock_candidates=""
    local long_running_candidates=""
    local resource_leak_candidates=""
    
    echo "$goroutine_sections" | while IFS= read -r section; do
        if [ -z "$section" ]; then continue; fi
        
        # Extract goroutine ID and status
        local gor_id=$(echo "$section" | head -1 | awk '{print $2}')
        local gor_status=$(echo "$section" | head -1 | awk '{print $3}' | sed 's/\[//g;s/\]://g')
        
        # Skip if we can't parse properly
        [ -z "$gor_id" ] || [ -z "$gor_status" ] && continue
        
        # Check for problematic patterns in this specific goroutine
        local is_stuck=false
        local stuck_reason=""
        
        # Pattern 1: Long-running channel operations (potential deadlock)
        if [ "$gor_status" = "chan receive" ] || [ "$gor_status" = "chan send" ]; then
            if echo "$section" | grep -q -E "(runtime\.chanrecv|runtime\.chansend)"; then
                # Check if it's also waiting on sync primitives (more concerning)
                if echo "$section" | grep -q -E "(sync\..*Lock|sync\..*Wait)"; then
                    is_stuck=true
                    stuck_reason="channel_deadlock_with_sync"
                    deadlock_candidates="$deadlock_candidates $gor_id:$stuck_reason"
                else
                    is_stuck=true
                    stuck_reason="channel_blocking"
                    deadlock_candidates="$deadlock_candidates $gor_id:$stuck_reason"
                fi
            fi
        fi
        
        # Pattern 2: Long-running network I/O (potential resource leak)
        if [ "$gor_status" = "IO wait" ] && echo "$section" | grep -q -E "(net\..*Read|net\..*Write)"; then
            is_stuck=true
            stuck_reason="network_io_stuck"
            resource_leak_candidates="$resource_leak_candidates $gor_id:$stuck_reason"
        fi
        
        # Pattern 3: Goroutines stuck in syscalls for too long
        if [ "$gor_status" = "syscall" ] && echo "$section" | grep -q -E "(os\..*)|syscall\."; then
            is_stuck=true
            stuck_reason="syscall_stuck"
            long_running_candidates="$long_running_candidates $gor_id:$stuck_reason"
        fi
        
        # Pattern 4: Goroutines waiting on mutexes for too long
        if [ "$gor_status" = "semacquire" ] && echo "$section" | grep -q "sync\."; then
            is_stuck=true
            stuck_reason="mutex_contention"
            deadlock_candidates="$deadlock_candidates $gor_id:$stuck_reason"
        fi
        
        # Pattern 5: Goroutines in select statements that might be stuck
        if [ "$gor_status" = "select" ] && echo "$section" | grep -q -E "(time\.After|time\.Sleep)"; then
            # This might be normal, but if there are many, it could indicate polling
            if echo "$section" | grep -q -E "for\s*\{" || echo "$section" | grep -q -E "time\.Sleep.*ms"; then
                is_stuck=true
                stuck_reason="polling_loop"
                long_running_candidates="$long_running_candidates $gor_id:$stuck_reason"
            fi
        fi
        
        # Log individual stuck goroutine details
        if [ "$is_stuck" = true ]; then
            stuck_count=$((stuck_count + 1))
            
            # Log the specific goroutine with truncated stack trace (first 3 lines of stack)
            local short_stack=$(echo "$section" | head -4 | tail -3 | tr '\n' '; ')
            log_warn "Stuck goroutine detected: ID=$gor_id status=[$gor_status] reason=$stuck_reason stack_preview: $short_stack"
            
            # Save to file for trend analysis
            echo "$current_time|$gor_id|$gor_status|$stuck_reason|$short_stack" >> "$stuck_goroutines_file"
        fi
    done
    
    # Summary analysis
    if [ -n "$deadlock_candidates" ]; then
        local deadlock_count=$(echo "$deadlock_candidates" | wc -w)
        log_warn "Potential deadlock: $deadlock_count goroutines in blocking states: $deadlock_candidates"
    fi
    
    if [ -n "$resource_leak_candidates" ]; then
        local leak_count=$(echo "$resource_leak_candidates" | wc -w) 
        log_warn "Potential resource leak: $leak_count goroutines stuck in I/O: $resource_leak_candidates"
    fi
    
    if [ -n "$long_running_candidates" ]; then
        local long_count=$(echo "$long_running_candidates" | wc -w)
        log_warn "Long-running goroutines: $long_count goroutines in syscalls/loops: $long_running_candidates"
    fi
    
    # Keep only last 50 entries in stuck goroutines log
    # Keep only last 50 entries in stuck goroutines log
    if [ -f "$stuck_goroutines_file" ]; then
        tail -50 "$stuck_goroutines_file" > "${stuck_goroutines_file}.tmp" && mv "${stuck_goroutines_file}.tmp" "$stuck_goroutines_file"
    fi
}

# Generate a detailed stuck goroutines report (can be called manually)
generate_stuck_goroutines_report() {
    local pid=${1:-$(pgrep -f "trustflow-node" | grep -v "$SCRIPT_PID" | head -1)}
    
    if [ -z "$pid" ]; then
        echo "No trustflow-node process found"
        return 1
    fi
    
    echo "=== Stuck Goroutines Report for PID $pid ==="
    echo "Generated at: $(date)"
    echo
    
    local stuck_file="/tmp/stuck_goroutines_$pid"
    if [ -f "$stuck_file" ]; then
        echo "Recent stuck goroutines detected:"
        echo "Time | Goroutine ID | Status | Reason | Stack Preview"
        echo "------------------------------------------------------------"
        cat "$stuck_file" | tail -20 | while IFS='|' read -r timestamp gor_id status reason stack; do
            printf "%-19s | %-12s | %-15s | %-20s | %s\n" "$timestamp" "$gor_id" "$status" "$reason" "${stack:0:60}..."
        done
    else
        echo "No stuck goroutines file found. Monitor may not have detected any issues yet."
    fi
    
    echo
    echo "=== Current Analysis ==="
    
    # Try to get fresh goroutine dump for immediate analysis
    local port=6060
    if goroutine_stacks=$(timeout 5 curl -s "http://localhost:$port/debug/pprof/goroutine?debug=2" 2>/dev/null); then
        echo "Fresh goroutine analysis:"
        identify_stuck_goroutines "$goroutine_stacks" "$pid" 2>&1 | grep "Stuck goroutine detected" | head -10
    else
        echo "Could not retrieve fresh goroutine data from pprof endpoint"
    fi
    
    echo
    echo "=== Trend Analysis ==="
    local analysis_file="/tmp/goroutine_analysis_$pid"
    if [ -f "$analysis_file" ]; then
        echo "Goroutine pattern trends (last 10 entries):"
        tail -10 "$analysis_file"
    else
        echo "No trend data available yet"
    fi
}

get_process_stats() {
    local pid=$1
    local errors=""
    
    # Basic process stats with error handling
    if ! PS_STATS=$(ps -p "$pid" -o pcpu,pmem,rss,vsz --no-headers 2>/dev/null); then
        errors="ps_failed"
        echo "0,0,0,0,0,0,0,0,0,0,0,0,0,0,none,${errors}"
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
    EXTERNAL_GOROUTINES=0
    APP_GOROUTINES=0
    LIBP2P_GOROUTINES=0
    QUIC_GOROUTINES=0
    DHT_GOROUTINES=0
    PUBSUB_GOROUTINES=0
    NET_GOROUTINES=0
    HEAP_ALLOCS=0  
    HEAP_INUSE=0
    GOROUTINE_ISSUES="none"
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
                
                # Always get goroutine breakdown for CSV output (lightweight)
                if goroutine_stacks=$(safe_curl "http://$pprof_host_found:$port/debug/pprof/goroutine?debug=2"); then
                    analyze_external_goroutines "$goroutine_stacks" "$1"
                fi
                
                # Check for stuck goroutines (optional detailed analysis)
                if [ "$ENABLE_GOROUTINE_ANALYSIS" = "true" ]; then
                    check_stuck_goroutines "$pprof_host_found" "$port" "$1"
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
    
    # Initialize detailed goroutine breakdown variables  
    QUIC_GOROUTINES=${QUIC_GOROUTINES:-0}
    DHT_GOROUTINES=${DHT_GOROUTINES:-0}
    PUBSUB_GOROUTINES=${PUBSUB_GOROUTINES:-0}
    NET_GOROUTINES=${NET_GOROUTINES:-0}
    RELAY_GOROUTINES=${RELAY_GOROUTINES:-0}
    
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
    
    echo "$CPU,$MEM,$RSS,$VSZ,$THREADS,$FD_COUNT,$TCP_CONN,$GOROUTINES,$EXTERNAL_GOROUTINES,$APP_GOROUTINES,$LIBP2P_GOROUTINES,$QUIC_GOROUTINES,$DHT_GOROUTINES,$PUBSUB_GOROUTINES,$NET_GOROUTINES,$RELAY_GOROUTINES,$HEAP_ALLOCS,$HEAP_INUSE_MB,$ACTIVE_STREAMS,$SERVICE_CACHE,$RELAY_CIRCUITS,$SCRIPT_MEMORY_MB,$GOROUTINE_ISSUES,${errors:-none}"
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

# Generate comprehensive goroutine leak report
show_leak_report() {
    local pid=$1
    echo
    echo "=== GOROUTINE LEAK ANALYSIS ==="
    
    echo "=== HISTORICAL LEAK DATA ==="
    if [ -f "/tmp/goroutine_analysis_$pid" ]; then
        echo "Recent leak detection data (last 20 entries):"
        tail -20 "/tmp/goroutine_analysis_$pid" | while IFS='|' read -r timestamp total external app libp2p quic dht pubsub net runtime; do
            if [ -n "$timestamp" ] && [ "$timestamp" != "timestamp" ]; then
                echo "[$timestamp] Total:$total Ext:$external App:$app L:$libp2p Q:$quic D:$dht P:$pubsub N:$net R:$runtime"
            fi
        done
    else
        echo "No historical leak analysis data available yet"
    fi
    
    echo
    echo "=== CURRENT GOROUTINE ANALYSIS ==="
    analyze_current_goroutines "$pid"
}

# Enhanced goroutine analysis 
analyze_current_goroutines() {
    local pid=$1
    local port=6060
    
    echo "Analyzing current goroutine patterns..."
    
    if goroutine_stacks=$(timeout 10 curl -s "http://localhost:$port/debug/pprof/goroutine?debug=2" 2>/dev/null); then
        # Count goroutines by pattern
        echo "Current goroutine breakdown:"
        echo "$goroutine_stacks" | awk '
        /^goroutine [0-9]+ \[/ {
            # Extract state
            match($0, /\[([^\]]+)\]/, state_match)
            state = state_match[1]
            
            # Get next line for function
            getline
            funcname = $1
            
            # Simplify function name
            if (match(funcname, /\.([^.]+)$/, func_match)) {
                funcname = func_match[1]
            }
            
            pattern = funcname "[" state "]"
            count[pattern]++
            total++
        }
        END {
            for (p in count) {
                printf "  %-40s: %3d\n", p, count[p]
            }
            printf "\nTotal goroutines analyzed: %d\n", total
        }'
        
        # Look for potential stuck patterns
        echo
        echo "Potential stuck goroutines (first 10):"
        echo "$goroutine_stacks" | grep -E -A1 "(chan receive|chan send|select)" | head -20 | while read -r line; do
            if [[ $line =~ ^goroutine ]]; then
                echo "  $line"
            fi
        done
    else
        echo "Could not retrieve goroutine data from pprof endpoint"
    fi
}

# Periodic leak report generation
generate_periodic_leak_report() {
    local pid=$1
    local report_interval=1800  # 30 minutes
    local last_report_file="/tmp/last_leak_report_$pid"
    local current_time=$(date +%s)
    
    # Check if it's time for a periodic report
    if [ -f "$last_report_file" ]; then
        local last_report_time=$(cat "$last_report_file")
        if [ $((current_time - last_report_time)) -lt $report_interval ]; then
            return  # Not time yet
        fi
    fi
    
    # Generate leak report
    echo "$(date): Generating periodic goroutine leak report"
    show_leak_report "$pid" | head -50  # Limit output size
    echo "$current_time" > "$last_report_file"
}

# Query HTTP resource stats from P2P manager
query_resource_stats() {
    local pid=$1
    local pprof_port=${2:-6060}
    
    # Try to get enhanced resource stats from HTTP endpoint
    local stats_url="http://localhost:$pprof_port/stats/resources"
    
    if resource_stats=$(curl -s --connect-timeout $CURL_TIMEOUT "$stats_url" 2>/dev/null); then
        # Parse JSON response and extract key metrics
        local active_streams=$(echo "$resource_stats" | grep -o '"active_streams":[0-9]*' | cut -d: -f2 || echo 0)
        local service_offers=$(echo "$resource_stats" | grep -o '"service_offers":[0-9]*' | cut -d: -f2 || echo 0)
        local relay_circuits=$(echo "$resource_stats" | grep -o '"relay_circuits":[0-9]*' | cut -d: -f2 || echo 0)
        local goroutines_active=$(echo "$resource_stats" | grep -o '"goroutines_active":[0-9]*' | cut -d: -f2 || echo 0)
        local goroutines_usage=$(echo "$resource_stats" | grep -o '"goroutines_usage_percent":[0-9]*' | cut -d: -f2 || echo 0)
        
        # Update global variables for CSV output
        ACTIVE_STREAMS=${active_streams:-0}
        SERVICE_CACHE=${service_offers:-0}
        RELAY_CIRCUITS=${relay_circuits:-0}
        
        return 0
    else
        # Fallback to existing method if HTTP endpoint unavailable
        return 1
    fi
}

# Get health status from HTTP endpoint
get_health_status() {
    local pid=$1
    local pprof_port=${2:-6060}
    
    local health_url="http://localhost:$pprof_port/health"
    
    if health_data=$(curl -s --connect-timeout $CURL_TIMEOUT "$health_url" 2>/dev/null); then
        local status=$(echo "$health_data" | grep -o '"status":"[^"]*"' | cut -d: -f2 | tr -d '"' || echo "unknown")
        local host_running=$(echo "$health_data" | grep -o '"host_running":[^,}]*' | cut -d: -f2 | tr -d ' ' || echo "false")
        
        # Log health status periodically
        if [ "$status" != "ok" ] || [ "$host_running" != "true" ]; then
            log_warn "P2P health check failed: status=$status, host_running=$host_running (PID: $pid)"
        fi
        
        return 0
    else
        return 1
    fi
}

# Find pprof port for the given process ID
find_pprof_port() {
    local pid=$1
    
    # First, try to read from config files
    local config_files=(
        "${HOME}/.trustflow/config.txt"
        "./config.txt"
        "/etc/trustflow/config.txt"
    )
    
    for config_file in "${config_files[@]}"; do
        if [ -f "$config_file" ]; then
            local pprof_port=$(grep -i "pprof_port" "$config_file" 2>/dev/null | cut -d= -f2 | tr -d ' ')
            if [ -n "$pprof_port" ] && [[ "$pprof_port" =~ ^[0-9]+$ ]]; then
                echo "$pprof_port"
                return 0
            fi
        fi
    done
    
    # If not found in config, try to detect from network connections
    # Look for listening ports associated with the PID
    local listening_ports=$(netstat -tulnp 2>/dev/null | grep ":$pid/" | awk '{print $4}' | sed 's/.*://' | sort -n)
    
    # Try common pprof port ranges (6060-6070)
    for port in 6060 6061 6062 6063 6064 6065 6066 6067 6068 6069 6070; do
        if echo "$listening_ports" | grep -q "^$port$"; then
            # Test if it's actually a pprof endpoint
            if curl -s --connect-timeout 1 "http://localhost:$port/debug/pprof/" >/dev/null 2>&1; then
                echo "$port"
                return 0
            fi
        fi
    done
    
    # Return empty if not found
    return 1
}

# Enhanced resource monitoring with HTTP stats integration
enhanced_resource_monitoring() {
    local pid=$1
    
    # Wrap in error handling to prevent script exit
    {
        # First try to get pprof port
        local pprof_port_found=""
        pprof_port_found=$(find_pprof_port "$pid" 2>/dev/null || echo "")
        
        if [ -n "$pprof_port_found" ]; then
            # Query enhanced resource stats from HTTP endpoint
            if query_resource_stats "$pid" "$pprof_port_found" 2>/dev/null; then
                log_info "Enhanced resource stats retrieved from HTTP endpoint (port $pprof_port_found)"
            fi
            
            # Check health status
            get_health_status "$pid" "$pprof_port_found" 2>/dev/null || true
        fi
    } 2>/dev/null || {
        # Silently continue if enhanced monitoring fails
        echo "[$(date '+%Y-%m-%d %H:%M:%S')] WARN: Enhanced monitoring failed for PID $pid" >> "$ERROR_LOG" 2>/dev/null || true
    }
}


# Main monitoring loop with enhanced error handling
while true; do
    # Wrap entire loop iteration in error handling to prevent script death
    {
        rotate_log_if_needed
        
        TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
    
    # Find the actual trustflow-node binary process, not scripts
    if ! PID=$(pgrep -x "$APP_NAME" || pgrep -f "/$APP_NAME$" || pgrep -f "$APP_NAME" | grep -v "monitoring" | grep -v "$$" | head -1); then
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
        
        # Parse stats for display and alerts and clean up any newlines
        # CSV order: cpu_percent,mem_percent,rss_mb,vsz_mb,threads,file_descriptors,tcp_connections,goroutines,external_goroutines,app_goroutines,libp2p_goroutines,quic_goroutines,dht_goroutines,pubsub_goroutines,net_goroutines,relay_goroutines,heap_allocs,heap_inuse_mb,active_streams,service_cache,relay_circuits,script_memory_mb,goroutine_issues,errors
        CPU=$(echo "$STATS" | cut -d, -f1 | tr -d '\n\r')
        MEM=$(echo "$STATS" | cut -d, -f2 | tr -d '\n\r') 
        RSS=$(echo "$STATS" | cut -d, -f3 | tr -d '\n\r')
        THREADS=$(echo "$STATS" | cut -d, -f5 | tr -d '\n\r')
        FD_COUNT=$(echo "$STATS" | cut -d, -f6 | tr -d '\n\r')
        TCP_CONN=$(echo "$STATS" | cut -d, -f7 | tr -d '\n\r')
        GOROUTINES=$(echo "$STATS" | cut -d, -f8 | tr -d '\n\r')
        EXTERNAL_GR=$(echo "$STATS" | cut -d, -f9 | tr -d '\n\r')
        APP_GR=$(echo "$STATS" | cut -d, -f10 | tr -d '\n\r')
        LIBP2P_GR=$(echo "$STATS" | cut -d, -f11 | tr -d '\n\r')
        QUIC_GR=$(echo "$STATS" | cut -d, -f12 | tr -d '\n\r')
        DHT_GR=$(echo "$STATS" | cut -d, -f13 | tr -d '\n\r')
        PUBSUB_GR=$(echo "$STATS" | cut -d, -f14 | tr -d '\n\r')
        NET_GR=$(echo "$STATS" | cut -d, -f15 | tr -d '\n\r')
        RELAY_GR=$(echo "$STATS" | cut -d, -f16 | tr -d '\n\r')
        ERRORS=$(echo "$STATS" | cut -d, -f24 | tr -d '\n\r')
        
        # Resource alerts
        check_resource_alerts "$PID" "$CPU" "$MEM" "$RSS" "$GOROUTINES"
        
        # Enhanced resource monitoring via HTTP endpoints (with error isolation)
        enhanced_resource_monitoring "$PID" || {
            echo "[$(date '+%Y-%m-%d %H:%M:%S')] WARN: Enhanced resource monitoring failed for PID $PID, continuing..." >> "$ERROR_LOG" 2>/dev/null || true
        }
        
        # Generate periodic leak reports (every 30 minutes) with error isolation  
        generate_periodic_leak_report "$PID" || {
            echo "[$(date '+%Y-%m-%d %H:%M:%S')] WARN: Periodic leak report failed for PID $PID, continuing..." >> "$ERROR_LOG" 2>/dev/null || true
        }
        
        # Enhanced display with detailed external package breakdown
        printf "\r%s | PID:%s | CPU:%s%% | MEM:%s%% | RSS:%sMB | T:%s | FD:%s | TCP:%s | GR:%s(E:%s/A:%s|L:%s/Q:%s/D:%s/P:%s/N:%s/R:%s)" \
            "$(date '+%H:%M:%S')" "$PID" "$CPU" "$MEM" "$RSS" "$THREADS" "$FD_COUNT" "$TCP_CONN" "$GOROUTINES" "$EXTERNAL_GR" "$APP_GR" "$LIBP2P_GR" "$QUIC_GR" "$DHT_GR" "$PUBSUB_GR" "$NET_GR" "$RELAY_GR"
        
        if [ "$ERRORS" != "none" ]; then
            printf " | ERR:%s" "$ERRORS"
        fi
        
        # Force output and ensure line completion
        printf "\n"; sleep 0.1
        
        # Generate periodic stuck goroutine reports if enabled (after display output)
        if [ "$ENABLE_PERIODIC_REPORTS" = "true" ]; then
            # Only generate report every 10th cycle to avoid log spam (every ~5 minutes with 30s interval)
            report_cycle_file="/tmp/report_cycle_$PID"
            report_cycle_count=0
            
            if [ -f "$report_cycle_file" ]; then
                report_cycle_count=$(cat "$report_cycle_file" 2>/dev/null || echo 0)
            fi
            
            report_cycle_count=$((report_cycle_count + 1))
            echo "$report_cycle_count" > "$report_cycle_file"
            
            # Generate report every 10th cycle  
            if [ $((report_cycle_count % 10)) -eq 0 ]; then
                # Run in background with proper error isolation
                (
                    # Isolate background job from main script's set -euo pipefail
                    set +euo pipefail
                    echo "[$(date '+%Y-%m-%d %H:%M:%S')] INFO: Starting periodic stuck goroutines report for PID $PID" >> "$ERROR_LOG" 2>/dev/null || true
                    
                    # Run with timeout to prevent hanging
                    if timeout 30s generate_stuck_goroutines_report "$PID" >> "$ERROR_LOG" 2>&1; then
                        echo "[$(date '+%Y-%m-%d %H:%M:%S')] INFO: Completed periodic stuck goroutines report for PID $PID" >> "$ERROR_LOG" 2>/dev/null || true
                    else
                        echo "[$(date '+%Y-%m-%d %H:%M:%S')] WARN: Periodic stuck goroutines report failed or timed out for PID $PID" >> "$ERROR_LOG" 2>/dev/null || true
                    fi
                ) &
                # Don't wait for background job to complete
            fi
        fi
    else
        log_error "Failed to get process stats for PID $PID"
        echo "$TIMESTAMP,$PID,error_getting_stats,0,0,0,0,0,0,0,0,0,0,0,0,0,stats_failed" >> "$LOG_FILE"
    fi
    
    } || {
        # Catch any errors that escape the individual error handlers
        echo "[$(date '+%Y-%m-%d %H:%M:%S')] ERROR: Main loop iteration failed, but continuing monitoring..." >> "$ERROR_LOG" 2>/dev/null || true
        printf "\r%s | ERROR in monitoring iteration, continuing...\n" "$(date '+%H:%M:%S')"
    }
    
    sleep "$INTERVAL"
done
