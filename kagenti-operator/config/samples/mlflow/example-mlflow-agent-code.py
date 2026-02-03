"""
Example MLflow Integration for Kagenti Agents

This example shows how to integrate MLflow tracking into a Kagenti agent
for logging:
- LLM call latency and token usage
- Agent task metrics (success/failure rates)
- Model performance over time
- Custom agent-specific metrics

To use this in your agent, set these environment variables:
  MLFLOW_TRACKING_URI=http://mlflow-tracking.mlflow.svc.cluster.local:5000
  MLFLOW_EXPERIMENT_NAME=your-agent-name

Dependencies: pip install mlflow opentelemetry-api opentelemetry-sdk
"""

import os
import time
import functools
from typing import Any, Callable
from contextlib import contextmanager

import mlflow
from mlflow.tracking import MlflowClient

# -------------------------------------------------------------------
# MLflow Setup
# -------------------------------------------------------------------

def setup_mlflow_tracking():
    """Initialize MLflow tracking for the agent."""
    tracking_uri = os.getenv("MLFLOW_TRACKING_URI", "http://localhost:5000")
    experiment_name = os.getenv("MLFLOW_EXPERIMENT_NAME", "kagenti-agent")
    
    mlflow.set_tracking_uri(tracking_uri)
    
    # Create or get experiment
    experiment = mlflow.get_experiment_by_name(experiment_name)
    if experiment is None:
        experiment_id = mlflow.create_experiment(
            experiment_name,
            tags={
                "platform": "kagenti",
                "type": "agent",
            }
        )
    else:
        experiment_id = experiment.experiment_id
    
    mlflow.set_experiment(experiment_name)
    print(f"MLflow tracking initialized: {tracking_uri}, experiment: {experiment_name}")
    return experiment_id


# -------------------------------------------------------------------
# LLM Metrics Tracking
# -------------------------------------------------------------------

class LLMMetricsTracker:
    """Track LLM call metrics with MLflow."""
    
    def __init__(self, model_name: str = None):
        self.model_name = model_name or os.getenv("LLM_MODEL", "unknown")
        self.client = MlflowClient()
    
    @contextmanager
    def track_llm_call(self, task_name: str = "llm_call"):
        """Context manager to track an LLM call."""
        start_time = time.time()
        metrics = {
            "input_tokens": 0,
            "output_tokens": 0,
            "total_tokens": 0,
            "success": 0,
            "error": 0,
        }
        
        try:
            yield metrics
            metrics["success"] = 1
        except Exception as e:
            metrics["error"] = 1
            raise
        finally:
            latency = time.time() - start_time
            metrics["latency_seconds"] = latency
            
            # Log to MLflow
            with mlflow.start_run(nested=True, run_name=task_name):
                mlflow.log_params({
                    "model": self.model_name,
                    "task": task_name,
                })
                mlflow.log_metrics({
                    "llm_latency_seconds": latency,
                    "input_tokens": metrics["input_tokens"],
                    "output_tokens": metrics["output_tokens"],
                    "total_tokens": metrics["total_tokens"],
                    "success": metrics["success"],
                    "error": metrics["error"],
                })
    
    def log_token_usage(self, input_tokens: int, output_tokens: int):
        """Log token usage from LLM response."""
        mlflow.log_metrics({
            "input_tokens": input_tokens,
            "output_tokens": output_tokens,
            "total_tokens": input_tokens + output_tokens,
        })


# -------------------------------------------------------------------
# Agent Task Metrics
# -------------------------------------------------------------------

class AgentTaskTracker:
    """Track agent task execution metrics."""
    
    def __init__(self, agent_name: str):
        self.agent_name = agent_name
        self.task_count = 0
        self.success_count = 0
        self.failure_count = 0
    
    def track_task(self, task_type: str = "generic"):
        """Decorator to track task execution."""
        def decorator(func: Callable) -> Callable:
            @functools.wraps(func)
            async def async_wrapper(*args, **kwargs):
                return await self._execute_and_track(func, task_type, *args, **kwargs)
            
            @functools.wraps(func)
            def sync_wrapper(*args, **kwargs):
                return self._execute_and_track_sync(func, task_type, *args, **kwargs)
            
            import asyncio
            if asyncio.iscoroutinefunction(func):
                return async_wrapper
            return sync_wrapper
        return decorator
    
    async def _execute_and_track(self, func, task_type, *args, **kwargs):
        """Execute async function and track metrics."""
        start_time = time.time()
        self.task_count += 1
        
        try:
            result = await func(*args, **kwargs)
            self.success_count += 1
            success = True
        except Exception as e:
            self.failure_count += 1
            success = False
            raise
        finally:
            duration = time.time() - start_time
            self._log_task_metrics(task_type, duration, success)
        
        return result
    
    def _execute_and_track_sync(self, func, task_type, *args, **kwargs):
        """Execute sync function and track metrics."""
        start_time = time.time()
        self.task_count += 1
        
        try:
            result = func(*args, **kwargs)
            self.success_count += 1
            success = True
        except Exception as e:
            self.failure_count += 1
            success = False
            raise
        finally:
            duration = time.time() - start_time
            self._log_task_metrics(task_type, duration, success)
        
        return result
    
    def _log_task_metrics(self, task_type: str, duration: float, success: bool):
        """Log task metrics to MLflow."""
        with mlflow.start_run(nested=True, run_name=f"task_{task_type}"):
            mlflow.log_params({
                "agent": self.agent_name,
                "task_type": task_type,
            })
            mlflow.log_metrics({
                "task_duration_seconds": duration,
                "task_success": 1 if success else 0,
                "task_failure": 0 if success else 1,
                "total_tasks": self.task_count,
                "total_successes": self.success_count,
                "total_failures": self.failure_count,
                "success_rate": self.success_count / self.task_count if self.task_count > 0 else 0,
            })


# -------------------------------------------------------------------
# Example Weather Agent with MLflow
# -------------------------------------------------------------------

class WeatherAgentWithMLflow:
    """Example weather agent with full MLflow integration."""
    
    def __init__(self):
        self.experiment_id = setup_mlflow_tracking()
        self.llm_tracker = LLMMetricsTracker()
        self.task_tracker = AgentTaskTracker("weather-agent")
    
    @property
    def get_weather(self):
        """Get weather with tracking - using property to apply decorator."""
        @self.task_tracker.track_task("get_weather")
        async def _get_weather(self, location: str) -> dict:
            """Get weather for a location with full tracking."""
            
            # Start a parent run for the entire operation
            with mlflow.start_run(run_name=f"weather_request_{location}"):
                mlflow.log_param("location", location)
                
                # Track the LLM call
                with self.llm_tracker.track_llm_call("weather_llm_processing") as metrics:
                    # Simulate LLM call to process weather request
                    result = await self._call_llm(location)
                    
                    # Update token metrics from response
                    metrics["input_tokens"] = result.get("input_tokens", 0)
                    metrics["output_tokens"] = result.get("output_tokens", 0)
                    metrics["total_tokens"] = metrics["input_tokens"] + metrics["output_tokens"]
                
                # Log the weather data
                weather_data = result.get("weather", {})
                mlflow.log_metrics({
                    "temperature": weather_data.get("temperature", 0),
                    "humidity": weather_data.get("humidity", 0),
                })
                
                return weather_data
        
        return _get_weather
    
    async def _call_llm(self, prompt: str) -> dict:
        """Simulated LLM call - replace with actual LLM client."""
        # In real implementation, this would call the LLM API
        await asyncio.sleep(0.1)  # Simulate latency
        return {
            "input_tokens": 50,
            "output_tokens": 100,
            "weather": {
                "temperature": 72,
                "humidity": 45,
                "condition": "sunny",
            }
        }
    
    def log_agent_session_stats(self):
        """Log overall agent session statistics."""
        with mlflow.start_run(run_name="session_summary"):
            mlflow.log_metrics({
                "session_total_tasks": self.task_tracker.task_count,
                "session_successes": self.task_tracker.success_count,
                "session_failures": self.task_tracker.failure_count,
                "session_success_rate": (
                    self.task_tracker.success_count / self.task_tracker.task_count
                    if self.task_tracker.task_count > 0 else 0
                ),
            })


# -------------------------------------------------------------------
# Integration with OpenTelemetry (for combined tracing)
# -------------------------------------------------------------------

def setup_otel_mlflow_integration():
    """
    Set up OpenTelemetry integration with MLflow.
    This allows correlating OTEL traces with MLflow runs.
    """
    from opentelemetry import trace
    from opentelemetry.sdk.trace import TracerProvider
    from opentelemetry.sdk.trace.export import BatchSpanProcessor
    from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
    
    # Get OTEL endpoint from environment
    otel_endpoint = os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4317")
    
    # Set up tracer provider
    provider = TracerProvider()
    processor = BatchSpanProcessor(OTLPSpanExporter(endpoint=otel_endpoint))
    provider.add_span_processor(processor)
    trace.set_tracer_provider(provider)
    
    return trace.get_tracer("kagenti-agent")


# -------------------------------------------------------------------
# Usage Example
# -------------------------------------------------------------------

if __name__ == "__main__":
    import asyncio
    
    # Initialize
    agent = WeatherAgentWithMLflow()
    
    # Example usage
    async def main():
        # Get weather with full tracking
        weather = await agent.get_weather("San Francisco")
        print(f"Weather: {weather}")
        
        # Log session stats
        agent.log_agent_session_stats()
    
    asyncio.run(main())
    
    print("\n✅ Check MLflow UI at http://localhost:30500 for tracked metrics!")
