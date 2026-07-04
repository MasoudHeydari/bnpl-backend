package permission

// All returns a defensive copy of every permission in the BNPL catalog.
func All() []Permission {
	out := make([]Permission, 0, len(customerApp)+len(merchantPanel)+len(branchPanel)+len(adminPanel))
	out = append(out, customerApp...)
	out = append(out, merchantPanel...)
	out = append(out, branchPanel...)
	out = append(out, adminPanel...)
	return out
}

func clonePermissions(src []Permission) []Permission {
	out := make([]Permission, len(src))
	copy(out, src)
	return out
}
