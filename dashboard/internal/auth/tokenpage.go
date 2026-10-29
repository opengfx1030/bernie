package auth

import (
	"io"
	"net/http"
	"strings"
)

// tokenPageHTML is what a browser gets when the dashboard runs in token
// mode and the request carries no token. It says what to do in words: the
// address that `bernie dash` printed ends in "/?token=…". The token itself
// is never in the page.
//
// The page carries no style attribute and no script: the CSP of spec §11
// (`style-src 'self'; script-src 'self'`) drops both.
const tokenPageHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>bernie: the dashboard needs a token</title>
</head>
<body>
<h1>This dashboard needs the token from the terminal.</h1>
<p>bernie printed the address of this page when it started. The address ends
in <code>/?token=...</code>. Open that address and the dashboard comes up.</p>
<p>Do you not have the address any more? Start the dashboard again with
<code>bernie dash</code>. It prints the address on the first line.</p>
<p>An agent reads the same host through the API. It sends the token as
<code>Authorization: Bearer &lt;token&gt;</code>.</p>
</body>
</html>
`

// wantsHTML says whether the request comes from a browser that wants a
// page. An API client asks for JSON and gets the JSON error.
func wantsHTML(r *http.Request) bool {
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/mcp") {
		return false
	}
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// tokenPage writes the page of a browser that has no token.
func (g *Guard) tokenPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = io.WriteString(w, tokenPageHTML)
}
