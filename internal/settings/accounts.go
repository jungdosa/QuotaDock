package settings

import "github.com/jungdosa/QuotaDock/internal/model"

const MaxCodexAccounts = 5

func lastFamilyAccount(order []string, family model.ProviderID) int {
	at := -1
	for i, id := range order {
		if model.AccountFamily(model.ProviderID(id)) == family {
			at = i
		}
	}
	return at
}
