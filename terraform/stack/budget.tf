# ── Org-wide budgets ─────────────────────────────────────────────────────────
# INTENTIONALLY UNFILTERED. This stack runs in the AWS Organization payer
# account, so a budget here with no cost_filter tracks TOTAL spend across every
# member account — present and future — via consolidated billing. These are the
# org-level safety net: they catch untagged spend and any new project before it
# gets its own budget, so nothing untracked can spiral.
#
# Per-project attribution lives elsewhere: each product's infra stack
# (petlook-infra, ffreis-flemming-infra, ffreis-platform-shared-infra, …) owns a
# CostCenter-tag-scoped monthly budget. Those budgets and these are complementary
# — do NOT add cost filters here, and do NOT add more unfiltered budgets in the
# product stacks (that was the 2026-06 bug: every product budget tracked the
# whole account and tripped together).
#
# Two tiers:
#   - admin   ($30): early tripwire, ~2.5x normal fleet spend (minimal-cost mode ~$10-12/mo)
#   - ceiling ($60): higher hard ceiling for a genuine runaway

# Recipient resolution. `bootstrap fetch` writes BOTH keys into
# fetched.auto.tfvars.json: `budget_alert_emails` (the full recipient list —
# what this stack reads) and `budget_alert_email` (the first recipient only,
# kept for consumers still on the old contract).
#
# Preferring the list while falling back to the singular is what makes a STALE
# fetched.auto.tfvars.json safe. A file written before bootstrap learned to emit
# the list carries only the singular; without this fallback that file would
# resolve to an empty recipient set and quietly disarm the org-wide spend
# tripwire. Nobody re-runs `make fetch` just because they are applying an
# unrelated change, so a stale file is the expected case, not the edge case.
#
# Deprecation path, in order: (1) every consumer reads the list, (2) drop
# var.budget_alert_email and this fallback here, (3) drop the singular key from
# fetchedConfig in ffreis-platform-bootstrap. Doing (3) first would break any
# consumer still on step (1).
#
# If BOTH keys are absent the fallback yields an empty list, and each budget
# below refuses to plan (see its lifecycle.precondition). That guard is a
# resource precondition rather than a variable validation because the rule spans
# two variables and belongs to the resource it breaks; and rather than a `check`
# block because `check` only ever warns, which is itself the silent failure being
# guarded against.
locals {
  budget_alert_emails = length(var.budget_alert_emails) > 0 ? var.budget_alert_emails : (
    var.budget_alert_email != "" ? [var.budget_alert_email] : []
  )
}

# Tier 1 — early tripwire.
resource "aws_budgets_budget" "platform_admin" {
  name         = "${var.org}-platform-admin-budget"
  budget_type  = "COST"
  limit_amount = tostring(var.budget_alert_threshold_usd)
  limit_unit   = "USD"
  time_unit    = "MONTHLY"

  # Warn at 80% so there is time to react before the limit is hit.
  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 80
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = local.budget_alert_emails
  }

  # Hard alert at 100% of the limit.
  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = local.budget_alert_emails
  }

  # Forward-looking alert: notify when the forecasted spend is on track to
  # exceed the limit even if actual spend has not yet reached it.
  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "FORECASTED"
    subscriber_email_addresses = local.budget_alert_emails
  }

  tags = merge(local.common_tags, {
    Layer = "platform-org"
    Stack = "platform-org"
  })

  # A notification with no subscribers is a budget that alerts nobody, and AWS
  # accepts it — the tripwire would read as healthy while being off. See the
  # note above local.budget_alert_emails.
  lifecycle {
    precondition {
      condition     = length(local.budget_alert_emails) > 0
      error_message = "No budget alert recipient resolved. Run `make fetch ENV=prod` to refresh envs/prod/fetched.auto.tfvars.json from the platform registry."
    }
  }
}

# Tier 2 — higher hard ceiling for a genuine runaway across the org.
resource "aws_budgets_budget" "platform_ceiling" {
  name         = "${var.org}-platform-ceiling-budget"
  budget_type  = "COST"
  limit_amount = tostring(var.budget_ceiling_threshold_usd)
  limit_unit   = "USD"
  time_unit    = "MONTHLY"

  # Warn at 80% so there is time to react before the ceiling is hit.
  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 80
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = local.budget_alert_emails
  }

  # Hard alert at 100% of the ceiling.
  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = local.budget_alert_emails
  }

  # Forward-looking alert on the ceiling.
  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "FORECASTED"
    subscriber_email_addresses = local.budget_alert_emails
  }

  tags = merge(local.common_tags, {
    Layer = "platform-org"
    Stack = "platform-org"
  })

  # A notification with no subscribers is a budget that alerts nobody, and AWS
  # accepts it — the tripwire would read as healthy while being off. See the
  # note above local.budget_alert_emails.
  lifecycle {
    precondition {
      condition     = length(local.budget_alert_emails) > 0
      error_message = "No budget alert recipient resolved. Run `make fetch ENV=prod` to refresh envs/prod/fetched.auto.tfvars.json from the platform registry."
    }
  }
}
