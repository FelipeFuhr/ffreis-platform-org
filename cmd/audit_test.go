package cmd

import (
	"testing"

	taggingtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
)

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
	m := taggingtypes.ResourceTagMapping{
		ResourceARN: sdkaws.String("arn:aws:dynamodb:us-east-1:123:table/ffreis-bootstrap-registry"),
		Tags: []taggingtypes.Tag{
			{Key: sdkaws.String("Layer"), Value: sdkaws.String("bootstrap")},
			{Key: sdkaws.String("ManagedBy"), Value: sdkaws.String("platform-bootstrap")},
		},
	}
	r := classifyResource(m)
	if r.status != "OK" {
		t.Errorf("want OK got %q", r.status)
	}
	if r.stack != "(bootstrap)" {
		t.Errorf("want (bootstrap) got %q", r.stack)
	}
}

func TestClassifyResource_OwnedAllTags(t *testing.T) {
	m := taggingtypes.ResourceTagMapping{
		ResourceARN: sdkaws.String("arn:aws:s3:::ffreis-tf-state-runtime"),
		Tags: []taggingtypes.Tag{
			{Key: sdkaws.String("Stack"), Value: sdkaws.String("platform-org")},
			{Key: sdkaws.String("Project"), Value: sdkaws.String("platform")},
			{Key: sdkaws.String("Environment"), Value: sdkaws.String("prod")},
			{Key: sdkaws.String("ManagedBy"), Value: sdkaws.String("terraform")},
		},
	}
	r := classifyResource(m)
	if r.status != "OK" {
		t.Errorf("want OK got %q", r.status)
	}
	if len(r.issues) != 0 {
		t.Errorf("want no issues got %v", r.issues)
	}
}

func TestClassifyResource_OwnedMissingTag(t *testing.T) {
	m := taggingtypes.ResourceTagMapping{
		ResourceARN: sdkaws.String("arn:aws:s3:::ffreis-tf-state-runtime"),
		Tags: []taggingtypes.Tag{
			{Key: sdkaws.String("Stack"), Value: sdkaws.String("flemming")},
			// Missing: Project, Environment, ManagedBy
		},
	}
	r := classifyResource(m)
	if r.status != "WARN" {
		t.Errorf("want WARN got %q", r.status)
	}
	if len(r.issues) == 0 {
		t.Error("want issues for missing tags")
	}
}

func TestClassifyResource_Unowned(t *testing.T) {
	m := taggingtypes.ResourceTagMapping{
		ResourceARN: sdkaws.String("arn:aws:s3:::some-manual-bucket"),
		Tags: []taggingtypes.Tag{
			{Key: sdkaws.String("Name"), Value: sdkaws.String("manual")},
		},
	}
	r := classifyResource(m)
	if r.status != "UNOWNED" {
		t.Errorf("want UNOWNED got %q", r.status)
	}
}

func TestClassifyResource_UnknownStack(t *testing.T) {
	m := taggingtypes.ResourceTagMapping{
		ResourceARN: sdkaws.String("arn:aws:lambda:us-east-1:123:function:rogue-lambda"),
		Tags: []taggingtypes.Tag{
			{Key: sdkaws.String("Stack"), Value: sdkaws.String("unknown-project")},
			{Key: sdkaws.String("ManagedBy"), Value: sdkaws.String("terraform")},
		},
	}
	r := classifyResource(m)
	if r.status != "UNOWNED" {
		t.Errorf("want UNOWNED for unknown Stack value, got %q", r.status)
	}
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
