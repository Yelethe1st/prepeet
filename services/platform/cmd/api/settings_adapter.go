package main

import (
	"context"
	"errors"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/internal/tenantadmin"
)

// settingsAdapter presents the workspace configuration as the API's port.
//
// It flattens the store's nested document to the two fields the read surface
// serves today. That is deliberate rather than lazy: the contract's shape is
// its own, and a change to how tenantadmin nests things should not reach the
// wire. The rest of the document arrives when there is a screen that renders
// it, rather than being carried now on the chance.
type settingsAdapter struct {
	settings *tenantadmin.SettingsStore
}

func (a settingsAdapter) Current(ctx context.Context, tenantID string) (api.TenantSettings, error) {
	current, err := a.settings.Current(ctx, tenantID)
	if err != nil {
		return api.TenantSettings{}, err
	}
	return api.TenantSettings{
		Version:     current.Version,
		LegalName:   current.Settings.Organisation.LegalName,
		DisplayName: current.Settings.Organisation.DisplayName,
		ChangedBy:   current.ChangedBy,
		ChangedAt:   current.ChangedAt,
	}, nil
}

// Save appends a version, translating the store's collision into the API's.
//
// The version travels on the document rather than as a separate argument
// because that is how it reaches the browser and comes back: the version read
// is the version changed, and separating them here would create a place for the
// two to disagree.
func (a settingsAdapter) Save(ctx context.Context, tenantID, actorID string, next api.TenantSettings) (api.TenantSettings, error) {
	current, err := a.settings.Current(ctx, tenantID)
	if err != nil {
		return api.TenantSettings{}, err
	}

	// Only the two fields the read surface serves are taken from the request.
	// The rest of the document is carried through from what is stored, so a
	// client that has not been taught the whole shape cannot silently blank the
	// parts it does not know about.
	document := current.Settings
	document.Organisation.LegalName = next.LegalName
	document.Organisation.DisplayName = next.DisplayName

	saved, err := a.settings.Save(ctx, tenantID, actorID, document, next.Version)
	if err != nil {
		if errors.Is(err, tenantadmin.ErrSettingsStale) {
			return api.TenantSettings{}, api.ErrSettingsConflict
		}
		return api.TenantSettings{}, err
	}
	return api.TenantSettings{
		Version:     saved.Version,
		LegalName:   saved.Settings.Organisation.LegalName,
		DisplayName: saved.Settings.Organisation.DisplayName,
		ChangedBy:   saved.ChangedBy,
		ChangedAt:   saved.ChangedAt,
	}, nil
}
