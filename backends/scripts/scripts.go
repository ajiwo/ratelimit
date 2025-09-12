package scripts

import _ "embed"

// Lua scripts for Redis rate limiting operations
// These are embedded at compile time for better performance and maintainability

//go:embed check_and_increment.lua
var CheckAndIncrementScript string

//go:embed check_and_consume_token.lua
var CheckAndConsumeTokenScript string
