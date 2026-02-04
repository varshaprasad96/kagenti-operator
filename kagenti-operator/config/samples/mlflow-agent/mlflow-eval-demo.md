# Kagenti Agent + MLflow Evaluation Demo

This demo showcases agent evaluation patterns with MLflow on OpenShift.

## Overview

| Component | Description |
|-----------|-------------|
| **Kagenti Agent** | Simple agent with MLflow tracking enabled |
| **ODH MLflow** | MLflow Operator for experiment tracking |
| **vLLM** | TinyLlama model for LLM-as-a-Judge |
| **Evaluation Jobs** | Kubernetes Jobs that run evaluations |

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                    OpenShift Cluster                          │
├──────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐     │
│  │   Kagenti   │     │  Target     │     │   Judge     │     │
│  │  Operator   │────▶│   Agent     │◀────│   (vLLM)    │     │
│  └─────────────┘     └──────┬──────┘     └─────────────┘     │
│                             │                                 │
│                             ▼                                 │
│                      ┌─────────────┐                         │
│                      │  ODH MLflow │                         │
│                      │   Tracking  │                         │
│                      └─────────────┘                         │
│                                                               │
└──────────────────────────────────────────────────────────────┘
```

---

## Step 1: Prerequisites Check

### Check MLflow Operator
```bash
oc get pods -n opendatahub -l app=mlflow
```

### Check Kagenti Operator
```bash
oc get pods -n kagenti-system
```

### Check vLLM (for LLM-as-a-Judge)
```bash
oc get pods -n vllm
```

---

## Step 2: Deploy Agent with MLflow

### Apply RBAC
```bash
oc apply -f kagenti-operator/config/samples/mlflow/odh-simple-agent-mlflow-demo-rbac.yaml
```

### Apply Agent CR
```bash
oc apply -f kagenti-operator/config/samples/mlflow/odh-simple-agent-mlflow-demo.yaml
```

### Verify Agent
```bash
oc get pods -n kagenti -l app.kubernetes.io/name=simple-agent-mlflow-demo
```

### Test Agent
```bash
# Port forward
oc port-forward -n kagenti svc/simple-agent-mlflow-demo 8000:8000 &

# Send test message
curl -X POST http://localhost:8000/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "Hello!"}'
```

---

## Step 3: Basic Evaluation Metrics

Basic evaluation measures:
- **Latency** - Response time in seconds
- **Response Length** - Character count
- **Word Count** - Number of words
- **Error Detection** - Whether response contains errors
- **Responsiveness** - Whether response is meaningful

### Run Basic Evaluation
```bash
oc apply -f kagenti-operator/config/samples/mlflow-agent/evaluate-job.yaml
oc logs -f job/agent-evaluation -n kagenti
```

### View in MLflow
- Experiment: `kagenti-agent-evaluation`
- Metrics: `latency_seconds`, `word_count`, `has_error`, `is_responsive`

---

## Step 4: LLM-as-a-Judge Evaluation

LLM-as-a-Judge uses a separate LLM to evaluate response quality.

### How It Works
1. Send question to Target Agent
2. Get response from Target Agent
3. Send response + criteria to Judge LLM
4. Judge returns rating (1-5) with reasoning
5. Log rating to MLflow

### Run LLM-as-a-Judge
```bash
oc apply -f kagenti-operator/config/samples/mlflow-agent/evaluate-with-judge.yaml
oc logs -f job/agent-judge-eval -n kagenti
```

### View in MLflow
- Experiment: `kagenti-agent-to-agent-eval`
- Metrics: `judge_rating`, `avg_judge_rating`
- Parameters: `question`, `target_response`, `judge_reason`

---

## Step 5: Multi-Agent Evaluation

Multi-agent evaluation extends LLM-as-a-Judge to full agent-to-agent conversations.

```
┌─────────────┐     Question      ┌─────────────┐
│  Eval Job   │ ─────────────────▶│ Target Agent│
│             │◀───────────────── │             │
│             │     Response      └─────────────┘
│             │
│             │  "Rate this       ┌─────────────┐
│             │   response"       │  Judge LLM  │
│             │ ─────────────────▶│   (vLLM)    │
│             │◀───────────────── │             │
│             │  Rating + Reason  └─────────────┘
│             │
│             │ ─────────────────▶ ODH MLflow
└─────────────┘
```

---

## Evaluation Metrics Reference

### Basic Metrics
| Metric | Type | Description |
|--------|------|-------------|
| `latency_seconds` | float | Response time |
| `response_length` | int | Character count |
| `word_count` | int | Word count |
| `has_error` | int | 1 if error, 0 if ok |
| `is_responsive` | int | 1 if meaningful |

### Judge Metrics
| Metric | Type | Description |
|--------|------|-------------|
| `judge_rating` | int | 1-5 quality score |
| `avg_judge_rating` | float | Average across tests |

### Parameters Logged
| Parameter | Description |
|-----------|-------------|
| `question` | Input question |
| `response_snippet` | Agent response (truncated) |
| `expected_topic` | Expected topic category |
| `judge_reason` | Judge's explanation |

---

## Cleanup

```bash
# Delete evaluation jobs
oc delete job agent-evaluation agent-judge-eval -n kagenti

# Delete configmaps
oc delete configmap evaluate-script judge-eval-script -n kagenti

# Delete agent
oc delete agent simple-agent-mlflow-demo -n kagenti
oc delete -f kagenti-operator/config/samples/mlflow/odh-simple-agent-mlflow-demo-rbac.yaml
```

---

## MLflow UI

View all results at:
```
https://data-science-gateway.apps.rosa.varsha.x2zf.p3.openshiftapps.com/mlflow
```

Select workspace: `opendatahub`

Experiments:
- `kagenti-mlflow-demo` - Agent tracing
- `kagenti-agent-evaluation` - Basic metrics
- `kagenti-agent-to-agent-eval` - LLM-as-a-Judge
