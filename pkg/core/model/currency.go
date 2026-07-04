package model

// Currency identifies a currency supported by the BNPL platform using its
// ISO 4217 three-letter code.
type Currency string

const (
	// IRRCurrency represents the Iranian Rial.
	IRRCurrency Currency = "IRR"

	// EURCurrency represents the Euro.
	EURCurrency Currency = "EUR"

	// USDCurrency represents the United States Dollar.
	USDCurrency Currency = "USD"
)

// String returns the currency code as a plain string.
func (c Currency) String() string {
	return string(c)
}

// IsValid reports whether the currency is known by the domain.
func (c Currency) IsValid() bool {
	switch c {
	case IRRCurrency, EURCurrency, USDCurrency:
		return true

	default:
		return false
	}
}
