# ---------------------------------------------------------------------------
# Runtime state backend — used by workload stacks (separate from root).
# Uses the shared platform-terraform-modules/modules/s3-bucket and
# dynamodb-table modules to enforce org-wide best practices automatically:
#   - public access block (all 4 settings)
#   - AES-256 server-side encryption
#   - TLS-only bucket policy (deny HTTP)
#   - versioning
#   - DynamoDB PITR
#   - DynamoDB SSE
# ---------------------------------------------------------------------------

module "tf_state_runtime" {
  source = "git::https://github.com/FelipeFuhr/ffreis-platform-terraform-modules.git//modules/s3-bucket?ref=e8bc4e4fdbc22306c8045c5f74deb67745a6e29b"

  bucket                = "${var.org}-tf-state-runtime"
  versioning_enabled    = true
  sse_algorithm         = "AES256"
  logging_target_bucket = ""
  force_destroy         = false

  # Expire noncurrent state versions after 90 days to contain storage costs.
  lifecycle_rules = [
    {
      id                                 = "expire-noncurrent-state"
      enabled                            = true
      noncurrent_version_expiration_days = 90
    },
  ]

  tags = merge(var.tags, {
    Name    = "${var.org}-tf-state-runtime"
    Purpose = "terraform-state"
    Tier    = "runtime"
    Layer   = "platform-org"
    Stack   = "platform-org"
  })
}

module "tf_locks_runtime" {
  source = "git::https://github.com/FelipeFuhr/ffreis-platform-terraform-modules.git//modules/dynamodb-table?ref=e8bc4e4fdbc22306c8045c5f74deb67745a6e29b"

  name     = "${var.org}-tf-locks-runtime"
  hash_key = "LockID"

  tags = merge(var.tags, {
    Name    = "${var.org}-tf-locks-runtime"
    Purpose = "terraform-locks"
    Tier    = "runtime"
    Layer   = "platform-org"
    Stack   = "platform-org"
  })
}
