#!/bin/bash
# Creates the buckets the product expects, once LocalStack reports ready.
#
# Buckets are created here rather than by application code: a service that can
# create its own buckets needs credentials it should not hold in a deployed
# environment, where Terraform creates them with versioning, encryption and
# lifecycle rules instead.
set -euo pipefail

for bucket in prepeet-media prepeet-documents prepeet-exports; do
  awslocal s3api create-bucket \
    --bucket "$bucket" \
    --region eu-west-2 \
    --create-bucket-configuration LocationConstraint=eu-west-2 >/dev/null 2>&1 || true
done

# Recordings are versioned so a deletion is recoverable within the retention
# window and an overwrite cannot silently destroy evidence.
awslocal s3api put-bucket-versioning \
  --bucket prepeet-media \
  --versioning-configuration Status=Enabled >/dev/null 2>&1 || true

echo "prepeet: buckets ready"
