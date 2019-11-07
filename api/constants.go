package api // Result is a string used to handle the results for probing
type Result string

const (
	// Success Result
	Success Result = "success"
	// Warning Result. Logically success, but with additional debugging information attached.
	Warning Result = "warning"
	// Failure Result
	Failure Result = "failure"
	// Unknown Result
	Unknown Result = "unknown"
)
