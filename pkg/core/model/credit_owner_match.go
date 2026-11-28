package model

// CreditBelongsToContract compares permanent legal-person ownership, not the
// issuing account of the shared credit. Contract/customer ownership and access
// permission are checked separately by the contract/repayment use cases and
// enforced by PostgreSQL composite FKs.
func CreditBelongsToContract(account CreditAccount, contract Contract) bool {
	if account.Currency != contract.Currency {
		return false
	}

	if !account.CreditOwnerID.IsZero() && !contract.CreditOwnerID.IsZero() {
		return account.CreditOwnerID == contract.CreditOwnerID
	}

	// Unpersisted legacy unit-test fixtures omit CreditOwnerID. PostgreSQL
	// never allows this case; preserve test semantics during incremental rollout.
	return account.CustomerID == contract.CustomerID
}

// CreditBelongsToCustomer checks eligibility for repayment of that customer's
// contracts while allowing the governed account to originate from another
// profile of the SAME legal person. The caller independently checks that each
// installment/contract belongs to customer.ID.
func CreditBelongsToCustomer(account CreditAccount, customer CustomerProfile, currency Currency) bool {
	if account.Currency != currency {
		return false
	}

	if !account.CreditOwnerID.IsZero() && !customer.CreditOwnerID.IsZero() {
		return account.CreditOwnerID == customer.CreditOwnerID
	}

	return account.CustomerID == customer.ID
}
