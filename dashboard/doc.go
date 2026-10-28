// Package dashboard serves the bernie web UI and its JSON API (spec §11).
// It is a view over the domain packages: every endpoint calls the same code
// as the matching CLI command and returns the same JSON.
//
// The root package owns the server and its small public surface. The internal
// packages split the API by domain, the access guard, the SSE writer, the
// shared telemetry sampler and the JSON response shapes. The embedded web
// tree is grouped by role under web/.
package dashboard
