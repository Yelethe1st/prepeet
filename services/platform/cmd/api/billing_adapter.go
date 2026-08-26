package main

import (
	"context"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/billing"
)

// billingAdapter presents the ledger as the port the API declared.
type billingAdapter struct {
	ledger *billing.Ledger
}

func (a billingAdapter) Usage(ctx context.Context, tenantID string) (api.TenantUsage, error) {
	usage, err := a.ledger.Usage(ctx, tenantID)
	if err != nil {
		return api.TenantUsage{}, err
	}
	return api.TenantUsage{
		Started:       usage.Started,
		Credited:      usage.Credited,
		Billable:      usage.Billable,
		Limit:         usage.Limit,
		Remaining:     usage.Remaining,
		WarnThreshold: usage.WarnThreshold,
		Warning:       string(usage.Warning),
	}, nil
}
