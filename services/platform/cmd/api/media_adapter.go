package main

import (
	"context"

	"github.com/Yelethe1st/prepeet/services/platform/platform/objectstore"
)

// mediaProber reads artifacts back for reconciliation: the completion path
// trusts what the bucket holds, never what any recorder claimed.
type mediaProber struct {
	store *objectstore.S3Store
}

func (p mediaProber) Stat(ctx context.Context, storageKey string) (int64, string, error) {
	key, err := objectstore.ParseKey(storageKey)
	if err != nil {
		return 0, "", err
	}
	return p.store.StatDigest(ctx, key)
}
