package bootstrap

import (
	"os/signal"
)

// signalNotifyContext is a test seam around signal.NotifyContext.
var signalNotifyContext = signal.NotifyContext
