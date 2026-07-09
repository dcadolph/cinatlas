package cmd

// Exit codes returned by Execute.
const (
	// CodeOK signals success.
	CodeOK = 0
	// CodeError signals a general runtime failure.
	CodeError = 1
	// CodeUsage signals a bad invocation or unknown command.
	CodeUsage = 2
	// CodeConfig signals missing or invalid configuration.
	CodeConfig = 3
	// CodeNotFound signals that the requested subject was not found.
	CodeNotFound = 4
)
