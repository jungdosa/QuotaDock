package model

func CodexAccountIDs() []ProviderID {
	return []ProviderID{ProviderCodex, ProviderCodex2, ProviderCodex3, ProviderCodex4, ProviderCodex5}
}
func CodexAccountIndex(id ProviderID) int {
	for i, candidate := range CodexAccountIDs() {
		if id == candidate {
			return i + 1
		}
	}
	return 0
}
func IsCodexAccount(id ProviderID) bool { return CodexAccountIndex(id) != 0 }

// AccountFamily describes presentation/grouping only. Authentication remains
// provider-specific and persisted IDs keep their existing values.
func AccountFamily(id ProviderID) ProviderID {
	if IsClaudeAccount(id) {
		return ProviderClaude
	}
	if IsCodexAccount(id) {
		return ProviderCodex
	}
	return ""
}
func AccountIndex(id ProviderID) int {
	if index := ClaudeAccountIndex(id); index != 0 {
		return index
	}
	return CodexAccountIndex(id)
}
