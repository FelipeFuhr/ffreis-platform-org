package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/budgets"
	budgettypes "github.com/aws/aws-sdk-go-v2/service/budgets/types"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	taggingtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
)

func captureAuditOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	old := auditStdout
	var buf bytes.Buffer
	auditStdout = &buf
	t.Cleanup(func() {
		auditStdout = old
	})
	return &buf
}

func TestScanResourcesHandlesPagination(t *testing.T) {
	old := getResourcesPage
	defer func() { getResourcesPage = old }()

	calls := 0
	getResourcesPage = func(_ context.Context, input *resourcegroupstaggingapi.GetResourcesInput) (*resourcegroupstaggingapi.GetResourcesOutput, error) {
		calls++
		switch calls {
		case 1:
			if sdkaws.ToString(input.PaginationToken) != "" {
				t.Fatalf("first page token: want empty got %q", sdkaws.ToString(input.PaginationToken))
			}
			return &resourcegroupstaggingapi.GetResourcesOutput{
				PaginationToken: sdkaws.String("next-page"),
				ResourceTagMappingList: []taggingtypes.ResourceTagMapping{
					testResourceTagMapping(
						"arn:aws:s3:::ffreis-tf-state-runtime",
						testTag("Stack", "platform-org"),
						testTag("Project", "platform"),
						testTag("Environment", testEnv),
						testTag("ManagedBy", "terraform"),
					),
				},
			}, nil
		case 2:
			if sdkaws.ToString(input.PaginationToken) != "next-page" {
				t.Fatalf("second page token: want next-page got %q", sdkaws.ToString(input.PaginationToken))
			}
			return &resourcegroupstaggingapi.GetResourcesOutput{
				ResourceTagMappingList: []taggingtypes.ResourceTagMapping{
					testResourceTagMapping(
						"arn:aws:s3:::manual-bucket",
						testTag("Name", "manual"),
					),
				},
			}, nil
		default:
			t.Fatalf("unexpected call %d", calls)
			return nil, nil
		}
	}

	got, err := scanResources(context.Background())
	if err != nil {
		t.Fatalf("scanResources: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("resources: want 2 got %d", len(got))
	}
	if got[0].status != "OK" || got[1].status != "UNOWNED" {
		t.Fatalf("unexpected statuses: %#v", got)
	}
}

func TestScanResourcesReturnsWrappedError(t *testing.T) {
	old := getResourcesPage
	defer func() { getResourcesPage = old }()
	getResourcesPage = func(context.Context, *resourcegroupstaggingapi.GetResourcesInput) (*resourcegroupstaggingapi.GetResourcesOutput, error) {
		return nil, errors.New("tagging failed")
	}

	_, err := scanResources(context.Background())
	if err == nil || !strings.Contains(err.Error(), "GetResources: tagging failed") {
		t.Fatalf(errUnexpectedError, err)
	}
}

func TestPrintResourceTableRendersDefaultsAndTruncates(t *testing.T) {
	buf := captureAuditOutput(t)
	printResourceTable([]auditResource{{
		status:       "WARN",
		resourceType: "very-long-resource-type-that-should-truncate",
		name:         "very-long-resource-name-that-should-also-truncate",
		issues:       []string{"missing tag: Project", "missing tag: Stack"},
	}})

	out := buf.String()
	if !strings.Contains(out, "STATUS") || !strings.Contains(out, "ISSUES") {
		t.Fatalf("missing table header: %q", out)
	}
	if !strings.Contains(out, "(no tag)") {
		t.Fatalf("missing default stack label: %q", out)
	}
	if !strings.Contains(out, "…") {
		t.Fatalf("expected truncated output with ellipsis: %q", out)
	}
	if !strings.Contains(out, "missing tag: Project; missing tag: Stack") {
		t.Fatalf("missing joined issues: %q", out)
	}
}

func TestPrintBudgetsReportsError(t *testing.T) {
	buf := captureAuditOutput(t)
	old := describeBudgetsFn
	defer func() { describeBudgetsFn = old }()
	describeBudgetsFn = func(context.Context, *budgets.DescribeBudgetsInput) (*budgets.DescribeBudgetsOutput, error) {
		return nil, errors.New("boom")
	}

	printBudgets(context.Background())
	if !strings.Contains(buf.String(), "could not list: boom") {
		t.Fatalf(errUnexpectedOutput, buf.String())
	}
}

func TestPrintBudgetsReportsEmpty(t *testing.T) {
	buf := captureAuditOutput(t)
	old := describeBudgetsFn
	defer func() { describeBudgetsFn = old }()
	describeBudgetsFn = func(context.Context, *budgets.DescribeBudgetsInput) (*budgets.DescribeBudgetsOutput, error) {
		return &budgets.DescribeBudgetsOutput{}, nil
	}

	printBudgets(context.Background())
	if !strings.Contains(buf.String(), "none found") {
		t.Fatalf(errUnexpectedOutput, buf.String())
	}
}

func TestPrintBudgetsReportsSuccess(t *testing.T) {
	buf := captureAuditOutput(t)
	old := describeBudgetsFn
	defer func() { describeBudgetsFn = old }()
	d.accountID = testAccountID
	describeBudgetsFn = func(_ context.Context, input *budgets.DescribeBudgetsInput) (*budgets.DescribeBudgetsOutput, error) {
		if sdkaws.ToString(input.AccountId) != d.accountID {
			t.Fatalf("account id: want %s got %s", d.accountID, sdkaws.ToString(input.AccountId))
		}
		return &budgets.DescribeBudgetsOutput{
			Budgets: []budgettypes.Budget{{
				BudgetName:  sdkaws.String("platform-budget"),
				BudgetLimit: &budgettypes.Spend{Amount: sdkaws.String("100.00")},
			}},
		}, nil
	}

	printBudgets(context.Background())
	if !strings.Contains(buf.String(), "platform-budget") || !strings.Contains(buf.String(), "$100.00/month") {
		t.Fatalf(errUnexpectedOutput, buf.String())
	}
}

func TestLoadActiveCostTagsHandlesPagination(t *testing.T) {
	old := listCostAllocationTagsFn
	defer func() { listCostAllocationTagsFn = old }()
	calls := 0
	listCostAllocationTagsFn = func(_ context.Context, input *costexplorer.ListCostAllocationTagsInput) (*costexplorer.ListCostAllocationTagsOutput, error) {
		calls++
		switch calls {
		case 1:
			if input.Status != cetypes.CostAllocationTagStatusActive {
				t.Fatalf("unexpected status: %s", input.Status)
			}
			return &costexplorer.ListCostAllocationTagsOutput{
				CostAllocationTags: []cetypes.CostAllocationTag{{TagKey: sdkaws.String("Stack")}},
				NextToken:          sdkaws.String("next-token"),
			}, nil
		case 2:
			if sdkaws.ToString(input.NextToken) != "next-token" {
				t.Fatalf("next token: want next-token got %q", sdkaws.ToString(input.NextToken))
			}
			return &costexplorer.ListCostAllocationTagsOutput{
				CostAllocationTags: []cetypes.CostAllocationTag{{TagKey: sdkaws.String("Project")}},
			}, nil
		default:
			t.Fatalf("unexpected call %d", calls)
			return nil, nil
		}
	}

	active, err := loadActiveCostTags(context.Background())
	if err != nil {
		t.Fatalf("loadActiveCostTags: %v", err)
	}
	if !active["Stack"] || !active["Project"] {
		t.Fatalf("unexpected active tags: %#v", active)
	}
}

func TestLoadActiveCostTagsReturnsError(t *testing.T) {
	old := listCostAllocationTagsFn
	defer func() { listCostAllocationTagsFn = old }()
	listCostAllocationTagsFn = func(context.Context, *costexplorer.ListCostAllocationTagsInput) (*costexplorer.ListCostAllocationTagsOutput, error) {
		return nil, errors.New("cost explorer failed")
	}

	_, err := loadActiveCostTags(context.Background())
	if err == nil || !strings.Contains(err.Error(), "cost explorer failed") {
		t.Fatalf(errUnexpectedError, err)
	}
}

func TestPrintCostTagStatuses(t *testing.T) {
	buf := captureAuditOutput(t)
	printCostTagStatuses(map[string]bool{
		"Stack":       true,
		"Environment": true,
	})

	out := buf.String()
	if !strings.Contains(out, "Stack") || !strings.Contains(out, "active") {
		t.Fatalf("missing active tag output: %q", out)
	}
	if !strings.Contains(out, "Project") || !strings.Contains(out, "not activated") {
		t.Fatalf("missing inactive tag output: %q", out)
	}
}

func TestPrintBudgetSectionHandlesCostTagFailure(t *testing.T) {
	buf := captureAuditOutput(t)
	oldBudgets := describeBudgetsFn
	oldTags := listCostAllocationTagsFn
	defer func() {
		describeBudgetsFn = oldBudgets
		listCostAllocationTagsFn = oldTags
	}()

	describeBudgetsFn = func(context.Context, *budgets.DescribeBudgetsInput) (*budgets.DescribeBudgetsOutput, error) {
		return &budgets.DescribeBudgetsOutput{}, nil
	}
	listCostAllocationTagsFn = func(context.Context, *costexplorer.ListCostAllocationTagsInput) (*costexplorer.ListCostAllocationTagsOutput, error) {
		return nil, errors.New("tag listing failed")
	}

	printBudgetSection(context.Background())
	out := buf.String()
	if !strings.Contains(out, "Budget & cost coverage:") || !strings.Contains(out, "tag listing failed") {
		t.Fatalf(errUnexpectedOutput, out)
	}
}

func TestAuditCommandRunEPrintsSummary(t *testing.T) {
	buf := captureAuditOutput(t)
	oldScan := scanResourcesFn
	oldTable := printResourceTableFn
	oldBudget := printBudgetSectionFn
	defer func() {
		scanResourcesFn = oldScan
		printResourceTableFn = oldTable
		printBudgetSectionFn = oldBudget
	}()

	d.accountID = testAccountID
	d.region = testRegion
	tableCalled := false
	budgetCalled := false
	scanResourcesFn = func(context.Context) ([]auditResource, error) {
		return []auditResource{{status: "OK"}, {status: "UNOWNED"}, {status: "WARN"}}, nil
	}
	printResourceTableFn = func(resources []auditResource) {
		tableCalled = len(resources) == 3
	}
	printBudgetSectionFn = func(context.Context) {
		budgetCalled = true
	}

	if err := auditCmd.RunE(auditCmd, nil); err != nil {
		t.Fatalf("auditCmd.RunE: %v", err)
	}
	if !tableCalled || !budgetCalled {
		t.Fatalf("expected table and budget printers to run: table=%v budget=%v", tableCalled, budgetCalled)
	}
	if !strings.Contains(buf.String(), "Summary: 1 owned, 1 unowned, 1 with tag issues") {
		t.Fatalf(errUnexpectedOutput, buf.String())
	}
}

func TestAuditCommandRunEReturnsWrappedError(t *testing.T) {
	oldScan := scanResourcesFn
	defer func() { scanResourcesFn = oldScan }()
	scanResourcesFn = func(context.Context) ([]auditResource, error) {
		return nil, errors.New("scan failed")
	}

	err := auditCmd.RunE(auditCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "scanning resources: scan failed") {
		t.Fatalf(errUnexpectedError, err)
	}
}
