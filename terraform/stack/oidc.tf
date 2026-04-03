# ---------------------------------------------------------------------------
# GitHub Actions OIDC provider
#
# Created once in the management account. All projects reference it via a
# data source. The thumbprint is GitHub's well-known certificate fingerprint.
# ---------------------------------------------------------------------------
resource "aws_iam_openid_connect_provider" "github" {
  url             = "https://token.actions.githubusercontent.com"
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = ["6938fd4d98bab03faadb97b34396831e3780aea1"]

  tags = merge(var.tags, {
    Name  = "github-actions-oidc"
    Layer = "platform-org"
  })
}
