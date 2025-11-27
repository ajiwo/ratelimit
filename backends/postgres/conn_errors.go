package postgres

// connErrorStrings contains string patterns used to identify connectivity-related errors
// in PostgreSQL connections. These patterns are used to distinguish between temporary
// connectivity issues (which should trigger health errors and potential failover)
// versus other types of errors (like SQL syntax errors or constraint violations).
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
	"i/o timeout",
	"broken pipe",
	"pool exhausted",
	"too many connections",
	"database is locked",
	"terminating connection",
}
