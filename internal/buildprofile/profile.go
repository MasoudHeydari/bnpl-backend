package buildprofile

// Profile identifies the compile-time capability profile of the binary.
type Profile string

const (
	Development Profile = "development"
	Production  Profile = "production"
)

// Current returns the profile compiled into this binary.
func Current() Profile { return current }

// IsDevelopment reports whether development-only capabilities are compiled in.
func IsDevelopment() bool { return current == Development }

// IsProduction reports whether this is a production-capability binary.
func IsProduction() bool { return current == Production }
