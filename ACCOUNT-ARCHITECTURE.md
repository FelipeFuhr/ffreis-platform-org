# Account architecture

What "the AWS organization" actually is, how access works, and what's
shared versus per-project. Written 2026-09-23 from live AWS state, not from
Terraform source alone — re-verify the specific ARNs/table names below
against reality before relying on them in an incident, since they will
drift.

## It is one AWS account, not several

`aws organizations describe-organization` succeeds — there is a real AWS
Organization (`o-v4ger0h0d5`) — but `aws organizations list-accounts`
returns exactly **one account** (`331258178424`, "ffreis"). There are no
member accounts today. The Organization exists (for future SCP capability,
and because `platform-bootstrap`/`platform-org` are built to provision a
"foundational AWS multi-account platform"), but in its current state it is
functionally a single account with one Organization wrapper, not a
multi-account setup.

**If you've heard this described as "an admin account holding other
projects' credentials," that framing doesn't hold up mechanically — correct
it to the model below.** There's no account boundary between admin and
projects, and no credentials are held on projects' behalf anywhere.

## The four layers

Each layer is its own repo. A layer depends only on the one below it,
never sideways or up.

| Layer | Repo | Owns |
|---|---|---|
| 0 — Bootstrap | `ffreis-platform-bootstrap` | The `platform-admin` IAM role itself, the bootstrap registry table, the ROOT Terraform state bucket/lock table, the platform-wide events SNS topic, the top-level monthly AWS Budget |
| 1 — Org | `ffreis-platform-org` (this repo) | Org-level Terraform + the `accounts`/`cost`/`resources` introspection CLI (backed by `ffreis-platform-inventory`) |
| 2 — Shared infra | `ffreis-platform-shared-infra` | Cross-cutting resources every product reads by reference: the shared VPC, build-cache buckets, the fleet CDN, SES domain identity, WAF IP sets, per-product least-privilege Terraform plan/apply roles, the free-tier monitor Lambda |
| 3 — Products | every `*-infra` repo (`petlook-infra`, `ffreis-flemming-infra`, `casaboa-infra`, ...) | That product's own resources, reading Layer 1/2 outputs via `data` sources |

`platform-bootstrap init` (Layer 0) is the only thing that creates the
`platform-admin` role. Everything above it either assumes that role
directly (human/ops tooling, via `~/.aws/config`'s `role_arn` chain) or
gets a narrower role of its own, scoped by Terraform, that never has to
touch `platform-admin` at all.

## Who can do what — three separate mechanisms, no stored credentials

**1. Humans and ops tooling: assume `platform-admin`.** One IAM user
(`ffreis-admin`) has no permissions of its own beyond `sts:AssumeRole` into
`arn:aws:iam::331258178424:role/platform-admin`, which carries
`Effect:Allow, Action:*, Resource:*` (with an explicit `Deny` on
root-identity mutations — MFA device, password policy, account contact
info, region enable/disable, account closure). This is the fleet's one
break-glass/full-access identity. `AWS_PROFILE=ffreis-platform` resolves to
it. It is a role inside the single account, not a separate account, and it
is not something a project "receives" — it's reached by a human or a
CI/ops job explicitly assuming it.

**2. Runtime workloads: a scoped IAM execution role per function, provisioned
by Terraform, nothing stored.** Every Lambda gets its own role
(`<product>-<function>-<env>-exec`, ~200 of them today), trusted only by
`lambda.amazonaws.com`, with an inline policy scoped to exactly what that
function touches. Example: `petlook-ask-prod-exec` can read exactly one S3
bucket and invoke exactly one Bedrock model + one guardrail. No AWS access
key exists for this — it's pure IAM role assumption at invoke time.

**3. CI/CD: GitHub Actions OIDC, not stored keys.** One OIDC provider
(`token.actions.githubusercontent.com`) is registered account-wide. Each
product's Terraform plan/apply roles trust it scoped to that repo (and
often that environment) via the `sub` claim — e.g.
`petlook-terraform-plan-prod` trusts only
`repo:ffreis-org/petlook-infra:environment:prod`. A workflow calls
`AssumeRoleWithWebIdentity` directly; no `AWS_ACCESS_KEY_ID` secret is ever
set for these flows. Layer 2 provisions the standard shape of these roles
(`*-terraform-plan` read-only, `*-terraform-apply` write, ExternalId +
optional MFA condition, a companion explicit-deny policy against
destructive actions) that Layer 3 repos consume.

**None of these three mechanisms involve one account or role holding
another project's credentials.** The closest thing to "shared credentials"
is mechanism 1 (`platform-admin`), and that's a single identity anyone with
`ffreis-admin` access can assume for admin work — not per-project, not
something projects are handed.

## Where non-AWS secrets actually live

Two genuinely different, non-overlapping systems — don't confuse them:

- **Per-project application secrets** (API keys, DB credentials, SMTP
  creds — anything that isn't AWS access itself) live in one DynamoDB table
  per product per environment: `petlook-secrets-{dev,prod}`,
  `flemming-secrets-{dev,prod}`, `ffreis-secrets-{dev,prod}`,
  `casaboa-secrets-dev`, `gitgate-secrets-dev`, and others as products are
  added. Each is small (single-digit item counts), simple hash-key schema,
  read directly by that product's own Lambdas. Decentralized by design —
  there is no central secrets store for this class.
- **Repo/CI-identity bootstrap secrets** (fleet GitHub PATs and similar)
  live in a separate, much smaller vault:
  `ffreis-platform-configctl` (the service) backed by
  `ffreis-vault-{identity,repo,root}-{dev,prod}` tables. This exists to
  onboard new repos into the fleet's CI conventions, not to hold AWS
  credentials or general application secrets. See that repo and
  `quality-kit`'s `/vault` skill for the current state of this initiative —
  it's still maturing.

Neither of these is what "admin account holds other projects' credentials"
was describing, if that's the mental model you're correcting against —
AWS access itself is never stored as a credential anywhere in this fleet;
only non-AWS application secrets are stored, and per-project, not centrally.

## What Layer 2 actually shares, confirmed live

Real resources, checked against live AWS state, not just Terraform source,
as of 2026-09-23 (verify freshness before trusting a specific name):

- **One VPC** (`vpc-04471ef721cf9507c`, `10.0.0.0/16`) — backs the fleet's
  self-hosted CI runner cluster (`ffreis-cluster-*` tables, cluster-warden
  roles).
- **Route53**: 4 hosted zones — `ffreis.com`, `petlook.app`,
  `flemming.com.br`, `pocketworldarcade.app` — with `ffreis.com` acting as
  parent for most product subdomains (`dashboard.ffreis.com`,
  `forma.ffreis.com`, `casaboa.ffreis.com`, and a dozen more), each with its
  own ACM cert (22 issued certs in `us-east-1` total).
- **Terraform state**: a *shared bucket per environment tier* holding a
  *per-product key prefix*, not a bucket per product —
  `ffreis-tf-state-{dev,prod,root,runtime}`, each containing prefixes like
  `petlook/`, `flemming/`, `platform-shared-infra/`. `root` holds the
  `platform-org/` bootstrap-layer state; the naming convention elsewhere in
  this workspace referencing a `dynamoctl-tf-locks-*` table is stale — the
  real tables are `ffreis-tf-locks-{dev,prod,root,runtime}`.
- **Build caches**: `ffreis-rust-build-cache` (sccache) and
  `ffreis-go-build-cache`, each with its own scoped OIDC upload role, 3-day
  expiry — genuinely shared CI infrastructure, not per-product.
- **Other shared state**: `ffreis-feature-flags-{dev,prod}`,
  `ffreis-fleet-widgets`, `ffreis-monitor-alert-state`,
  `ffreis-bootstrap-registry`, `ffreis-aws-cost-cache`,
  `ffreis-lambda-artifacts` (S3), `ffreis-shared-access-logs` (S3,
  CloudFront logging target for the whole website fleet).

Full detail and the Terraform that provisions this lives in
`ffreis-platform-shared-infra`'s own README — this section is a pointer,
not a duplicate; that repo's list is the one to trust if this drifts.

## Cost and billing: per-tag, not per-account

Since it's one account, billing separation is by tag, not by account
boundary. `CostCenter`, `Project`, and `Environment` are active cost
allocation tags (confirmed via `aws ce list-cost-allocation-tags`).
`ManagedBy` and `Lifecycle` — two tags this workspace's own tagging
standard otherwise mandates — are **not** active cost-allocation tags, so
spend can't currently be sliced by those two. As of the September 2026
snapshot used for this doc, a nontrivial slice of that month's spend
(~$8) was on **untagged** resources — worth a periodic sweep, see
`platform-org`'s own `cost --json` command for prior audits of this gap.

## Quick reference

```sh
# Who am I, which identity, which account
aws sts get-caller-identity --profile ffreis-platform

# Org/account structure, cost, and tagged-resource inventory — read-only
./bin/platform-org accounts --json
./bin/platform-org cost --days 7 --profile bootstrap
./bin/platform-org resources --group-by CostCenter --json
```

## When this drifts

This document describes a snapshot. Before trusting a specific account ID,
role ARN, table name, or resource count from it in anything consequential,
re-verify against live AWS state (`aws sts get-caller-identity`,
`aws organizations list-accounts`, `aws iam get-role`, or the
`platform-org` CLI's read-only commands above).
