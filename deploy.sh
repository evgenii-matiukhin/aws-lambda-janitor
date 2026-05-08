#!/usr/bin/env bash
set -euo pipefail

: "${GITHUB_ORG:?GITHUB_ORG must be set}"
: "${GITHUB_REPO:?GITHUB_REPO must be set}"
: "${CDK_DEFAULT_ACCOUNT:?CDK_DEFAULT_ACCOUNT must be set}"

AWS_REGION="${CDK_DEFAULT_REGION:-us-east-1}"

echo "Synthesizing CDK stacks..."
cdk synth --all

echo "Deploying CDK stacks..."
cdk deploy --all --require-approval never

echo "Deployment complete."
aws cloudformation describe-stacks \
  --stack-name lambda-janitor \
  --region "${AWS_REGION}" \
  --query 'Stacks[0].Outputs' \
  --output table 2>/dev/null || true
