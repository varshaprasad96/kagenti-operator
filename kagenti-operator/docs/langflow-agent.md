# Langflow Agent Deployment

This guide shows how to package a Langflow flow as a container and deploy it with the Kagenti Operator.
Langflow supports deploying flows as an API or MCP server; use Langflow's docs for the exact runtime options you want. See the Langflow repository for details.  
https://github.com/langflow-ai/langflow

## 1) Create a Langflow flow

- Build your flow in the Langflow UI.
- Export the flow to JSON (for example `flow.json`) and commit it to your agent repo.

## 2) Create a lightweight agent repo

Your repo should include:

```
my-langflow-agent/
  flow.json
  requirements.txt
  start.sh
```

Example `requirements.txt`:

```
langflow
```

Example `start.sh`:

```
#!/usr/bin/env bash
set -euo pipefail

# Run the Langflow server on the container port.
# Replace with the exact options you want (API/MCP, auth, etc).
langflow run --host 0.0.0.0 --port "${PORT:-7860}"
```

Commit this repo to GitHub (or another git host your cluster can reach).

## 3) Build and deploy with Kagenti

Use the sample below to build the image with AgentBuild and then deploy it with Agent.
Replace placeholders with your repo and registry settings.

See `config/samples/langflow-agent-build-and-deploy.yaml` for a full example.

