package beads

// IDPrefixer is implemented by stores that own a stable Beads ID prefix,
// without the trailing "-".
type IDPrefixer interface {
	IDPrefix() string
}

// ExplicitIDPrefixer is implemented by stores whose create path supports
// caller-supplied IDs using the store-owned prefix. This is intentionally
// narrower than IDPrefixer: some stores expose an ownership prefix for routing
// but should still let Create allocate IDs so post-create metadata writes stay
// visible to callers.
type ExplicitIDPrefixer interface {
	ExplicitIDPrefix() string
}

// StoreIDPrefix returns the normalized Beads ID prefix owned by store, when
// the store can expose one. This is for routing/ownership decisions; callers
// that need caller-supplied create IDs should use StoreExplicitIDPrefix.
func StoreIDPrefix(store Store) string {
	if store == nil {
		return ""
	}
	prefixer, ok := store.(IDPrefixer)
	if !ok {
		return ""
	}
	return normalizeIDPrefix(prefixer.IDPrefix())
}

// StoreExplicitIDPrefix returns the normalized prefix callers may use for
// explicit create IDs. It is separate from StoreIDPrefix so reporting an
// ownership prefix cannot accidentally change create/finalization semantics.
func StoreExplicitIDPrefix(store Store) string {
	if store == nil {
		return ""
	}
	prefixer, ok := store.(ExplicitIDPrefixer)
	if !ok {
		return ""
	}
	return normalizeIDPrefix(prefixer.ExplicitIDPrefix())
}
