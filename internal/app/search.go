package app

import "strings"

// likeLiteral escapes the LIKE wildcards in s, so a % or _ the Admin typed matches itself. Use it as a parameter of
// LIKE '%' || lower($n) || '%'.
var likeLiteral = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace
