variable "aws_region" {
  description = "AWS region for the management account."
  type        = string
  default     = "us-east-1"
}

variable "aws_profile" {
  description = "AWS CLI profile to use for the management account. Set to null when using injected environment credentials (AWS_ACCESS_KEY_ID etc.)."
  type        = string
  default     = null
}

variable "org" {
  description = "Short org identifier used in resource naming (e.g. ffreis)."
  type        = string
}

variable "accounts" {
  description = "Member accounts to create under the environments OU. Map key is the account name."
  type = map(object({
    email = string
  }))
}

variable "budget_alert_emails" {
  description = "Email addresses that receive budget alert notifications. Supplied via fetched.auto.tfvars.json (written by `make fetch ENV=prod`, sourced from the platform registry in DynamoDB) — never committed to source control."
  type        = list(string)
  default     = []

  validation {
    condition     = alltrue([for email in var.budget_alert_emails : can(regex("^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$", email))])
    error_message = "All values must be valid email addresses."
  }
}

# DEPRECATED — read var.budget_alert_emails instead. Kept, and optional, so a
# fetched.auto.tfvars.json written by either bootstrap version still applies:
# an older file carries only this key, a newer one carries only the list, and a
# current one carries both. Defaulting to "" is what makes the newer-file case
# work at all; without it Terraform would reject the plan for a missing
# required variable.
variable "budget_alert_email" {
  description = "DEPRECATED: single budget alert recipient, superseded by budget_alert_emails. Populated by older versions of `bootstrap fetch`, which emit only the first recipient. Remove once no fetched.auto.tfvars.json in circulation still carries it."
  type        = string
  default     = ""

  validation {
    condition     = var.budget_alert_email == "" || can(regex("^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$", var.budget_alert_email))
    error_message = "Must be empty or a valid email address."
  }
}

variable "budget_alert_threshold_usd" {
  description = "Tier-1 org-wide monthly spend limit in USD (early tripwire, ~2.5x normal fleet spend). Alerts fire at 80% and 100% actual, and 100% forecasted."
  type        = number
  default     = 30
  validation {
    condition     = var.budget_alert_threshold_usd > 0
    error_message = "Budget threshold must be greater than 0."
  }
}

variable "budget_ceiling_threshold_usd" {
  description = "Tier-2 org-wide monthly spend ceiling in USD (higher hard ceiling for a genuine runaway). Must be >= the tier-1 threshold."
  type        = number
  default     = 60
  validation {
    condition     = var.budget_ceiling_threshold_usd >= var.budget_alert_threshold_usd
    error_message = "Ceiling must be greater than or equal to budget_alert_threshold_usd."
  }
}

variable "project" {
  description = "Project tag value applied to all managed resources."
  type        = string
  default     = "platform"
}

variable "environment" {
  description = "Environment tag value applied to all managed resources."
  type        = string
  default     = "prod"
}

variable "tags" {
  description = "Additional tags applied to all resources."
  type        = map(string)
  default     = {}
}
