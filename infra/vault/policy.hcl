# Vault policy for AI-Box sandboxes
path "aibox/data/git-token/*" {
  capabilities = ["read"]
}

path "aibox/data/llm-api-key" {
  capabilities = ["read"]
}

path "aibox/data/mirror-token" {
  capabilities = ["read"]
}

path "sys/leases/revoke" {
  capabilities = ["update"]
}
