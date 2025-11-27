package redis

// connErrorStrings contains string patterns used to identify connectivity-related errors
// in Redis connections. These patterns are used to distinguish between temporary
// connectivity issues (which should trigger health errors and potential failover)
// versus other types of errors (like invalid commands or key not found errors).
//
// Redis operational errors like "NOSCRIPT", "WRONGTYPE", or key-not-found errors
// are intentionally excluded as they should not trigger health-based failover.
//
// The patterns are matched against the lowercase version of error messages using
// string containment.
//
// There are brittle detections but users can override these patterns by providing their
// own ConnErrorStrings in the Config.
var connErrorStrings = []string{
	"connection refused",
	"connection timeout",
	"connection reset",
	"network is unreachable",
	"no such host",
	"timeout",
	"i/o timeout",
	"broken pipe",
	"connection pool exhausted",
}
