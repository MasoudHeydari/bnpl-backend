package header

const (
	// AcceptLanguage is the standard HTTP request header used to negotiate the
	// response language.
	AcceptLanguage = "Accept-Language"

	// Language is a legacy/simple language selector accepted as a fallback when
	// Accept-Language does not contain a supported locale.
	Language = "lang"

	// ContentLanguage identifies the locale actually used for the response.
	ContentLanguage = "Content-Language"

	// Vary declares request headers that can change the response representation.
	Vary = "Vary"
)
