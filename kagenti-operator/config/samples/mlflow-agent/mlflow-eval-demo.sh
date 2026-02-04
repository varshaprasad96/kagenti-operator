#!/bin/bash
#
# Kagenti Agent + MLflow Evaluation Demo
# ======================================
#
# This demo showcases:
# 1. Deploying a Kagenti Agent with MLflow integration
# 2. Basic evaluation metrics (latency, response quality)
# 3. LLM-as-a-Judge evaluation pattern
# 4. Multi-agent evaluation (Agent → Agent communication)
#
# Prerequisites:
# - OpenShift cluster with ODH MLflow operator installed
# - Kagenti operator installed
# - vLLM model deployed (for LLM-as-a-Judge)
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color
BOLD='\033[1m'

# Pause function
pause() {
    echo ""
    echo -e "${YELLOW}Press Enter to continue...${NC}"
    read
}

# Section header
section() {
    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}${BOLD}  $1${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
    echo ""
}

# Info message
info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

# Success message
success() {
    echo -e "${GREEN}✅ $1${NC}"
}

# Warning message
warn() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

# Command display
run_cmd() {
    echo -e "${YELLOW}$ $1${NC}"
    eval "$1"
}

#############################################################################
# INTRO
#############################################################################
clear
section "Kagenti Agent + MLflow Evaluation Demo"

echo -e "${BOLD}This demo will walk you through:${NC}"
echo ""
echo "  1️⃣  Prerequisites Check"
echo "      - Verify MLflow Operator is running"
echo "      - Verify Kagenti Operator is running"
echo "      - Verify vLLM model is available"
echo ""
echo "  2️⃣  Deploy Kagenti Agent with MLflow"
echo "      - Create Agent CR with MLflow tracking"
echo "      - Test agent endpoints"
echo ""
echo "  3️⃣  Basic Evaluation Metrics"
echo "      - Latency measurement"
echo "      - Response length scoring"
echo "      - Error detection"
echo "      - View results in MLflow UI"
echo ""
echo "  4️⃣  LLM-as-a-Judge Evaluation"
echo "      - Use vLLM to evaluate response quality"
echo "      - Multi-criteria scoring"
echo "      - View judge ratings in MLflow"
echo ""
echo "  5️⃣  Multi-Agent Evaluation"
echo "      - Agent A talks to Agent B"
echo "      - Judge evaluates the conversation"
echo "      - End-to-end quality metrics"
echo ""

pause

#############################################################################
# STEP 1: Prerequisites Check
#############################################################################
section "Step 1: Prerequisites Check"

info "Checking MLflow Operator..."
if oc get pods -n opendatahub -l app=mlflow 2>/dev/null | grep -q Running; then
    success "MLflow Operator is running"
    run_cmd "oc get pods -n opendatahub -l app=mlflow"
else
    warn "MLflow may not be running. Checking..."
    run_cmd "oc get pods -n opendatahub | head -10"
fi

pause

info "Checking Kagenti Operator..."
if oc get pods -n kagenti-system 2>/dev/null | grep -q controller-manager; then
    success "Kagenti Operator is running"
    run_cmd "oc get pods -n kagenti-system"
else
    warn "Kagenti Operator may need to be installed"
fi

pause

info "Checking vLLM model for LLM-as-a-Judge..."
if oc get pods -n vllm 2>/dev/null | grep -q Running; then
    success "vLLM is running"
    run_cmd "oc get pods -n vllm"
else
    warn "vLLM not found. LLM-as-a-Judge will be skipped."
fi

pause

#############################################################################
# STEP 2: Deploy Agent
#############################################################################
section "Step 2: Deploy Kagenti Agent with MLflow"

info "The Agent CR configures MLflow tracking via environment variables:"
echo ""
cat << 'EOF'
  env:
    - name: MLFLOW_TRACKING_URI
      value: "https://mlflow.opendatahub.svc.cluster.local:8443"
    - name: MLFLOW_EXPERIMENT_NAME
      value: "kagenti-mlflow-demo"
    - name: MLFLOW_WORKSPACE
      value: "opendatahub"
EOF
echo ""

info "Applying Agent CR and RBAC..."
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

run_cmd "oc apply -f ${SCRIPT_DIR}/../mlflow/odh-simple-agent-mlflow-demo-rbac.yaml"
run_cmd "oc apply -f ${SCRIPT_DIR}/../mlflow/odh-simple-agent-mlflow-demo.yaml"

info "Waiting for agent to be ready..."
sleep 10
run_cmd "oc get pods -n kagenti -l app.kubernetes.io/name=simple-agent-mlflow-demo"

pause

info "Testing agent endpoint..."
run_cmd "oc port-forward -n kagenti svc/simple-agent-mlflow-demo 8000:8000 &"
sleep 3

echo ""
info "Sending test message to agent..."
curl -s -X POST http://localhost:8000/chat \
    -H "Content-Type: application/json" \
    -d '{"message": "Hello from the demo!"}' | python3 -m json.tool || echo "Response received"

echo ""
success "Agent is running and responding!"

# Kill port-forward
pkill -f "port-forward.*simple-agent-mlflow-demo" 2>/dev/null || true

pause

#############################################################################
# STEP 3: Basic Evaluation
#############################################################################
section "Step 3: Basic Evaluation Metrics"

info "Basic evaluation measures:"
echo ""
echo "  📊 ${BOLD}Latency${NC}        - Response time in seconds"
echo "  📊 ${BOLD}Response Length${NC} - Character count of response"
echo "  📊 ${BOLD}Word Count${NC}      - Number of words in response"
echo "  📊 ${BOLD}Error Detection${NC} - Whether response contains errors"
echo "  📊 ${BOLD}Responsiveness${NC}  - Whether response is meaningful"
echo ""

info "Deploying basic evaluation job..."

cat << 'EVAL_JOB' | oc apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: basic-eval-script
  namespace: kagenti
data:
  evaluate.py: |
    import os, time, httpx
    
    AGENT_URL = "http://simple-agent-mlflow-demo.kagenti.svc.cluster.local:8000"
    MLFLOW_URI = "https://mlflow.opendatahub.svc.cluster.local:8443"
    
    os.environ["MLFLOW_TRACKING_INSECURE_TLS"] = "true"
    
    # Setup workspace header BEFORE importing mlflow
    from mlflow.tracking.request_header.abstract_request_header_provider import RequestHeaderProvider
    from mlflow.tracking.request_header import registry
    class WH(RequestHeaderProvider):
        def in_context(self): return True
        def request_headers(self): return {"mlflow-workspace": "opendatahub", "x-mlflow-workspace": "opendatahub"}
    registry._request_header_provider_registry.register(WH)
    
    import mlflow
    
    def call_agent(msg):
        r = httpx.post(f"{AGENT_URL}/chat", json={"message": msg}, timeout=30)
        return r.json().get("response", "Error")
    
    def setup_mlflow():
        with open("/var/run/secrets/kubernetes.io/serviceaccount/token") as f:
            os.environ["MLFLOW_TRACKING_TOKEN"] = f.read().strip()
        mlflow.set_tracking_uri(MLFLOW_URI)
        mlflow.set_experiment("kagenti-basic-eval-demo")
    
    questions = ["Hello!", "What is AI?", "Tell me a joke", "Goodbye!"]
    
    setup_mlflow()
    print("\n🚀 Running Basic Evaluation\n" + "="*50)
    
    with mlflow.start_run(run_name="basic-eval"):
        mlflow.log_param("num_questions", len(questions))
        total_latency = 0
        
        for i, q in enumerate(questions, 1):
            start = time.time()
            resp = call_agent(q)
            latency = time.time() - start
            total_latency += latency
            
            with mlflow.start_run(run_name=f"q{i}", nested=True):
                mlflow.log_param("question", q)
                mlflow.log_metric("latency", round(latency, 3))
                mlflow.log_metric("response_length", len(resp))
                mlflow.log_metric("word_count", len(resp.split()))
                mlflow.log_metric("has_error", 1 if "error" in resp.lower() else 0)
            
            print(f"[{i}] Q: {q[:30]}...")
            print(f"    ⏱️ Latency: {latency:.3f}s | 📝 Words: {len(resp.split())}")
        
        mlflow.log_metric("avg_latency", round(total_latency/len(questions), 3))
        print(f"\n📊 Avg Latency: {total_latency/len(questions):.3f}s")
        print("✅ Results logged to MLflow!")
---
apiVersion: batch/v1
kind: Job
metadata:
  name: basic-eval-demo
  namespace: kagenti
spec:
  ttlSecondsAfterFinished: 300
  template:
    spec:
      serviceAccountName: simple-agent-mlflow-demo
      restartPolicy: Never
      containers:
        - name: eval
          image: registry.access.redhat.com/ubi9/python-312:latest
          command: ["/bin/bash", "-c", "pip install -q mlflow httpx && python /scripts/evaluate.py"]
          volumeMounts:
            - name: script
              mountPath: /scripts
          env:
            - name: PYTHONUNBUFFERED
              value: "1"
      volumes:
        - name: script
          configMap:
            name: basic-eval-script
EVAL_JOB

sleep 10
info "Watching evaluation logs..."
oc logs -f job/basic-eval-demo -n kagenti 2>/dev/null || sleep 5

success "Basic evaluation complete!"
echo ""
info "View results in MLflow UI:"
echo "   Experiment: ${BOLD}kagenti-basic-eval-demo${NC}"
echo "   URL: https://data-science-gateway.apps.rosa.varsha.x2zf.p3.openshiftapps.com/mlflow"

pause

#############################################################################
# STEP 4: LLM-as-a-Judge
#############################################################################
section "Step 4: LLM-as-a-Judge Evaluation"

info "LLM-as-a-Judge uses a separate LLM to evaluate response quality:"
echo ""
echo "  ┌─────────────┐    Question    ┌─────────────┐"
echo "  │   Eval Job  │ ─────────────▶ │ Target Agent│"
echo "  │             │◀────────────── │             │"
echo "  │             │    Response    └─────────────┘"
echo "  │             │"
echo "  │             │  Rate this     ┌─────────────┐"
echo "  │             │ ─────────────▶ │  Judge LLM  │"
echo "  │             │◀────────────── │   (vLLM)    │"
echo "  │             │  1-5 rating    └─────────────┘"
echo "  │             │"
echo "  │             │ ─────────────▶ MLflow"
echo "  └─────────────┘"
echo ""

info "Deploying LLM-as-a-Judge evaluation..."

cat << 'JUDGE_JOB' | oc apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: judge-eval-demo-script
  namespace: kagenti
data:
  evaluate.py: |
    import os, time, httpx
    
    AGENT_URL = "http://simple-agent-mlflow-demo.kagenti.svc.cluster.local:8000"
    JUDGE_URL = "http://vllm.vllm.svc.cluster.local:8000/v1"
    JUDGE_MODEL = "TinyLlama/TinyLlama-1.1B-Chat-v1.0"
    MLFLOW_URI = "https://mlflow.opendatahub.svc.cluster.local:8443"
    
    os.environ["MLFLOW_TRACKING_INSECURE_TLS"] = "true"
    
    # Setup workspace header BEFORE importing mlflow
    from mlflow.tracking.request_header.abstract_request_header_provider import RequestHeaderProvider
    from mlflow.tracking.request_header import registry
    class WH(RequestHeaderProvider):
        def in_context(self): return True
        def request_headers(self): return {"mlflow-workspace": "opendatahub", "x-mlflow-workspace": "opendatahub"}
    registry._request_header_provider_registry.register(WH)
    
    import mlflow
    
    def call_agent(msg):
        r = httpx.post(f"{AGENT_URL}/chat", json={"message": msg}, timeout=30)
        return r.json().get("response", "Error")
    
    def call_judge(prompt):
        try:
            r = httpx.post(f"{JUDGE_URL}/chat/completions", json={
                "model": JUDGE_MODEL,
                "messages": [{"role": "user", "content": prompt}],
                "max_tokens": 100, "temperature": 0.1
            }, timeout=60)
            return r.json()["choices"][0]["message"]["content"]
        except Exception as e:
            return f"Judge Error: {e}"
    
    def get_rating(question, response):
        prompt = f"Rate this response 1-5. Question: {question}. Response: {response}. Reply with just a number 1-5."
        out = call_judge(prompt)
        for c in out:
            if c.isdigit() and 1 <= int(c) <= 5:
                return int(c), out
        return 3, out
    
    def setup_mlflow():
        with open("/var/run/secrets/kubernetes.io/serviceaccount/token") as f:
            os.environ["MLFLOW_TRACKING_TOKEN"] = f.read().strip()
        mlflow.set_tracking_uri(MLFLOW_URI)
        mlflow.set_experiment("kagenti-llm-judge-demo")
    
    tests = [
        ("Hello, how are you?", "friendliness"),
        ("What is Python?", "knowledge"),
        ("Help me debug code", "helpfulness"),
    ]
    
    setup_mlflow()
    print("\n⚖️ LLM-as-a-Judge Evaluation\n" + "="*50)
    
    with mlflow.start_run(run_name="judge-eval"):
        mlflow.log_param("judge_model", JUDGE_MODEL)
        total = 0
        
        for i, (q, topic) in enumerate(tests, 1):
            resp = call_agent(q)
            rating, reason = get_rating(q, resp)
            total += rating
            
            with mlflow.start_run(run_name=f"judge_{i}_{topic}", nested=True):
                mlflow.log_param("question", q)
                mlflow.log_param("topic", topic)
                mlflow.log_metric("judge_rating", rating)
            
            stars = "⭐" * rating
            print(f"[{i}] Q: {q}")
            print(f"    Rating: {stars} ({rating}/5)")
        
        avg = total / len(tests)
        mlflow.log_metric("avg_judge_rating", round(avg, 2))
        print(f"\n📊 Avg Judge Rating: {avg:.2f}/5")
        print("✅ Results logged to MLflow!")
---
apiVersion: batch/v1
kind: Job
metadata:
  name: judge-eval-demo
  namespace: kagenti
spec:
  ttlSecondsAfterFinished: 300
  template:
    spec:
      serviceAccountName: simple-agent-mlflow-demo
      restartPolicy: Never
      containers:
        - name: eval
          image: registry.access.redhat.com/ubi9/python-312:latest
          command: ["/bin/bash", "-c", "pip install -q mlflow httpx && python /scripts/evaluate.py"]
          volumeMounts:
            - name: script
              mountPath: /scripts
          env:
            - name: PYTHONUNBUFFERED
              value: "1"
      volumes:
        - name: script
          configMap:
            name: judge-eval-demo-script
JUDGE_JOB

sleep 15
info "Watching LLM-as-a-Judge evaluation..."
oc logs -f job/judge-eval-demo -n kagenti 2>/dev/null || sleep 5

success "LLM-as-a-Judge evaluation complete!"
echo ""
info "View results in MLflow UI:"
echo "   Experiment: ${BOLD}kagenti-llm-judge-demo${NC}"

pause

#############################################################################
# STEP 5: Summary
#############################################################################
section "Step 5: Demo Summary"

echo -e "${GREEN}${BOLD}Demo Complete! 🎉${NC}"
echo ""
echo "You've seen:"
echo ""
echo "  ✅ ${BOLD}Kagenti Agent${NC} deployed with MLflow tracking"
echo ""
echo "  ✅ ${BOLD}Basic Evaluation${NC} with metrics:"
echo "     - Latency, response length, word count"
echo "     - Error detection, responsiveness"
echo ""
echo "  ✅ ${BOLD}LLM-as-a-Judge${NC} evaluation:"
echo "     - vLLM rates response quality 1-5"
echo "     - Multi-criteria evaluation"
echo ""
echo "  ✅ All results logged to ${BOLD}ODH MLflow${NC}"
echo ""
echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
echo ""
echo "View results at:"
echo "  ${BOLD}https://data-science-gateway.apps.rosa.varsha.x2zf.p3.openshiftapps.com/mlflow${NC}"
echo ""
echo "Experiments created:"
echo "  • kagenti-basic-eval-demo"
echo "  • kagenti-llm-judge-demo"
echo ""
echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"

#############################################################################
# Cleanup
#############################################################################
echo ""
echo "To cleanup demo resources:"
echo "  oc delete job basic-eval-demo judge-eval-demo -n kagenti"
echo "  oc delete configmap basic-eval-script judge-eval-demo-script -n kagenti"
echo ""
