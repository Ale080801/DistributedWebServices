#!/bin/bash

echo -e "\e[33mTerminating all Go processes for the project...\e[0m"
cd "$(dirname "$0")/.."

if [ -f scripts/.pids ]; then
    pids=$(cat scripts/.pids)
    for pid in $pids; do
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid"
            echo "Killed PID $pid"
        else
            echo "PID $pid not running"
        fi
    done
    rm -f scripts/.pids
fi

pkill -f "catalog-service" || true
pkill -f "auth-service" || true
pkill -f "inventory-service" || true
pkill -f "orders-service" || true
pkill -f "api-gateway" || true

# Additional backup: kill any remaining processes listening on the service ports
echo "Checking for remaining processes on service ports (50051-50054, 8080)..."
for port in 50051 50052 50053 50054 8080; do
    if command -v lsof >/dev/null 2>&1; then
        pids=$(lsof -t -i:"$port" 2>/dev/null)
        for pid in $pids; do
            if [ -n "$pid" ]; then
                kill "$pid" 2>/dev/null && echo "Killed process on port $port (PID $pid)" || true
                sleep 0.2
                if kill -0 "$pid" 2>/dev/null; then
                    kill -9 "$pid" 2>/dev/null && echo "Force killed process on port $port (PID $pid)" || true
                fi
            fi
        done
    elif command -v fuser >/dev/null 2>&1; then
        fuser -k -n tcp "$port" >/dev/null 2>&1 && echo "Killed process on port $port" || true
    fi
done

echo -e "\e[32mStop completed.\e[0m"
