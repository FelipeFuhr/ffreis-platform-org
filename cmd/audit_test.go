package cmd

import (
	"testing"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	taggingtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
)

func testTag(key, value string) taggingtypes.Tag {
	return taggingtypes.Tag{
		Key:   sdkaws.String(key),
		Value: sdkaws.String(value),
	}
}

func testResourceTagMapping(arn string, tags ...taggingtypes.Tag) taggingtypes.ResourceTagMapping {
	return taggingtypes.ResourceTagMapping{
		ResourceARN: sdkaws.String(arn),
		Tags:        tags,
	}
}

func assertAuditStatus(t *testing.T, r auditResource, want string) {
	t.Helper()

	if r.status != want {
		t.Errorf("want %s got %q", want, r.status)
	}
}

func TestParseARN(t *testing.T) {
	cases := []struct {
		arn      string
		wantType string
		wantName string
	}{
		{
			arn:      "arn:aws:s3:::ffreis-tf-state-root",
			wantType: "s3",
			wantName: "ffreis-tf-state-root",
		},
		{
			arn:      "arn:aws:dynamodb:us-east-1:123456789012:table/ffreis-tf-locks-root",
			wantType: "dynamodb/table",
			wantName: "ffreis-tf-locks-root",
		},
		{
			arn:      "arn:aws:iam::123456789012:role/platform-admin",
			wantType: "iam/role",
			wantName: "platform-admin",
		},
		{
			arn:      "arn:aws:sns:us-east-1:123456789012:ffreis-platform-events",
			wantType: "sns",
			wantName: "ffreis-platform-events",
		},
		{
			arn:      "arn:aws:lambda:us-east-1:123456789012:function:my-function",
			wantType: "lambda/function",
			wantName: "my-function",
		},
	}

	for _, tc := range cases {
		t.Run(tc.arn, func(t *testing.T) {
			gotType, gotName := parseARN(tc.arn)
			if gotType != tc.wantType {
				t.Errorf("type: want %q got %q", tc.wantType, gotType)
			}
			if gotName != tc.wantName {
				t.Errorf("name: want %q got %q", tc.wantName, gotName)
			}
		})
	}
}

func TestClassifyResource_Bootstrap(t *testing.T) {
	m := testResourceTagMapping(
		"arn:aws:dynamodb:us-east-1:123:table/ffreis-bootstrap-registry",
		testTag("Layer", "bootstrap"),
		testTag("ManagedBy", "platform-bootstrap"),
	)
	r := classifyResource(m)
	assertAuditStatus(t, r, "OK")
	if r.stack != "(bootstrap)" {
		t.Errorf("want (bootstrap) got %q", r.stack)
	}
}

func TestClassifyResource_OwnedAllTags(t *testing.T) {
	m := testResourceTagMapping(
		"arn:aws:s3:::ffreis-tf-state-runtime",
		testTag("Stack", "platform-org"),
		testTag("Project", "platform"),
		testTag("Environment", "prod"),
		testTag("ManagedBy", "terraform"),
	)
	r := classifyResource(m)
	assertAuditStatus(t, r, "OK")
	if len(r.issues) != 0 {
		t.Errorf("want no issues got %v", r.issues)
	}
}

func TestClassifyResource_OwnedMissingTag(t *testing.T) {
	m := testResourceTagMapping(
		"arn:aws:s3:::ffreis-tf-state-runtime",
		testTag("Stack", "flemming"),
	)
	r := classifyResource(m)
	assertAuditStatus(t, r, "WARN")
	if len(r.issues) == 0 {
		t.Error("want issues for missing tags")
	}
}

func TestClassifyResource_Unowned(t *testing.T) {
	m := testResourceTagMapping(
		"arn:aws:s3:::some-manual-bucket",
		testTag("Name", "manual"),
	)
	r := classifyResource(m)
	assertAuditStatus(t, r, "UNOWNED")
}

func TestClassifyResource_UnknownStack(t *testing.T) {
	m := testResourceTagMapping(
		"arn:aws:lambda:us-east-1:123:function:rogue-lambda",
		testTag("Stack", "unknown-project"),
		testTag("ManagedBy", "terraform"),
	)
	r := classifyResource(m)
	assertAuditStatus(t, r, "UNOWNED")
}

func TestIsKnownStack(t *testing.T) {
	for _, known := range knownStackValues {
		if !isKnownStack(known) {
			t.Errorf("expected %q to be known", known)
		}
	}
	if isKnownStack("rogue-stack") {
		t.Error("expected rogue-stack to be unknown")
	}
	if isKnownStack("") {
		t.Error("expected empty string to be unknown")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("short string truncated unexpectedly: %q", got)
	}
	long := "hello world foo bar baz"
	got := truncate(long, 10)
	if len([]rune(got)) > 10 {
		t.Errorf("truncate did not shorten enough: %q (%d runes)", got, len([]rune(got)))
	}
}
