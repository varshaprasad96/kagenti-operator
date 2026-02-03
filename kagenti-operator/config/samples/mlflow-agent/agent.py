"""
Simple MLflow Agent with Kubernetes Auth (Non-blocking)
"""
import os
import time
import logging
import threading
from datetime import datetime
from fastapi import FastAPI
from pydantic import BaseModel

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = FastAPI(title="Simple MLflow Agent", version="1.0.0")


class MLflowTracker:
    def __init__(self):
        self.tracking_uri = os.getenv("MLFLOW_TRACKING_URI", "http://localhost:5000")
        self.experiment_name = os.getenv("MLFLOW_EXPERIMENT_NAME", "simple-mlflow-agent")
        self.insecure_tls = os.getenv("MLFLOW_TRACKING_INSECURE_TLS", "false").lower() == "true"
        self.request_count = 0
        self.success_count = 0
        self.total_latency = 0.0
        self.enabled = False
        self.mlflow = None
        self.init_attempted = False
        self.init_lock = threading.Lock()
        
        # Start background initialization
        threading.Thread(target=self._init_mlflow_background, daemon=True).start()
    
    def _get_k8s_token(self):
        """Read Kubernetes service account token."""
        token_path = "/var/run/secrets/kubernetes.io/serviceaccount/token"
        try:
            with open(token_path, "r") as f:
                return f.read().strip()
        except Exception as e:
            return None
    
    def _init_mlflow_background(self):
        """Initialize MLflow in background."""
        with self.init_lock:
            if self.init_attempted:
                return
            self.init_attempted = True
        
        try:
            import mlflow
            self.mlflow = mlflow
            
            if self.insecure_tls:
                import urllib3
                urllib3.disable_warnings()
                os.environ["MLFLOW_TRACKING_INSECURE_TLS"] = "true"
            
            token = self._get_k8s_token()
            if token:
                os.environ["MLFLOW_TRACKING_TOKEN"] = token
                logger.info("Using K8s token for MLflow auth")
            
            os.environ["MLFLOW_HTTP_REQUEST_TIMEOUT"] = "5"
            
            mlflow.set_tracking_uri(self.tracking_uri)
            mlflow.set_experiment(self.experiment_name)
            logger.info(f"MLflow initialized: {self.tracking_uri}")
            self.enabled = True
        except Exception as e:
            logger.warning(f"MLflow init failed: {e}")
            self.enabled = False
    
    def log_request(self, task_name: str, latency: float, success: bool, metadata: dict = None):
        self.request_count += 1
        self.total_latency += latency
        if success:
            self.success_count += 1
        
        # Always log locally
        logger.info(f"[Request] {task_name}: latency={latency:.3f}s, count={self.request_count}")
        
        # Log to MLflow in background if enabled
        if self.enabled and self.mlflow:
            threading.Thread(
                target=self._log_to_mlflow,
                args=(task_name, latency, success, metadata),
                daemon=True
            ).start()
    
    def _log_to_mlflow(self, task_name: str, latency: float, success: bool, metadata: dict):
        try:
            with self.mlflow.start_run(run_name=f"{task_name}_{self.request_count}"):
                self.mlflow.log_param("task_name", task_name)
                if metadata:
                    for k, v in metadata.items():
                        self.mlflow.log_param(k, str(v)[:250])
                self.mlflow.log_metrics({
                    "latency_seconds": latency,
                    "success": 1 if success else 0,
                    "request_count": self.request_count,
                    "success_rate": self.success_count / max(self.request_count, 1),
                    "avg_latency": self.total_latency / max(self.request_count, 1),
                })
            logger.info(f"[MLflow] Logged: {task_name}")
        except Exception as e:
            logger.warning(f"[MLflow] Failed: {e}")


tracker = MLflowTracker()


class MessageRequest(BaseModel):
    message: str


@app.get("/")
async def root():
    return {
        "name": "Simple MLflow Agent",
        "mlflow_enabled": tracker.enabled,
        "mlflow_uri": tracker.tracking_uri,
        "stats": {
            "total_requests": tracker.request_count,
            "success_rate": tracker.success_count / max(tracker.request_count, 1),
            "avg_latency_ms": (tracker.total_latency / max(tracker.request_count, 1)) * 1000,
        }
    }


@app.get("/health")
async def health():
    return {"status": "healthy"}


@app.post("/chat")
async def chat(request: MessageRequest):
    start_time = time.time()
    response = f"Hello! You said: '{request.message}' | Request #{tracker.request_count + 1} | MLflow: {'ON' if tracker.enabled else 'OFF'}"
    latency = time.time() - start_time
    tracker.log_request("chat", latency, True, {"message_length": len(request.message)})
    return {
        "response": response,
        "latency_ms": latency * 1000,
        "mlflow_enabled": tracker.enabled,
        "request_id": tracker.request_count
    }


@app.post("/generate")
async def generate(request: MessageRequest):
    """Simulate LLM generation with metrics."""
    import asyncio
    start_time = time.time()
    await asyncio.sleep(0.1)  # Simulate 100ms latency
    response = f"Generated: {request.message[:50]}..."
    latency = time.time() - start_time
    tracker.log_request("generate", latency, True, {
        "input_tokens": len(request.message.split()),
        "output_tokens": len(response.split()),
    })
    return {"response": response, "latency_ms": latency * 1000, "request_id": tracker.request_count}


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)
