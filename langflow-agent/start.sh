#!/usr/bin/env bash
set -euo pipefail

# Run Langflow UI/API so you can import flow.json after deployment.
langflow run --host 0.0.0.0 --port "${PORT:-7860}"
