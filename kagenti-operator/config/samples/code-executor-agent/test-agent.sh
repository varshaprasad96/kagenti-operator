#!/bin/bash
# Test script for code-executor-agent

set -e

NAMESPACE="kagenti"
SERVICE_NAME="code-executor-agent"
PORT="8000"

echo "=== Testing Code Executor Agent ==="
echo ""

# Check if pod is running
echo "1. Checking pod status..."
kubectl get pods -n $NAMESPACE -l app.kubernetes.io/name=$SERVICE_NAME

echo ""
echo "2. Checking service..."
kubectl get svc -n $NAMESPACE -l app.kubernetes.io/name=$SERVICE_NAME

echo ""
echo "3. Checking recent logs..."
kubectl logs -n $NAMESPACE -l app.kubernetes.io/name=$SERVICE_NAME --tail=10

echo ""
echo "4. Testing health endpoint..."
kubectl run test-health-$(date +%s) --image=curlimages/curl:8.1.2 --rm -i --restart=Never -n $NAMESPACE -- \
  curl -sS http://${SERVICE_NAME}.${NAMESPACE}.svc.cluster.local:${PORT}/health || true

echo ""
echo "5. Testing code execution with simple Python code..."
kubectl run test-exec-$(date +%s) --image=curlimages/curl:8.1.2 --rm -i --restart=Never -n $NAMESPACE -- \
  curl -sS -X POST http://${SERVICE_NAME}.${NAMESPACE}.svc.cluster.local:${PORT}/execute \
  -H "Content-Type: application/json" \
  -d '{
    "code": "print(\"Hello from sandbox!\"); print(f\"2 + 2 = {2+2}\")",
    "language": "python"
  }' || true

echo ""
echo "6. Testing with a more complex example..."
kubectl run test-exec2-$(date +%s) --image=curlimages/curl:8.1.2 --rm -i --restart=Never -n $NAMESPACE -- \
  curl -sS -X POST http://${SERVICE_NAME}.${NAMESPACE}.svc.cluster.local:${PORT}/execute/python \
  -H "Content-Type: application/json" \
  -d '{
    "code": "import math\nresult = math.sqrt(144)\nprint(f\"Square root of 144 is {result}\")\nfor i in range(3):\n    print(f\"Count: {i}\")"
  }' || true

echo ""
echo "=== Test Complete ==="
