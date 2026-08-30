package api

import (
	"context"
	"net/http"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
)

// The usage and quota surface: TEN-08 at the HTTP boundary, read-only. Both
// operations authorize through the one policy path with tenant.billing_read
// and scope to the session's active tenant; the request names nothing.

// TenantBilling is what the API needs from the ledger, declared here per
// ADR-0005 and wired in cmd.
type TenantBilling interface {
	Usage(ctx context.Context, tenantID string) (TenantUsage, error)
}

// TenantUsage mirrors the ledger's answer at the port.
type TenantUsage struct {
	Started       int
	Credited      int
	Billable      int
	Limit         *int
	Remaining     *int
	WarnThreshold float64
	// Warning is none, approaching or reached.
	Warning string
}

// billingHandlers handles /tenant/usage and /tenant/quota.
type billingHandlers struct {
	authentication *authentication
	ledger         TenantBilling
}

// usage resolves the caller's authority and reads the ledger.
//
// The capability comes from the contract's declaration for the operation being
// served rather than from a literal here, per ADR-0004: the document is the
// source, and a handler that named its own would be a second place to change
// when the answer changes.
func (b *billingHandlers) usage(ctx context.Context) (TenantUsage, *failure) {
	capability := requiredCapability(ctx)
	if capability == "" {
		// The contract says this operation needs no authority, which cannot be
		// true of a billing read. Refuse rather than ask for nothing.
		refused := b.authentication.rejectedSession(ctx)
		return TenantUsage{}, &refused
	}

	presented := sessionTokenFromContext(ctx)
	if presented == "" {
		refused := b.authentication.rejectedSession(ctx)
		return TenantUsage{}, &refused
	}
	principal, err := b.authentication.identity.Authorize(ctx, presented, capability)
	if err != nil {
		refused := b.authentication.failed(ctx, err)
		return TenantUsage{}, &refused
	}
	answer, err := b.ledger.Usage(ctx, principal.ActiveTenantID)
	if err != nil {
		refused := b.authentication.failed(ctx, err)
		return TenantUsage{}, &refused
	}
	return answer, nil
}

// GetTenantUsage answers the counts the invoice will use.
func (b *billingHandlers) GetTenantUsage(ctx context.Context, _ prepeetapi.GetTenantUsageRequestObject) (prepeetapi.GetTenantUsageResponseObject, error) {
	answer, refused := b.usage(ctx)
	if refused != nil {
		return *refused, nil
	}
	return prepeetapi.GetTenantUsage200JSONResponse{
		Body: prepeetapi.TenantUsage{
			Started:  answer.Started,
			Credited: answer.Credited,
			Billable: answer.Billable,
		},
		Headers: prepeetapi.GetTenantUsage200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// GetTenantQuota answers the limit and where the warning stands.
func (b *billingHandlers) GetTenantQuota(ctx context.Context, _ prepeetapi.GetTenantQuotaRequestObject) (prepeetapi.GetTenantQuotaResponseObject, error) {
	answer, refused := b.usage(ctx)
	if refused != nil {
		return *refused, nil
	}
	body := prepeetapi.TenantQuota{
		Billable:      answer.Billable,
		WarnThreshold: float32(answer.WarnThreshold),
		Warning:       prepeetapi.TenantQuotaWarning(answer.Warning),
	}
	body.SessionLimit = answer.Limit
	body.Remaining = answer.Remaining
	return prepeetapi.GetTenantQuota200JSONResponse{
		Body:    body,
		Headers: prepeetapi.GetTenantQuota200ResponseHeaders{CacheControl: NoStore},
	}, nil
}

// The failure type must speak these operations' responses.
var (
	_ prepeetapi.GetTenantUsageResponseObject = failure{}
	_ prepeetapi.GetTenantQuotaResponseObject = failure{}
)

func (f failure) VisitGetTenantUsageResponse(w http.ResponseWriter) error { return f.write(w) }
func (f failure) VisitGetTenantQuotaResponse(w http.ResponseWriter) error { return f.write(w) }
