#!/bin/bash
# Demo script showing request ID generation

echo "Request ID Demo"
echo "==============="
echo ""

# Start the server in the background
echo "Starting DirIO server..."
go run cmd/dirio/main.go serve --data-dir /tmp/dirio-demo --port 9876 &
SERVER_PID=$!

# Wait for server to start
sleep 2

echo ""
echo "Making requests to show request IDs..."
echo ""

# Make a request and show the X-Request-Id header
echo "1. Creating a bucket (server generates request ID):"
curl -i http://localhost:9876/demo-bucket -X PUT 2>&1 | grep -E '(HTTP|X-Request-Id)'

echo ""
echo "2. Listing buckets (server generates request ID):"
curl -i http://localhost:9876/ 2>&1 | grep -E '(HTTP|X-Request-Id)'

echo ""
echo "3. Using custom request ID from upstream proxy:"
curl -i -H "X-Request-Id: custom-proxy-id-12345" http://localhost:9876/ 2>&1 | grep -E '(HTTP|X-Request-Id)'

echo ""
echo "Cleaning up..."
kill $SERVER_PID 2>/dev/null
rm -rf /tmp/dirio-demo

echo "Demo complete!"