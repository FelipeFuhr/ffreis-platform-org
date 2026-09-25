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
  source = "git::https://github.com/FelipeFuhr/ffreis-platform-terraform-modules.git//modules/s3-bucket?ref=f828c757a5c837a33675e0f383f988f93d4f3387"

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

  tags = merge(local.common_tags, {
    Name    = "${var.org}-tf-state-runtime"
    Purpose = "terraform-state"
    Tier    = "runtime"
    Layer   = "platform-org"
    Stack   = "platform-org"
  })
}

module "tf_locks_runtime" {
  source = "git::https://github.com/FelipeFuhr/ffreis-platform-terraform-modules.git//modules/dynamodb-table?ref=f828c757a5c837a33675e0f383f988f93d4f3387"

  name     = "${var.org}-tf-locks-runtime"
  hash_key = "LockID"

  tags = merge(local.common_tags, {
    Name    = "${var.org}-tf-locks-runtime"
    Purpose = "terraform-locks"
    Tier    = "runtime"
    Layer   = "platform-org"
    Stack   = "platform-org"
  })
}

# ---------------------------------------------------------------------------
# Bootstrap state buckets — created by platform-bootstrap CLI and adopted
# here for tag management. These three buckets hold Terraform state for the
# entire platform (root=bootstrap layer, prod/dev=workload environments).
#
# Only tags are managed by Terraform. All other bucket configuration
# (versioning, encryption, public-access-block) is owned by the bootstrap
# CLI's EnsureStateBucket — ignore_changes prevents drift on those attributes.
# ---------------------------------------------------------------------------

import {
  to = aws_s3_bucket.tf_state_root
  id = "${var.org}-tf-state-root"
}

resource "aws_s3_bucket" "tf_state_root" {
  bucket        = "${var.org}-tf-state-root"
  force_destroy = false

  # ── Checkov: this resource manages TAGS ONLY ───────────────────────────────
  # The bucket itself is created and owned by the platform-bootstrap CLI's
  # EnsureStateBucket (see the header comment above); `ignore_changes` keeps
  # Terraform from fighting it. Checkov reads this HCL statically and therefore
  # cannot see settings that genuinely exist on the live bucket. Declaring them
  # here to satisfy the scan would re-introduce exactly the drift this
  # ownership split was designed to prevent.
  # Verified live 2026-09-25 with `aws s3api` against ffreis-tf-state-root:
  #   versioning=Enabled  public-access-block=all four true  encryption=AES256
  #checkov:skip=CKV_AWS_21:Versioning IS Enabled on the live bucket (verified); owned by the bootstrap CLI, not declared here.
  #checkov:skip=CKV2_AWS_6:Public access block IS applied to the live bucket, all four settings true (verified); owned by the bootstrap CLI.
  #checkov:skip=CKV_AWS_18:S3 server access logging bills a PUT plus storage per request against a bucket reached only by Terraform via narrowly-scoped IAM; data-plane access is already auditable through CloudTrail.
  #checkov:skip=CKV_AWS_144:Cross-region replication would double storage and add a second bucket plus IAM role. State is versioned and this is not the only copy -- the infrastructure is reproducible from git.
  #checkov:skip=CKV2_AWS_62:Event notifications are an integration feature, not a security control; nothing in this stack reacts to object events on a state bucket.
  #checkov:skip=CKV2_AWS_61:A lifecycle rule expiring objects on a Terraform state bucket would be actively harmful -- version history IS the recovery mechanism.
  #checkov:skip=CKV_AWS_145:Encrypted with AES256 (verified live), so the data IS encrypted at rest -- this check is about key OWNERSHIP. A customer-managed KMS key is $1/month against this fleet's ~$0 fixed-cost target, and the fleet's standing no-KMS decision names AES256 as the sanctioned alternative. REVISIT IF this bucket starts holding regulated data.

  lifecycle {
    prevent_destroy = true
    ignore_changes  = [object_lock_enabled]
  }

  tags = merge(local.common_tags, {
    Name    = "${var.org}-tf-state-root"
    Purpose = "terraform-state"
    Tier    = "root"
    Layer   = "bootstrap"
    Stack   = "bootstrap"
  })
}

import {
  to = aws_s3_bucket.tf_state_prod
  id = "${var.org}-tf-state-prod"
}

resource "aws_s3_bucket" "tf_state_prod" {
  bucket        = "${var.org}-tf-state-prod"
  force_destroy = false

  # ── Checkov: this resource manages TAGS ONLY ───────────────────────────────
  # The bucket itself is created and owned by the platform-bootstrap CLI's
  # EnsureStateBucket (see the header comment above); `ignore_changes` keeps
  # Terraform from fighting it. Checkov reads this HCL statically and therefore
  # cannot see settings that genuinely exist on the live bucket. Declaring them
  # here to satisfy the scan would re-introduce exactly the drift this
  # ownership split was designed to prevent.
  # Verified live 2026-09-25 with `aws s3api` against ffreis-tf-state-prod:
  #   versioning=Enabled  public-access-block=all four true  encryption=aws:kms (AWS-managed key)
  #checkov:skip=CKV_AWS_21:Versioning IS Enabled on the live bucket (verified); owned by the bootstrap CLI, not declared here.
  #checkov:skip=CKV2_AWS_6:Public access block IS applied to the live bucket, all four settings true (verified); owned by the bootstrap CLI.
  #checkov:skip=CKV_AWS_18:S3 server access logging bills a PUT plus storage per request against a bucket reached only by Terraform via narrowly-scoped IAM; data-plane access is already auditable through CloudTrail.
  #checkov:skip=CKV_AWS_144:Cross-region replication would double storage and add a second bucket plus IAM role. State is versioned and this is not the only copy -- the infrastructure is reproducible from git.
  #checkov:skip=CKV2_AWS_62:Event notifications are an integration feature, not a security control; nothing in this stack reacts to object events on a state bucket.
  #checkov:skip=CKV2_AWS_61:A lifecycle rule expiring objects on a Terraform state bucket would be actively harmful -- version history IS the recovery mechanism.
  #checkov:skip=CKV_AWS_145:Encrypted with aws:kms using the AWS-managed key (verified live), which already satisfies this check in substance -- checkov cannot see it because encryption is not declared in this HCL. Not converted to a customer-managed key: that is $1/month/key against a ~$0 fixed-cost target, and the fleet's 2026-07 KMS audit deliberately accepted AWS-managed-key usage on existing buckets rather than touching live state storage.

  lifecycle {
    prevent_destroy = true
    ignore_changes  = [object_lock_enabled]
  }

  tags = merge(local.common_tags, {
    Name    = "${var.org}-tf-state-prod"
    Purpose = "terraform-state"
    Tier    = "prod"
    Layer   = "platform-org"
    Stack   = "platform-org"
  })
}

import {
  to = aws_s3_bucket.tf_state_dev
  id = "${var.org}-tf-state-dev"
}

resource "aws_s3_bucket" "tf_state_dev" {
  bucket        = "${var.org}-tf-state-dev"
  force_destroy = false

  # ── Checkov: this resource manages TAGS ONLY ───────────────────────────────
  # The bucket itself is created and owned by the platform-bootstrap CLI's
  # EnsureStateBucket (see the header comment above); `ignore_changes` keeps
  # Terraform from fighting it. Checkov reads this HCL statically and therefore
  # cannot see settings that genuinely exist on the live bucket. Declaring them
  # here to satisfy the scan would re-introduce exactly the drift this
  # ownership split was designed to prevent.
  # Verified live 2026-09-25 with `aws s3api` against ffreis-tf-state-dev:
  #   versioning=Enabled  public-access-block=all four true  encryption=aws:kms (AWS-managed key)
  #checkov:skip=CKV_AWS_21:Versioning IS Enabled on the live bucket (verified); owned by the bootstrap CLI, not declared here.
  #checkov:skip=CKV2_AWS_6:Public access block IS applied to the live bucket, all four settings true (verified); owned by the bootstrap CLI.
  #checkov:skip=CKV_AWS_18:S3 server access logging bills a PUT plus storage per request against a bucket reached only by Terraform via narrowly-scoped IAM; data-plane access is already auditable through CloudTrail.
  #checkov:skip=CKV_AWS_144:Cross-region replication would double storage and add a second bucket plus IAM role. State is versioned and this is not the only copy -- the infrastructure is reproducible from git.
  #checkov:skip=CKV2_AWS_62:Event notifications are an integration feature, not a security control; nothing in this stack reacts to object events on a state bucket.
  #checkov:skip=CKV2_AWS_61:A lifecycle rule expiring objects on a Terraform state bucket would be actively harmful -- version history IS the recovery mechanism.
  #checkov:skip=CKV_AWS_145:Encrypted with aws:kms using the AWS-managed key (verified live), which already satisfies this check in substance -- checkov cannot see it because encryption is not declared in this HCL. Not converted to a customer-managed key: that is $1/month/key against a ~$0 fixed-cost target, and the fleet's 2026-07 KMS audit deliberately accepted AWS-managed-key usage on existing buckets rather than touching live state storage.

  lifecycle {
    prevent_destroy = true
    ignore_changes  = [object_lock_enabled]
  }

  # Environment and lifecycle override: this is the dev state bucket.
  tags = merge(local.common_tags, {
    Name           = "${var.org}-tf-state-dev"
    Purpose        = "terraform-state"
    Tier           = "dev"
    Layer          = "platform-org"
    Stack          = "platform-org"
    Environment    = "dev"
    LifecycleState = "development"
  })
}
