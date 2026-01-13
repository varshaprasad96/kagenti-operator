# Code Executor Agent Dockerfile
# This agent can execute Python code in isolated sandboxes using FastAPI

# Use Python base image
FROM python:3.11-slim

# Set working directory
WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    git \
    && rm -rf /var/lib/apt/lists/*

# Install Python dependencies
RUN pip install --no-cache-dir \
    fastapi \
    uvicorn \
    pydantic

# Install agent-sandbox Python SDK
RUN pip install --no-cache-dir \
    "git+https://github.com/kubernetes-sigs/agent-sandbox.git@main#subdirectory=clients/python/agentic-sandbox-client"

# Copy your agent code
COPY agent_code.py /app/agent_code.py

# Expose port
EXPOSE 8000

# Run the FastAPI app
CMD ["python", "/app/agent_code.py"]
