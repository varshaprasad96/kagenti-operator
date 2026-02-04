#!/usr/bin/env python3
"""
Agent Evaluation Script using MLflow GenAI

This script demonstrates the official MLflow GenAI agent evaluation workflow:
https://mlflow.org/docs/latest/genai/eval-monitor/running-evaluation/agents/

Features:
1. Uses mlflow.genai.evaluate() API
2. Custom scorers with @scorer decorator
3. Dataset with inputs, expectations, and tags
4. Traces and evaluation results logged to MLflow UI

Prerequisites:
- pip install 'mlflow[genai]>=3.3' httpx openai
- Agent running and accessible via port-forward

Usage:
    # Start MLflow server locally
    mlflow server --port 5000
    
    # Port-forward the agent
    oc port-forward -n kagenti svc/simple-agent-mlflow-demo 8000:8000
    
    # Run evaluation
    export OPENAI_API_KEY=your-key  # Optional, for LLM judges
    python evaluate-agent.py
"""

import os
import httpx

# Configuration
AGENT_URL = os.getenv("AGENT_URL", "http://localhost:8000")
MLFLOW_TRACKING_URI = os.getenv(
    "MLFLOW_TRACKING_URI",
    "https://data-science-gateway.apps.rosa.varsha.x2zf.p3.openshiftapps.com/mlflow"
)
MLFLOW_WORKSPACE = os.getenv("MLFLOW_WORKSPACE", "opendatahub")

# Disable SSL verification for self-signed certs
os.environ["MLFLOW_TRACKING_INSECURE_TLS"] = "true"


def setup_mlflow_workspace():
    """Register workspace header provider for ODH MLflow."""
    try:
        from mlflow.tracking.request_header.abstract_request_header_provider import (
            RequestHeaderProvider,
        )
        from mlflow.tracking.request_header import registry as header_registry

        workspace = MLFLOW_WORKSPACE

        class WorkspaceHeaderProvider(RequestHeaderProvider):
            def in_context(self):
                return True

            def request_headers(self):
                return {
                    "mlflow-workspace": workspace,
                    "x-mlflow-workspace": workspace,
                }

        header_registry._request_header_provider_registry.register(WorkspaceHeaderProvider)
        print(f"✅ MLflow workspace header registered: {workspace}")
    except Exception as e:
        print(f"⚠️ Could not register workspace header: {e}")

print("\n" + "=" * 60)
print("Kagenti Agent Evaluation with MLflow GenAI")
print("=" * 60)
print(f"\nAgent URL: {AGENT_URL}")
print(f"MLflow URI: {MLFLOW_TRACKING_URI}")


def call_agent(message: str) -> str:
    """Call the Kagenti agent and return the response text."""
    try:
        response = httpx.post(
            f"{AGENT_URL}/chat",
            json={"message": message},
            timeout=10.0
        )
        response.raise_for_status()
        data = response.json()
        return data.get("response", "No response")
    except Exception as e:
        return f"Error: {e}"


# ============================================================================
# Step 1: Define the prediction function
# ============================================================================
def predict_fn(question: str) -> str:
    """Prediction function that MLflow calls for each test case."""
    return call_agent(question)


# ============================================================================
# Step 2: Create evaluation dataset
# Dataset format: inputs, expectations, and optional tags
# ============================================================================
eval_dataset = [
    {
        "inputs": {"question": "Hello, how are you?"},
        "expectations": {"should_greet": True, "topic": "greeting"},
        "tags": {"category": "greeting"},
    },
    {
        "inputs": {"question": "What is MLflow?"},
        "expectations": {
            "should_mention": ["mlflow", "ml", "experiment", "tracking"],
            "topic": "knowledge",
        },
        "tags": {"category": "knowledge"},
    },
    {
        "inputs": {"question": "Can you help me with Python code?"},
        "expectations": {"should_offer_help": True, "topic": "assistance"},
        "tags": {"category": "assistance"},
    },
    {
        "inputs": {"question": "What is 2 + 2?"},
        "expectations": {"answer": "4", "topic": "math"},
        "tags": {"category": "math"},
    },
    {
        "inputs": {"question": "Explain Kubernetes in simple terms"},
        "expectations": {"topic": "knowledge", "should_be_simple": True},
        "tags": {"category": "knowledge"},
    },
]


# ============================================================================
# Step 3: Define custom scorers
# ============================================================================
def define_scorers():
    """Define custom scorers for agent evaluation."""
    import mlflow
    from mlflow.genai import scorer
    
    @scorer
    def response_length(outputs: str) -> int:
        """Measures the response length."""
        return len(outputs) if outputs else 0
    
    @scorer
    def has_error(outputs: str) -> bool:
        """Checks if the response contains an error."""
        return "error" in outputs.lower() if outputs else True
    
    @scorer
    def mentions_expected_topics(outputs: str, expectations: dict) -> bool:
        """Checks if the response mentions expected keywords."""
        if not outputs:
            return False
        
        should_mention = expectations.get("should_mention", [])
        if not should_mention:
            return True
        
        output_lower = outputs.lower()
        return any(keyword.lower() in output_lower for keyword in should_mention)
    
    @scorer
    def response_quality(outputs: str) -> dict:
        """Composite scorer for response quality metrics."""
        if not outputs:
            return {"has_content": False, "word_count": 0, "is_helpful": False}
        
        word_count = len(outputs.split())
        has_content = word_count > 3
        is_helpful = word_count > 10 and "error" not in outputs.lower()
        
        return {
            "has_content": has_content,
            "word_count": word_count,
            "is_helpful": is_helpful,
        }
    
    return [response_length, has_error, mentions_expected_topics, response_quality]


# ============================================================================
# Step 4: Run the evaluation
# ============================================================================
def run_evaluation():
    """Run MLflow GenAI evaluation."""
    import mlflow
    
    # Setup MLflow with workspace headers for ODH
    setup_mlflow_workspace()
    mlflow.set_tracking_uri(MLFLOW_TRACKING_URI)
    mlflow.set_experiment("kagenti-agent-evaluation")
    
    print("\n🔍 Testing agent connectivity...")
    test_response = call_agent("test")
    if "Error" in test_response:
        print(f"❌ Cannot connect to agent: {test_response}")
        print("\nMake sure to port-forward the agent first:")
        print("  oc port-forward -n kagenti svc/simple-agent-mlflow-demo 8000:8000")
        return
    print("✅ Agent is reachable")
    
    print("\n" + "=" * 60)
    print("Running MLflow GenAI Evaluation")
    print("=" * 60 + "\n")
    
    # Define scorers
    scorers = define_scorers()
    print(f"📊 Scorers: {[s.__name__ if hasattr(s, '__name__') else str(s) for s in scorers]}")
    print(f"📋 Test cases: {len(eval_dataset)}")
    
    # Run evaluation
    print("\n🚀 Starting evaluation...")
    
    try:
        results = mlflow.genai.evaluate(
            data=eval_dataset,
            predict_fn=predict_fn,
            scorers=scorers,
        )
        
        print("\n✅ Evaluation complete!")
        print(f"\n📊 Results summary:")
        print(results.tables["eval_results"])
        
        print(f"\n🔗 View detailed results in MLflow UI:")
        print(f"   {MLFLOW_TRACKING_URI}")
        print(f"   Experiment: kagenti-agent-evaluation")
        
    except Exception as e:
        import traceback
        print(f"\n❌ Evaluation failed: {e}")
        print(f"\nFull traceback:")
        traceback.print_exc()
        print("\nFalling back to simple evaluation...")
        run_simple_evaluation()


def run_simple_evaluation():
    """Fallback simple evaluation without mlflow.genai."""
    print("\n" + "=" * 60)
    print("Running Simple Evaluation")
    print("=" * 60 + "\n")
    
    for i, item in enumerate(eval_dataset):
        question = item["inputs"]["question"]
        print(f"[{i+1}/{len(eval_dataset)}] 📤 {question}")
        
        response = predict_fn(question)
        print(f"    🤖 {response[:80]}...")
        
        # Simple metrics
        has_error = "error" in response.lower()
        word_count = len(response.split())
        
        print(f"    📊 Words: {word_count} | Status: {'❌ Error' if has_error else '✅ OK'}\n")
    
    print("✅ Simple evaluation complete!")


if __name__ == "__main__":
    # For ODH MLflow, use simple evaluation (mlflow.genai.evaluate requires local tracing)
    # Set USE_GENAI_EVAL=true to try mlflow.genai.evaluate()
    use_genai = os.getenv("USE_GENAI_EVAL", "false").lower() == "true"
    
    try:
        import mlflow
        print(f"\n📦 MLflow version: {mlflow.__version__}")
        
        if use_genai and hasattr(mlflow, 'genai'):
            print("🔬 Using mlflow.genai.evaluate() (experimental with ODH)")
            run_evaluation()
        else:
            print("📊 Using simple evaluation (recommended for ODH MLflow)")
            run_simple_evaluation()
            
    except ImportError as e:
        print(f"⚠️ MLflow not installed: {e}")
        run_simple_evaluation()
