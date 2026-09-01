package main

import (
	"context"

	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
	"github.com/Yelethe1st/prepeet/services/platform/internal/tenantadmin"
)

// rubricUsageAdapter answers the rubric library's question about campaigns.
//
// TEN-04 refuses to discard a draft a running campaign is using, and until now
// it was refusing on the strength of a question nobody could answer: the port
// was declared and had no implementation, because campaigns are another
// context's and only cmd may see both.
//
// The whole adapter is a rename. That is the shape ADR-0005 is aiming for: the
// library knows what the refusal means, recruiting knows which campaigns are
// open, and neither has to know the other exists.
type rubricUsageAdapter struct {
	campaigns *recruiting.Store
}

func newRubricUsage(campaigns *recruiting.Store) tenantadmin.RubricUsage {
	return rubricUsageAdapter{campaigns: campaigns}
}

func (a rubricUsageAdapter) InUse(ctx context.Context, tenantID, reference string) ([]string, error) {
	return a.campaigns.CampaignsUsing(ctx, tenantID, reference)
}
