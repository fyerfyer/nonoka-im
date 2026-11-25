#!/usr/bin/env bash
set -euo pipefail

# Enable Bifrost dashboard authentication after the first boot. Run once with
# the setup token, then remove BIFROST_SETUP_TOKEN from the Nonoka/agent-worker
# environment and use BIFROST_ADMIN_USERNAME/BIFROST_ADMIN_PASSWORD.
: "${BIFROST_URL:?BIFROST_URL is required}"
: "${BIFROST_SETUP_TOKEN:?BIFROST_SETUP_TOKEN is required}"
: "${BIFROST_ADMIN_USERNAME:?BIFROST_ADMIN_USERNAME is required}"
: "${BIFROST_ADMIN_PASSWORD:?BIFROST_ADMIN_PASSWORD is required}"

curl -fsS -X PUT "${BIFROST_URL%/}/api/config" \
  -H "X-Bifrost-Setup-Token: ${BIFROST_SETUP_TOKEN}" \
  -H 'Content-Type: application/json' \
  -d "{
    \"auth_config\": {
      \"is_enabled\": true,
      \"admin_username\": \"${BIFROST_ADMIN_USERNAME}\",
      \"admin_password\": \"${BIFROST_ADMIN_PASSWORD}\"
    },
    \"client\": {
      \"enforce_auth_on_inference\": true,
      \"disable_content_logging\": true
    }
  }" >/dev/null

echo "Bifrost dashboard auth enabled. Remove BIFROST_SETUP_TOKEN from runtime env and use admin credentials."
