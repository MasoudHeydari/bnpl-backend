package permission

// Permission is a catalog value object for a BNPL authorization capability.
//
// Code is the canonical wire representation exchanged with Identity/FC through
// trusted gateway headers. Name and Description are local catalog metadata used
// for documentation and parity checks; they are never supplied by clients.
type Permission struct {
	Code        string
	Name        string
	Description string
}

func permission(code, name, description string) Permission {
	return Permission{
		Code:        code,
		Name:        name,
		Description: description,
	}
}

// String returns the canonical wire representation of the permission.
func (p Permission) String() string {
	return p.Code
}
