#!/bin/bash
# Test script to interact with the Kagenti MLflow agent

AGENT_URL="${AGENT_URL:-http://simple-agent-mlflow.kagenti.svc.cluster.local:8000}"
NAMESPACE="${NAMESPACE:-kagenti}"

echo "🤖 Kagenti MLflow Agent Tester"
echo "================================"
echo ""

# Function to send a message
send_message() {
    local message="$1"
    echo "📤 You: $message"
    
    # Run curl from within the cluster
    response=$(oc run test-curl-$RANDOM -n $NAMESPACE --rm -i --quiet \
        --image=curlimages/curl:8.6.0 --restart=Never -- \
        curl -sS -X POST "$AGENT_URL/chat" \
        -H 'Content-Type: application/json' \
        -d "{\"message\":\"$message\"}" 2>/dev/null)
    
    # Parse the response
    agent_response=$(echo "$response" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('response','Error'))" 2>/dev/null || echo "$response")
    mlflow_status=$(echo "$response" | python3 -c "import sys,json; d=json.load(sys.stdin); print('✅ Logged' if d.get('mlflow_enabled') else '❌ Not logged')" 2>/dev/null || echo "")
    
    echo "🤖 Agent: $agent_response"
    echo "📊 MLflow: $mlflow_status"
    echo ""
}

# Check agent status first
echo "🔍 Checking agent status..."
status=$(oc run status-curl-$RANDOM -n $NAMESPACE --rm -i --quiet \
    --image=curlimages/curl:8.6.0 --restart=Never -- \
    curl -sS "$AGENT_URL/" 2>/dev/null)

mlflow_enabled=$(echo "$status" | python3 -c "import sys,json; d=json.load(sys.stdin); print('✅ Enabled' if d.get('mlflow_enabled') else '❌ Disabled')" 2>/dev/null || echo "Unknown")
mlflow_uri=$(echo "$status" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('mlflow_uri','Unknown'))" 2>/dev/null || echo "Unknown")

echo "📊 MLflow Status: $mlflow_enabled"
echo "🔗 MLflow URI: $mlflow_uri"
echo ""
echo "================================"
echo ""

# Send test messages
messages=(
    "Hello! What can you do?"
    "Tell me about MLflow tracking"
    "How is the weather today?"
    "Generate a summary of our conversation"
    "Thank you, goodbye!"
)

for msg in "${messages[@]}"; do
    send_message "$msg"
    sleep 2
done

echo "================================"
echo "✅ Test complete!"
echo ""
echo "📊 Check MLflow UI for logged runs:"
echo "   https://data-science-gateway.apps.rosa.varsha.x2zf.p3.openshiftapps.com/mlflow/#/workspaces/opendatahub"
echo ""
echo "📋 Check agent logs:"
echo "   oc logs -n $NAMESPACE deploy/simple-agent-mlflow --tail=50"
