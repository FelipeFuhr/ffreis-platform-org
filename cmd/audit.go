package cmd

import (
	"context"
	"fmt"
	"strings"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/budgets"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	taggingtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/spf13/cobra"
)

// knownStackValues are all Stack tag values claimed by the platform.
// Resources with a Stack tag NOT in this list are unowned.
// Bootstrap resources don't carry Stack — they're identified by Layer=bootstrap.
var knownStackValues = []string{
	"platform-org",
	"platform-shared-infra",
	"flemming",
	"ffreis-website",
}

// requiredTags are the tags every Terraform-managed resource must carry.
var requiredTags = []string{"Project", "Environment", "ManagedBy", "Stack"}

// auditResource is one finding from the resource scan.
type auditResource struct {
	status       string // OK, WARN, UNOWNED
	resourceType string // human-readable type derived from ARN
	name         string // resource name derived from ARN
	stack        string // Stack tag value, or "(unowned)" / "(bootstrap)"
	issues       []string
}

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Scan all AWS resources for ownership, tag completeness, and budget coverage",
	Long: `audit uses the AWS Resource Groups Tagging API to scan all tagged resources
in the account and report on:

  1. Ownership — resources with no known Stack tag are flagged as unowned.
  2. Tag completeness — resources owned by this stack must carry all required tags.
  3. Budget coverage — verifies a budget exists and cost allocation tags are active.

Since platform-org is the management layer, this audit covers ALL stacks.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()

		fmt.Printf("\nAudit report — account: %s  region: %s\n\n", d.accountID, d.region)

		resources, err := scanResources(ctx)
		if err != nil {
			return fmt.Errorf("scanning resources: %w", err)
		}

		printResourceTable(resources)
		printBudgetSection(ctx)

		var owned, unowned, warn int
		for _, r := range resources {
			switch r.status {
			case "OK":
				owned++
			case "UNOWNED":
				unowned++
			default:
				warn++
			}
		}
		fmt.Printf("\nSummary: %d owned, %d unowned, %d with tag issues\n\n",
			owned, unowned, warn)

		return nil
	},
}

// scanResources fetches all tagged resources from the Tagging API and
// categorises each one as owned, unowned, or owned-with-issues.
func scanResources(ctx context.Context) ([]auditResource, error) {
	var results []auditResource

	var nextToken *string
	for {
		out, err := d.tagging.GetResources(ctx, &resourcegroupstaggingapi.GetResourcesInput{
			ResourcesPerPage: sdkaws.Int32(100),
			PaginationToken:  nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("GetResources: %w", err)
		}

		for _, mapping := range out.ResourceTagMappingList {
			results = append(results, classifyResource(mapping))
		}

		if sdkaws.ToString(out.PaginationToken) == "" {
			break
		}
		nextToken = out.PaginationToken
	}

	return results, nil
}

// classifyResource turns a single Tagging API mapping into an auditResource.
func classifyResource(m taggingtypes.ResourceTagMapping) auditResource {
	arn := sdkaws.ToString(m.ResourceARN)
	rtype, name := parseARN(arn)

	tags := make(map[string]string, len(m.Tags))
	for _, t := range m.Tags {
		tags[sdkaws.ToString(t.Key)] = sdkaws.ToString(t.Value)
	}

	// Bootstrap resources: identified by Layer=bootstrap.
	if tags["Layer"] == "bootstrap" {
		return auditResource{
			status:       "OK",
			resourceType: rtype,
			name:         name,
			stack:        "(bootstrap)",
		}
	}

	stackVal := tags["Stack"]

	// Check if Stack tag is a known platform value.
	if !isKnownStack(stackVal) {
		return auditResource{
			status:       "UNOWNED",
			resourceType: rtype,
			name:         name,
			stack:        stackVal,
			issues:       []string{"Stack tag absent or unrecognised"},
		}
	}

	// Owned resource — check required tags.
	var issues []string
	for _, req := range requiredTags {
		if tags[req] == "" {
			issues = append(issues, "missing tag: "+req)
		}
	}

	status := "OK"
	if len(issues) > 0 {
		status = "WARN"
	}

	return auditResource{
		status:       status,
		resourceType: rtype,
		name:         name,
		stack:        stackVal,
		issues:       issues,
	}
}

func isKnownStack(v string) bool {
	for _, s := range knownStackValues {
		if v == s {
			return true
		}
	}
	return false
}

// parseARN extracts a human-readable resource type and name from an ARN.
// ARN format: arn:partition:service:region:account:resource
// where resource can be "type/name", "type:name", or just "name".
func parseARN(arn string) (resourceType, name string) {
	// Split on ":" up to 6 parts: arn, partition, service, region, account, resource
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) < 6 {
		return "unknown", arn
	}
	service := parts[2]
	resource := parts[5]

	// resource can be "type/name", "type:name", or just "name"
	if idx := strings.Index(resource, "/"); idx >= 0 {
		return service + "/" + resource[:idx], resource[idx+1:]
	}
	if idx := strings.Index(resource, ":"); idx >= 0 {
		return service + "/" + resource[:idx], resource[idx+1:]
	}
	// S3 bucket ARNs: arn:aws:s3:::bucket-name (resource = bucket-name)
	return service, resource
}

// printResourceTable writes the audit findings table to stdout.
func printResourceTable(resources []auditResource) {
	const colFmt = "%-8s  %-30s  %-40s  %-22s  %s\n"
	fmt.Printf(colFmt, "STATUS", "TYPE", "NAME", "STACK", "ISSUES")
	fmt.Printf(colFmt,
		strings.Repeat("-", 8),
		strings.Repeat("-", 30),
		strings.Repeat("-", 40),
		strings.Repeat("-", 22),
		strings.Repeat("-", 20),
	)
	for _, r := range resources {
		issues := "-"
		if len(r.issues) > 0 {
			issues = strings.Join(r.issues, "; ")
		}
		stack := r.stack
		if stack == "" {
			stack = "(no tag)"
		}
		fmt.Printf(colFmt, r.status, truncate(r.resourceType, 30), truncate(r.name, 40), truncate(stack, 22), issues)
	}
}

// printBudgetSection checks and prints budget coverage and cost allocation tag status.
func printBudgetSection(ctx context.Context) {
	fmt.Println()
	fmt.Println("Budget & cost coverage:")

	// Check budgets.
	budgetsOut, err := d.budgets.DescribeBudgets(ctx, &budgets.DescribeBudgetsInput{
		AccountId: sdkaws.String(d.accountID),
	})
	if err != nil {
		fmt.Printf("  WARN  budgets       could not list: %v\n", err)
	} else if len(budgetsOut.Budgets) == 0 {
		fmt.Println("  WARN  budgets       none found — create a budget to track spend")
	} else {
		for _, b := range budgetsOut.Budgets {
			fmt.Printf("  OK    budget        %-40s  $%s/month\n",
				sdkaws.ToString(b.BudgetName),
				sdkaws.ToString(b.BudgetLimit.Amount),
			)
		}
	}

	// Check cost allocation tags (paginated).
	active := make(map[string]bool)
	var nextToken *string
	for {
		ceOut, err := d.ce.ListCostAllocationTags(ctx, &costexplorer.ListCostAllocationTagsInput{
			Status:    cetypes.CostAllocationTagStatusActive,
			NextToken: nextToken,
		})
		if err != nil {
			fmt.Printf("  WARN  cost-tags     could not list: %v\n", err)
			return
		}

		for _, t := range ceOut.CostAllocationTags {
			active[sdkaws.ToString(t.TagKey)] = true
		}

		if sdkaws.ToString(ceOut.NextToken) == "" {
			break
		}
		nextToken = ceOut.NextToken
	}

	requiredCostTags := []string{"Stack", "Project", "Layer", "Owner", "Environment"}
	for _, tag := range requiredCostTags {
		if active[tag] {
			fmt.Printf("  OK    cost-tag      %-40s  active\n", tag)
		} else {
			fmt.Printf("  WARN  cost-tag      %-40s  not activated — run platform-org apply\n", tag)
		}
	}
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func init() {
	rootCmd.AddCommand(auditCmd)
}
