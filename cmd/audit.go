package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/budgets"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	taggingtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/spf13/cobra"
)

// requiredTags are the tags every Terraform-managed resource must carry.
// Ownership is determined by ManagedBy=terraform (or Layer=bootstrap) — no
// hardcoded list of stack names is needed, so new stacks are recognised
// automatically once their resources carry the correct tags.
var requiredTags = []string{"Project", "Environment", "ManagedBy", "Stack"}

var (
	auditStdout          io.Writer = os.Stdout
	scanResourcesFn                = scanResources
	printResourceTableFn           = printResourceTable
	printBudgetSectionFn           = printBudgetSection
	getResourcesPage               = func(ctx context.Context, input *resourcegroupstaggingapi.GetResourcesInput) (*resourcegroupstaggingapi.GetResourcesOutput, error) {
		return d.tagging.GetResources(ctx, input)
	}
	describeBudgetsFn = func(ctx context.Context, input *budgets.DescribeBudgetsInput) (*budgets.DescribeBudgetsOutput, error) {
		return d.budgets.DescribeBudgets(ctx, input)
	}
	listCostAllocationTagsFn = func(ctx context.Context, input *costexplorer.ListCostAllocationTagsInput) (*costexplorer.ListCostAllocationTagsOutput, error) {
		return d.ce.ListCostAllocationTags(ctx, input)
	}
)

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

  1. Ownership — resources without ManagedBy=terraform or Layer=bootstrap are unowned.
     No hardcoded stack list: any new stack is recognised automatically.
  2. Tag completeness — terraform-managed resources must carry all required tags.
  3. Budget coverage — verifies a budget exists and cost allocation tags are active.

Since platform-org is the management layer, this audit covers ALL stacks.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()

		fmt.Fprintf(auditStdout, "\nAudit report — account: %s  region: %s\n\n", d.accountID, d.region)

		resources, err := scanResourcesFn(ctx)
		if err != nil {
			return fmt.Errorf("scanning resources: %w", err)
		}

		printResourceTableFn(resources)
		printBudgetSectionFn(ctx)

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
		fmt.Fprintf(auditStdout, "\nSummary: %d owned, %d unowned, %d with tag issues\n\n", owned, unowned, warn)

		return nil
	},
}

// scanResources fetches all tagged resources from the Tagging API and
// categorises each one as owned, unowned, or owned-with-issues.
func scanResources(ctx context.Context) ([]auditResource, error) {
	var results []auditResource

	var nextToken *string
	for {
		out, err := getResourcesPage(ctx, &resourcegroupstaggingapi.GetResourcesInput{
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

	// Ownership gate: ManagedBy=terraform means some Terraform stack owns this.
	// Any resource lacking this tag is unowned — no list of stack names needed.
	if tags["ManagedBy"] != "terraform" {
		return auditResource{
			status:       "UNOWNED",
			resourceType: rtype,
			name:         name,
			stack:        tags["Stack"],
			issues:       []string{"ManagedBy tag absent or not 'terraform'"},
		}
	}

	// Terraform-managed resource — check required tags.
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
		stack:        tags["Stack"],
		issues:       issues,
	}
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
	fmt.Fprintf(auditStdout, colFmt, "STATUS", "TYPE", "NAME", "STACK", "ISSUES")
	fmt.Fprintf(auditStdout, colFmt,
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
		fmt.Fprintf(auditStdout, colFmt, r.status, truncate(r.resourceType, 30), truncate(r.name, 40), truncate(stack, 22), issues)
	}
}

// printBudgetSection checks and prints budget coverage and cost allocation tag status.
func printBudgetSection(ctx context.Context) {
	fmt.Fprintln(auditStdout)
	fmt.Fprintln(auditStdout, "Budget & cost coverage:")

	printBudgets(ctx)

	active, err := loadActiveCostTags(ctx)
	if err != nil {
		fmt.Fprintf(auditStdout, "  WARN  cost-tags     could not list: %v\n", err)
		return
	}

	printCostTagStatuses(active)
}

func printBudgets(ctx context.Context) {
	budgetsOut, err := describeBudgetsFn(ctx, &budgets.DescribeBudgetsInput{
		AccountId: sdkaws.String(d.accountID),
	})
	if err != nil {
		fmt.Fprintf(auditStdout, "  WARN  budgets       could not list: %v\n", err)
		return
	}
	if len(budgetsOut.Budgets) == 0 {
		fmt.Fprintln(auditStdout, "  WARN  budgets       none found — create a budget to track spend")
		return
	}
	for _, b := range budgetsOut.Budgets {
		fmt.Fprintf(auditStdout, "  OK    budget        %-40s  $%s/month\n",
			sdkaws.ToString(b.BudgetName),
			sdkaws.ToString(b.BudgetLimit.Amount),
		)
	}
}

func loadActiveCostTags(ctx context.Context) (map[string]bool, error) {
	active := make(map[string]bool)
	var nextToken *string
	for {
		ceOut, err := listCostAllocationTagsFn(ctx, &costexplorer.ListCostAllocationTagsInput{
			Status:    cetypes.CostAllocationTagStatusActive,
			NextToken: nextToken,
		})
		if err != nil {
			return nil, err
		}

		for _, t := range ceOut.CostAllocationTags {
			active[sdkaws.ToString(t.TagKey)] = true
		}

		if sdkaws.ToString(ceOut.NextToken) == "" {
			return active, nil
		}
		nextToken = ceOut.NextToken
	}
}

func printCostTagStatuses(active map[string]bool) {
	requiredCostTags := []string{"Stack", "Project", "Layer", "Owner", "Environment"}
	for _, tag := range requiredCostTags {
		if active[tag] {
			fmt.Fprintf(auditStdout, "  OK    cost-tag      %-40s  active\n", tag)
			continue
		}
		fmt.Fprintf(auditStdout, "  WARN  cost-tag      %-40s  not activated — run platform-org apply\n", tag)
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
