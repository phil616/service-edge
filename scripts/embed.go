// Package scripts embeds the install script templates so the control plane can
// render them at request time.
package scripts

import _ "embed"

//go:embed install-frps.sh.tmpl
var FRPSInstall string

//go:embed install-frpc.sh.tmpl
var FRPCInstall string
