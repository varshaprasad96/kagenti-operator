# Integrating Agent-Sandbox with Kagenti Agent

This guide shows how to integrate agent-sandbox with your Kagenti agent to execute code in isolated sandboxes.

## Overview

Your Kagenti agent can use agent-sandbox to:
- Execute code/commands in isolated, secure sandboxes
- Maintain persistent state across executions
- Scale sandbox creation/cleanup automatically

## Architecture

```
Kagenti Agent → SandboxClient → SandboxRouter → Sandbox Pod
```

1. Your Kagenti agent calls `SandboxClient` (Python SDK)
2. `SandboxClient` creates a `SandboxClaim` → creates a `Sandbox` → creates a Pod
3. Requests are routed through the `sandbox-router` to the sandbox pod
4. Code executes in the isolated sandbox
5. Results are returned to your agent

## Prerequisites

1. **Agent Sandbox Controller installed** in your cluster
   ```bash
   export VERSION="v0.1.0"  # Use latest version from releases
   kubectl apply -f https://github.com/kubernetes-sigs/agent-sandbox/releases/download/${VERSION}/extensions.yaml
   ```

2. **Sandbox Router deployed** (see Step 2 below)

3. **Python SDK installed** in your Kagenti agent environment

## Step 1: Create SandboxTemplate

Apply the sandbox template:

```bash
kubectl apply -f kagenti-operator/config/samples/python-sandbox-template.yaml
```

Verify:
```bash
kubectl get sandboxtemplate -n kagenti
```

## Step 2: Deploy Sandbox Router

The sandbox router routes requests from agents to sandbox pods.

**Option A: Build and Deploy Sandbox Router**

### Step 2.1: Clone the Agent-Sandbox Repository

```bash
# Clone the repository
git clone https://github.com/kubernetes-sigs/agent-sandbox.git
cd agent-sandbox/clients/python/agentic-sandbox-client/sandbox-router
```

### Step 2.2: Build the Sandbox Router Image

Choose one of the following methods:

**Method 1: Using Docker (if you have Docker installed)**

```bash
# Build the image
docker build -t sandbox-router:latest .

# If using a registry (recommended for production):
# Replace 'your-registry' with your container registry (e.g., ghcr.io/your-org, docker.io/your-org)
docker build -t your-registry/sandbox-router:v0.1.0 .
docker push your-registry/sandbox-router:v0.1.0
```

**Method 2: Using Buildah/Podman**

```bash
# Build the image
buildah bud -t sandbox-router:latest .

# Push to registry (if needed)
buildah push sandbox-router:latest your-registry/sandbox-router:v0.1.0
```

**Method 3: Using Kind (for local development)**

If you're using Kind for local development:

```bash
# Build the image
docker build -t sandbox-router:latest .

# Load into Kind cluster
kind load docker-image sandbox-router:latest --name <your-cluster-name>
```

### Step 2.3: Update the Deployment YAML

Edit `sandbox-router-deployment.yaml` and update the image:

```bash
# If using local image (Kind/local cluster)
# Change line 25 from:
#   image: sandbox-router:latest  # TODO: Replace with actual image
# To:
#   image: sandbox-router:latest

# If using a registry image:
# Change line 25 to:
#   image: your-registry/sandbox-router:v0.1.0
```

Or use `sed` to update it:

```bash
# For local/kind image
sed -i '' 's|image: sandbox-router:latest  # TODO: Replace with actual image|image: sandbox-router:latest|' \
  kagenti-operator/config/samples/sandbox-router-deployment.yaml

# For registry image (replace with your registry)
sed -i '' 's|image: sandbox-router:latest  # TODO: Replace with actual image|image: ghcr.io/your-org/sandbox-router:v0.1.0|' \
  kagenti-operator/config/samples/sandbox-router-deployment.yaml
```

### Step 2.4: Apply the Deployment

```bash
kubectl apply -f kagenti-operator/config/samples/sandbox-router-deployment.yaml
```

**Option B: Deploy Manually (Alternative)**

If you prefer to deploy manually or customize the deployment, follow the instructions in the [sandbox-router README](https://github.com/kubernetes-sigs/agent-sandbox/tree/main/clients/python/agentic-sandbox-client/sandbox-router).

Verify router is running:
```bash
kubectl get pods -n kagenti -l app=sandbox-router
kubectl get svc -n kagenti sandbox-router-svc
```

## Step 3: Create RBAC for Agent

Apply RBAC permissions:

```bash
kubectl apply -f kagenti-operator/config/samples/code-executor-agent-rbac.yaml
```

This grants the agent permissions to:
- Create and manage SandboxClaims
- Read SandboxTemplates
- Read Sandboxes and Pods

## Step 4: Build Agent Image with Sandbox Support

A complete example agent project is provided in `code-executor-agent/` directory. You can use it as-is or customize it.

### Option A: Use the Provided Example Project (Recommended)

The example project includes:
- `agent_code.py` - Complete agent implementation with sandbox support
- `Dockerfile` - Ready-to-use container image definition
- `requirements.txt` - Python dependencies
- `README.md` - Build and deployment instructions

**Step 4.1: Navigate to the project directory**

```bash
cd kagenti-operator/config/samples/code-executor-agent
```

**Step 4.2: Build the image**

```bash
# For local/kind development
docker build -t code-executor-agent:latest .

# For production (with registry)
docker build -t your-registry/code-executor-agent:v1.0.0 .
docker push your-registry/code-executor-agent:v1.0.0
```

**Step 4.3: Load into Kind (if using Kind)**

```bash
kind load docker-image code-executor-agent:latest --name <your-cluster-name>
```

**Step 4.4: Update the Agent deployment YAML**

Edit `code-executor-agent.yaml` and update the image references:

```bash
# Navigate back to samples directory
cd ..

# Update the image in the YAML file
sed -i '' 's|image: "ghcr.io/kagenti/agent-examples/weather_service:v0.0.1-alpha.2"|image: "code-executor-agent:latest"|g' \
  code-executor-agent.yaml
```

Or manually edit `code-executor-agent.yaml` and replace both image references (lines 30 and 44) with your built image.

### Option B: Create Your Own Agent Project

If you prefer to create your own project:

1. **Create a new directory:**
   ```bash
   mkdir my-code-executor-agent
   cd my-code-executor-agent
   ```

2. **Create `agent_code.py`** (see Step 5 for example code)

3. **Create `Dockerfile`:**
   ```dockerfile
   FROM python:3.11-slim
   WORKDIR /app
   RUN pip install --no-cache-dir \
       "git+https://github.com/kubernetes-sigs/agent-sandbox.git@main#subdirectory=clients/python/agentic-sandbox-client" \
       a2a-server uvicorn fastapi
   COPY agent_code.py /app/
   EXPOSE 8000
   CMD ["python", "/app/agent_code.py"]
   ```

4. **Build and deploy** (same as Option A)

## Step 5: Example Agent Code

Here's example Python code for your agent that uses SandboxClient:

```python
# agent_code.py
from agentic_sandbox import SandboxClient
import os
import json
from a2a.server.apps.jsonrpc import jsonrpc_app

# Configuration from environment
SANDBOX_TEMPLATE = os.getenv("SANDBOX_TEMPLATE_NAME", "python-sandbox-template")
SANDBOX_NAMESPACE = os.getenv("SANDBOX_NAMESPACE", "kagenti")
SANDBOX_ROUTER_URL = os.getenv("SANDBOX_ROUTER_URL", "http://sandbox-router-svc.kagenti.svc.cluster.local:8080")

def execute_code_in_sandbox(code: str, language: str = "python") -> dict:
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

@jsonrpc_app.method("message/send")
async def handle_message(request: dict) -> dict:
    """
    Handle JSON-RPC messages from Kagenti agent.
    """
    params = request.get("params", {})
    message = params.get("message", {})
    parts = message.get("parts", [])
    
    if not parts:
        return {
            "jsonrpc": "2.0",
            "id": request.get("id"),
            "error": {"code": -32602, "message": "Invalid params: no message parts"}
        }
    
    text = parts[0].get("text", "")
    message_id = message.get("messageId", "unknown")
    
    # Check if this is a code execution request
    if text.startswith("execute:"):
        code = text.replace("execute:", "").strip()
        result = execute_code_in_sandbox(code)
        
        response_text = f"Code execution result:\n"
        response_text += f"Exit Code: {result['exit_code']}\n"
        if result['stdout']:
            response_text += f"Output:\n{result['stdout']}\n"
        if result['stderr']:
            response_text += f"Errors:\n{result['stderr']}\n"
        
        return {
            "jsonrpc": "2.0",
            "id": request.get("id"),
            "result": {
                "artifacts": [{
                    "artifactId": f"exec-{message_id}",
                    "parts": [{
                        "kind": "text",
                        "text": response_text
                    }]
                }],
                "contextId": message.get("contextId", "default"),
                "history": [message],
                "id": message_id,
                "kind": "task",
                "status": {
                    "state": "completed" if result['success'] else "failed",
                    "timestamp": "2026-01-12T00:00:00Z"
                }
            }
        }
    
    # Handle other message types (e.g., weather queries, etc.)
    return {
        "jsonrpc": "2.0",
        "id": request.get("id"),
        "result": {
            "artifacts": [{
                "artifactId": f"response-{message_id}",
                "parts": [{
                    "kind": "text",
                    "text": f"I received your message: {text}"
                }]
            }],
            "contextId": message.get("contextId", "default"),
            "history": [message],
            "id": message_id,
            "kind": "task",
            "status": {
                "state": "completed",
                "timestamp": "2026-01-12T00:00:00Z"
            }
        }
    }

if __name__ == "__main__":
    # Start the A2A JSON-RPC server
    import uvicorn
    port = int(os.getenv("PORT", "8000"))
    uvicorn.run(jsonrpc_app, host="0.0.0.0", port=port)
```

## Step 6: Deploy the Agent

Update `code-executor-agent.yaml` with your built image, then deploy:

```bash
# Update the image in code-executor-agent.yaml first!
kubectl apply -f kagenti-operator/config/samples/code-executor-agent.yaml
```

Verify:
```bash
kubectl get agent code-executor-agent -n kagenti
kubectl get pods -n kagenti -l app.kubernetes.io/name=code-executor-agent
```

## Step 7: Test the Integration

Test code execution:

```bash
kubectl run test-code-exec --image=curlimages/curl:8.1.2 --rm -i --restart=Never -n kagenti -- \
  curl -sS -X POST http://code-executor-agent.kagenti.svc.cluster.local:8000/ \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc":"2.0",
    "id":"test-1",
    "method":"message/send",
    "params":{
      "message":{
        "role":"user",
        "parts":[{
          "kind":"text",
          "text":"execute: print(\"Hello from sandbox!\"); print(2+2)"
        }],
        "messageId":"msg-1"
      }
    }
  }'
```

Expected response should include the execution output.

## Troubleshooting

1. **Sandbox not created**: Check SandboxClaims:
   ```bash
   kubectl get sandboxclaims -n kagenti
   kubectl describe sandboxclaim <name> -n kagenti
   ```

2. **Router connection issues**: Verify router is running:
   ```bash
   kubectl get pods -n kagenti -l app=sandbox-router
   kubectl logs -n kagenti -l app=sandbox-router
   ```

3. **Template not found**: Ensure template exists:
   ```bash
   kubectl get sandboxtemplate -n kagenti
   ```

4. **RBAC issues**: Check service account permissions:
   ```bash
   kubectl auth can-i create sandboxclaims --as=system:serviceaccount:kagenti:code-executor-agent-sa -n kagenti
   ```

5. **Agent logs**: Check agent logs for errors:
   ```bash
   kubectl logs -n kagenti -l app.kubernetes.io/name=code-executor-agent
   ```

## Next Steps

- See [agent-sandbox documentation](https://github.com/kubernetes-sigs/agent-sandbox) for advanced usage
- Explore persistent sandboxes for stateful code execution
- Add more language support (bash, node, etc.) by creating additional SandboxTemplates
