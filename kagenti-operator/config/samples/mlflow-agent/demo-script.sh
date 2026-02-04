#!/bin/bash
#
# Kagenti + MLflow Integration Demo Script
# =========================================
# This script demonstrates the integration between Kagenti AI Agent operator
# and Open Data Hub MLflow for experiment tracking.
#
# Prerequisites:
# - OpenShift cluster with ODH installed
# - oc CLI logged in
#
# Usage: 
#   ./demo-script.sh          # Interactive mode with pauses
#   ./demo-script.sh --auto   # Automated mode (no pauses)
#

set -e

# Parse arguments
AUTO_MODE=false
if [[ "$1" == "--auto" ]]; then
    AUTO_MODE=true
fi

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SAMPLES_DIR="$(dirname "$SCRIPT_DIR")"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../../../.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Helper function to print section headers
section() {
    echo ""
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
    echo ""
}

# Helper function to run command with explanation
run_cmd() {
    echo -e "${YELLOW}$ $1${NC}"
    echo ""
    eval "$1"
    echo ""
}

# Helper function for pause (for manual demo)
pause() {
    if [[ "$AUTO_MODE" == "true" ]]; then
        sleep 2
    else
        echo -e "${GREEN}[Press Enter to continue...]${NC}"
        read -r
    fi
}

#==============================================================================
# PRE-FLIGHT CHECKS
#==============================================================================

section "Pre-flight Checks"

echo "Checking prerequisites..."
echo ""

# Check oc is logged in
if ! oc whoami &>/dev/null; then
    echo -e "${RED}ERROR: Not logged into OpenShift. Please run 'oc login' first.${NC}"
    exit 1
fi
echo -e "${GREEN}✓${NC} Logged into OpenShift as: $(oc whoami)"

# Check kagenti namespace exists
if ! oc get namespace kagenti &>/dev/null; then
    echo "Creating kagenti namespace..."
    oc create namespace kagenti
fi
echo -e "${GREEN}✓${NC} Namespace 'kagenti' exists"

# Check demo files exist
if [[ ! -f "$SAMPLES_DIR/mlflow/odh-simple-agent-mlflow-demo.yaml" ]]; then
    echo -e "${RED}ERROR: Demo manifest not found at $SAMPLES_DIR/mlflow/odh-simple-agent-mlflow-demo.yaml${NC}"
    exit 1
fi
echo -e "${GREEN}✓${NC} Demo manifests found"

echo ""
echo -e "${GREEN}All pre-flight checks passed!${NC}"
echo ""

pause

#==============================================================================
# PART 1: Show MLflow Operator Installation
#==============================================================================

section "1. Verifying MLflow Operator Installation"

echo "First, let's verify that the Open Data Hub MLflow operator is installed and running."
echo ""

run_cmd "oc get pods -n opendatahub -l app.kubernetes.io/name=mlflow-operator"

echo "Let's also check the MLflow instance:"
echo ""

run_cmd "oc get mlflow -n opendatahub"

echo "And verify the MLflow deployment is running:"
echo ""

run_cmd "oc get pods -n opendatahub -l app.kubernetes.io/name=mlflow"

pause

#==============================================================================
# PART 2: Show Kagenti Operator Installation
#==============================================================================

section "2. Verifying Kagenti Operator Installation"

echo "Now let's verify that the Kagenti AI Agent operator is installed and running."
echo ""

run_cmd "oc get pods -n kagenti-system -l control-plane=controller-manager"

pause

#==============================================================================
# PART 3: Show Agent CRDs
#==============================================================================

section "3. Kagenti Agent Custom Resource Definitions (CRDs)"

echo "Kagenti provides Custom Resource Definitions for managing AI agents."
echo "Let's see what CRDs are available:"
echo ""

run_cmd "oc get crd | grep kagenti"

echo ""
echo "The key CRDs are:"
echo "  - agents.agent.kagenti.dev        : Define and deploy AI agents"
echo "  - agentbuilds.agent.kagenti.dev   : Build agent container images"
echo "  - agentcards.agent.kagenti.dev    : Agent metadata and capabilities"
echo ""

run_cmd "oc explain agent.spec --recursive | head -30"

pause

#==============================================================================
# PART 4: Show the Custom Agent Definition
#==============================================================================

section "4. Custom Agent with MLflow Tracing"

echo "I've created a custom Kagenti Agent that includes MLflow tracing."
echo "Let's look at the Agent definition:"
echo ""

run_cmd "cat <<'EOF'
apiVersion: agent.kagenti.dev/v1alpha1
kind: Agent
metadata:
  name: simple-agent-mlflow-demo
  namespace: kagenti
  labels:
    kagenti.io/observability: mlflow
spec:
  description: \"Simple agent with MLflow tracking\"
  replicas: 1
  imageSource:
    image: quay.io/vnarsing/simple-agent:latest
  podTemplateSpec:
    spec:
      serviceAccountName: simple-agent-mlflow-demo
      containers:
        - name: agent
          env:
            # MLflow configuration
            - name: MLFLOW_TRACKING_URI
              value: \"https://mlflow.opendatahub.svc.cluster.local:8443\"
            - name: MLFLOW_EXPERIMENT_NAME
              value: \"kagenti-mlflow-demo\"
            - name: MLFLOW_WORKSPACE
              value: \"opendatahub\"
            - name: MLFLOW_TRACKING_INSECURE_TLS
              value: \"true\"
EOF"

echo ""
echo "Key MLflow configuration:"
echo "  - MLFLOW_TRACKING_URI: Points to the ODH MLflow service"
echo "  - MLFLOW_EXPERIMENT_NAME: Name of the experiment in MLflow"
echo "  - MLFLOW_WORKSPACE: Maps to the Kubernetes namespace"
echo ""

pause

#==============================================================================
# PART 5: Deploy the Agent
#==============================================================================

section "5. Deploying the MLflow-Enabled Agent"

echo "Now let's deploy the agent using kubectl/oc apply:"
echo ""

# Always apply RBAC first (idempotent)
echo "Setting up RBAC for MLflow access..."
run_cmd "oc apply -f $SAMPLES_DIR/mlflow/odh-simple-agent-mlflow-demo-rbac.yaml"

echo ""

# Check if agent already exists
if oc get agent -n kagenti simple-agent-mlflow-demo &>/dev/null; then
    echo "Agent already exists. Showing current status:"
else
    echo "Applying the agent manifest..."
    run_cmd "oc apply -f $SAMPLES_DIR/mlflow/odh-simple-agent-mlflow-demo.yaml"
    
    echo ""
    echo "Waiting for agent to start..."
    sleep 10
fi

echo ""
echo "Checking agent status:"
echo ""

run_cmd "oc get agent -n kagenti simple-agent-mlflow-demo"

echo ""
echo "Verifying the agent pod is running:"
echo ""

run_cmd "oc get pods -n kagenti -l app.kubernetes.io/name=simple-agent-mlflow-demo"

echo ""
echo "Checking agent logs to confirm MLflow initialization:"
echo ""

run_cmd "oc logs -n kagenti deploy/simple-agent-mlflow-demo --tail=10"

pause

#==============================================================================
# PART 6: Interact with the Agent
#==============================================================================

section "6. Interacting with the Agent (MLflow Logging)"

echo "Now let's send some requests to the agent."
echo "Each request will be logged to MLflow with metrics like latency, success rate, etc."
echo ""

# Send test messages
messages=(
    "Hello! What can you help me with?"
    "Explain the benefits of MLflow tracking"
    "How does Kagenti integrate with MLflow?"
    "Generate a summary of AI observability"
    "Thank you for the demo!"
)

for i in "${!messages[@]}"; do
    msg="${messages[$i]}"
    echo -e "${GREEN}📤 Request $((i+1)): $msg${NC}"
    echo ""
    
    response=$(oc run demo-curl-$RANDOM -n kagenti --rm -i --quiet \
        --image=curlimages/curl:8.6.0 --restart=Never -- \
        curl -sS -X POST http://simple-agent-mlflow-demo.kagenti.svc.cluster.local:8000/chat \
        -H 'Content-Type: application/json' \
        -d "{\"message\":\"$msg\"}" 2>/dev/null)
    
    echo -e "${BLUE}🤖 Response: $response${NC}"
    echo ""
    sleep 2
done

pause

#==============================================================================
# PART 7: Verify MLflow Logging
#==============================================================================

section "7. Verifying MLflow Experiment Logging"

echo "Let's check the agent logs to see the MLflow runs that were created:"
echo ""

run_cmd "oc logs -n kagenti deploy/simple-agent-mlflow-demo --tail=30 | grep -E 'View run|Logged|Request|experiment'"

echo ""
echo "Each request created an MLflow run with:"
echo "  - Run name (e.g., chat_1, chat_2, ...)"
echo "  - Parameters (task_name, message_length)"
echo "  - Metrics (latency_seconds, success, request_count, success_rate)"
echo ""

pause

#==============================================================================
# PART 8: Summary
#==============================================================================

section "8. Summary & MLflow UI"

echo "🎉 Demo Complete!"
echo ""
echo "What we demonstrated:"
echo "  ✅ MLflow Operator running in Open Data Hub"
echo "  ✅ Kagenti Operator managing AI agents"
echo "  ✅ Agent CRDs for declarative agent deployment"
echo "  ✅ Custom agent with MLflow tracing integration"
echo "  ✅ Real-time experiment logging to MLflow"
echo ""
echo "📊 View the experiments in MLflow UI:"
echo ""
echo -e "${GREEN}   https://data-science-gateway.apps.rosa.varsha.x2zf.p3.openshiftapps.com/mlflow/${NC}"
echo ""
echo "   Select workspace: opendatahub"
echo "   Experiment: kagenti-mlflow-demo"
echo ""

pause

#==============================================================================
# PART 9: (Optional) Agent Evaluation with MLflow GenAI
#==============================================================================

section "9. (Bonus) Agent Evaluation with MLflow GenAI"

echo "MLflow also supports evaluating GenAI applications using LLM-as-a-Judge."
echo ""
echo "Key capabilities (from https://mlflow.org/docs/latest/genai/eval-monitor/):"
echo "  📊 Dataset Management - Test cases and ground truth expectations"
echo "  👥 Human Feedback - Collect and track annotations"
echo "  🤖 LLM-as-a-Judge - Automated quality assessment using AI"
echo "  📈 Systematic Evaluation - Track and compare evaluation results"
echo "  🔍 Production Monitoring - Latency, token usage, quality metrics"
echo ""
echo "To run an evaluation locally:"
echo ""
echo -e "${YELLOW}  # Port-forward the agent${NC}"
echo -e "${YELLOW}  kubectl port-forward -n kagenti svc/simple-agent-mlflow-demo 8000:8000${NC}"
echo ""
echo -e "${YELLOW}  # Run the evaluation script${NC}"
echo -e "${YELLOW}  export OPENAI_API_KEY=your-key  # For LLM judges${NC}"
echo -e "${YELLOW}  python $SCRIPT_DIR/evaluate-agent.py${NC}"
echo ""
echo "The evaluation script demonstrates:"
echo "  - Defining evaluation datasets with expected responses"
echo "  - Running predictions through the agent"
echo "  - Using built-in scorers (Correctness, Guidelines)"
echo "  - Creating custom scorers for specific metrics"
echo "  - Tracking evaluation results in MLflow"
echo ""

echo "🔗 Resources:"
echo "   - Kagenti: https://github.com/kagenti/kagenti"
echo "   - MLflow Operator: https://github.com/opendatahub-io/mlflow-operator"
echo "   - MLflow GenAI Eval: https://mlflow.org/docs/latest/genai/eval-monitor/"
echo ""
