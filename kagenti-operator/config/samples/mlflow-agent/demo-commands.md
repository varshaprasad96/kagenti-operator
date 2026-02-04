# Kagenti + MLflow Integration Demo Commands

Copy and paste these commands during your demo recording.

---

## 1. Verify MLflow Operator Installation

```bash
# Check MLflow operator pod
oc get pods -n opendatahub -l app.kubernetes.io/name=mlflow-operator

# Check MLflow instance
oc get mlflow -n opendatahub

# Check MLflow deployment
oc get pods -n opendatahub -l app.kubernetes.io/name=mlflow
```

---

## 2. Verify Kagenti Operator Installation

```bash
# Check Kagenti controller manager
oc get pods -n kagenti-system -l control-plane=controller-manager

# Check controller logs
oc logs -n kagenti-system deploy/kagenti-controller-manager --tail=5
```

---

## 3. Show Agent CRDs

```bash
# List Kagenti CRDs
oc get crd | grep kagenti

# Explain Agent spec
oc explain agent.spec --recursive | head -30
```

---

## 4. Show the Custom Agent Definition

```bash
# View the agent manifest
cat kagenti-operator/config/samples/mlflow/odh-simple-agent-mlflow-demo.yaml
```

Key configuration points to highlight:
- `MLFLOW_TRACKING_URI`: Points to ODH MLflow service
- `MLFLOW_EXPERIMENT_NAME`: Experiment name in MLflow
- `MLFLOW_WORKSPACE`: Maps to Kubernetes namespace

---

## 5. Deploy the Agent

```bash
# First, apply RBAC for MLflow access
oc apply -f kagenti-operator/config/samples/mlflow/odh-simple-agent-mlflow-demo-rbac.yaml

# Apply the agent
oc apply -f kagenti-operator/config/samples/mlflow/odh-simple-agent-mlflow-demo.yaml

# Check agent status
oc get agent -n kagenti simple-agent-mlflow-demo

# Check pod is running
oc get pods -n kagenti -l app.kubernetes.io/name=simple-agent-mlflow-demo

# Verify MLflow initialization in logs
oc logs -n kagenti deploy/simple-agent-mlflow-demo --tail=10
```

---

## 6. Interact with the Agent

```bash
# Send test message 1
oc run demo-1 -n kagenti --rm -i --image=curlimages/curl:8.6.0 --restart=Never -- \
  curl -sS -X POST http://simple-agent-mlflow-demo.kagenti.svc.cluster.local:8000/chat \
  -H 'Content-Type: application/json' \
  -d '{"message":"Hello! What can you help me with?"}'

# Send test message 2
oc run demo-2 -n kagenti --rm -i --image=curlimages/curl:8.6.0 --restart=Never -- \
  curl -sS -X POST http://simple-agent-mlflow-demo.kagenti.svc.cluster.local:8000/chat \
  -H 'Content-Type: application/json' \
  -d '{"message":"Explain the benefits of MLflow tracking"}'

# Send test message 3
oc run demo-3 -n kagenti --rm -i --image=curlimages/curl:8.6.0 --restart=Never -- \
  curl -sS -X POST http://simple-agent-mlflow-demo.kagenti.svc.cluster.local:8000/chat \
  -H 'Content-Type: application/json' \
  -d '{"message":"How does Kagenti integrate with MLflow?"}'
```

---

## 7. Verify MLflow Logging

```bash
# Check agent logs for MLflow runs
oc logs -n kagenti deploy/simple-agent-mlflow-demo --tail=30 | grep -E 'View run|Logged|Request'
```

Expected output:
```
🏃 View run chat_1 at: https://mlflow.opendatahub.svc.cluster.local:8443/#/experiments/3/runs/xxx
INFO:__main__:[MLflow] Logged: chat
```

---

## 8. Open MLflow UI

Navigate to:
```
https://data-science-gateway.apps.rosa.varsha.x2zf.p3.openshiftapps.com/mlflow/
```

1. Select workspace: **opendatahub**
2. Find experiment: **kagenti-mlflow-demo**
3. View runs with metrics: `latency_seconds`, `success_rate`, `request_count`

---

## Quick One-Liner Test (for demos)

```bash
for i in 1 2 3; do
  oc run test-$i -n kagenti --rm -i --quiet --image=curlimages/curl:8.6.0 --restart=Never -- \
    curl -sS -X POST http://simple-agent-mlflow-demo.kagenti.svc.cluster.local:8000/chat \
    -H 'Content-Type: application/json' \
    -d "{\"message\":\"Test message $i\"}"
  echo ""
  sleep 1
done
oc logs -n kagenti deploy/simple-agent-mlflow-demo --tail=15 | grep -E 'View run|Logged'
```

---

## Cleanup (Optional)

```bash
# Delete the agent
oc delete agent -n kagenti simple-agent-mlflow-demo

# Delete RBAC
oc delete -f kagenti-operator/config/samples/mlflow/odh-simple-agent-mlflow-demo-rbac.yaml

# Verify cleanup
oc get pods -n kagenti
```

---

## 9. (Bonus) Agent Evaluation with MLflow GenAI

MLflow supports evaluating GenAI applications using LLM-as-a-Judge.
Reference: https://mlflow.org/docs/latest/genai/eval-monitor/

### Key Capabilities

- **Dataset Management** - Test cases and ground truth expectations
- **Human Feedback** - Collect and track annotations
- **LLM-as-a-Judge** - Automated quality assessment using AI
- **Systematic Evaluation** - Track and compare evaluation results
- **Production Monitoring** - Latency, token usage, quality metrics

### Run Evaluation Locally

```bash
# Port-forward the agent
kubectl port-forward -n kagenti svc/simple-agent-mlflow-demo 8000:8000

# Set OpenAI API key for LLM judges (optional)
export OPENAI_API_KEY=your-key

# Run the evaluation script
python kagenti-operator/config/samples/mlflow-agent/evaluate-agent.py
```

### Evaluation Script Overview

The evaluation script demonstrates:

```python
import mlflow
from mlflow.genai.scorers import Correctness, Guidelines

# Define evaluation dataset
dataset = [
    {
        "inputs": {"question": "What is MLflow?"},
        "expectations": {"expected_response": "MLflow is an ML platform..."},
    },
]

# Define prediction function
def predict_fn(question: str) -> str:
    return call_agent(question)

# Run evaluation with LLM judges
results = mlflow.genai.evaluate(
    data=dataset,
    predict_fn=predict_fn,
    scorers=[
        Correctness(),  # Built-in LLM judge
        Guidelines(name="is_professional", guidelines="..."),
    ],
)
```

---

## Resources

- Kagenti: https://github.com/kagenti/kagenti
- MLflow Operator: https://github.com/opendatahub-io/mlflow-operator
- MLflow GenAI Eval: https://mlflow.org/docs/latest/genai/eval-monitor/
