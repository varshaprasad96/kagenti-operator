#!/usr/bin/env python3
"""
Code Executor Agent for Kagenti
This agent can execute Python code in isolated sandboxes using agent-sandbox.
FastAPI-based REST API.
"""
import os
from typing import Dict, Any, Optional
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from agentic_sandbox import SandboxClient
import uvicorn

# Configuration from environment
SANDBOX_TEMPLATE = os.getenv("SANDBOX_TEMPLATE_NAME", "python-sandbox-template")
SANDBOX_NAMESPACE = os.getenv("SANDBOX_NAMESPACE", "kagenti")
SANDBOX_ROUTER_URL = os.getenv("SANDBOX_ROUTER_URL", "http://sandbox-router-svc.kagenti.svc.cluster.local:8080")

# Create FastAPI app
app = FastAPI(
    title="Code Executor Agent",
    description="Execute Python code in isolated sandboxes",
    version="1.0.0"
)


class CodeExecutionRequest(BaseModel):
    """Request model for code execution"""
    code: str
    language: str = "python"


class CodeExecutionResponse(BaseModel):
    """Response model for code execution"""
    stdout: str
    stderr: str
    exit_code: int
    success: bool


def execute_code_in_sandbox(code: str, language: str = "python") -> Dict[str, Any]:
    """
    Execute code in an isolated sandbox.
    Returns a dict with stdout, stderr, and exit_code.
    """
    try:
        with SandboxClient(
            template_name=SANDBOX_TEMPLATE,
            namespace=SANDBOX_NAMESPACE,
            api_url=SANDBOX_ROUTER_URL
        ) as sandbox:
            # Write code to a file
            filename = "code.py" if language == "python" else "code.sh"
            sandbox.write(filename, code)
            
            # Execute the code
            if language == "python":
                result = sandbox.run(f"python3 {filename}")
            else:
                result = sandbox.run(f"bash {filename}")
            
            return {
                "stdout": result.stdout,
                "stderr": result.stderr,
                "exit_code": result.exit_code,
                "success": result.exit_code == 0
            }
    except Exception as e:
        return {
            "stdout": "",
            "stderr": str(e),
            "exit_code": 1,
            "success": False
        }


@app.get("/")
async def root():
    """Health check endpoint"""
    return {
        "status": "healthy",
        "service": "code-executor-agent",
        "version": "1.0.0"
    }


@app.get("/health")
async def health():
    """Health check endpoint"""
    return {"status": "healthy"}


@app.post("/execute", response_model=CodeExecutionResponse)
async def execute_code(request: CodeExecutionRequest):
    """
    Execute code in an isolated sandbox.
    
    - **code**: The code to execute (Python or shell script)
    - **language**: Programming language ("python" or "bash", default: "python")
    """
    if not request.code.strip():
        raise HTTPException(status_code=400, detail="Code cannot be empty")
    
    result = execute_code_in_sandbox(request.code, request.language)
    return CodeExecutionResponse(**result)


@app.post("/execute/python")
async def execute_python_code(request: Dict[str, str]):
    """
    Execute Python code (convenience endpoint).
    
    Expects JSON body: {"code": "print('Hello, World!')"}
    """
    code = request.get("code", "")
    if not code:
        raise HTTPException(status_code=400, detail="Code field is required")
    
    result = execute_code_in_sandbox(code, language="python")
    return CodeExecutionResponse(**result)


if __name__ == "__main__":
    port = int(os.getenv("PORT", "8000"))
    host = os.getenv("HOST", "0.0.0.0")
    uvicorn.run(app, host=host, port=port)
