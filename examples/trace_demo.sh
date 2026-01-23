#!/bin/bash
# Demo script showing trace ID and request ID functionality

echo "Trace ID Demo"
echo "============="
echo ""

# Start the server in the background
echo "Starting DirIO server..."
go run cmd/dirio/main.go serve --data-dir /tmp/dirio-trace-demo --port 9877 > /tmp/dirio-trace.log 2>&1 &
SERVER_PID=$!

# Wait for server to start
sleep 2

echo ""
echo "Making requests to demonstrate trace IDs..."
echo ""

echo "1. Server generates both trace_id and request_id:"
curl -i http://localhost:9877/ 2>&1 | grep -E '(HTTP|X-Trace-ID|X-Request-Id)'

echo ""
echo "2. Providing custom trace ID (simulating distributed tracing):"
curl -i -H "X-Trace-ID: upstream-trace-12345" http://localhost:9877/ 2>&1 | grep -E '(HTTP|X-Trace-ID|X-Request-Id)'

echo ""
echo "3. Server logs showing trace_id and request_id:"
tail -2 /tmp/dirio-trace.log | grep "request handled"

echo ""
echo "Cleaning up..."
kill $SERVER_PID 2>/dev/null
rm -rf /tmp/dirio-trace-demo /tmp/dirio-trace.log

echo "Demo complete!"
echo ""
echo "Notice how:"
echo "  - Each request gets both a trace_id and request_id"
echo "  - The IDs are different (separate concerns)"
echo "  - Both appear in response headers and logs"
echo "  - Custom trace IDs from upstream are respected"