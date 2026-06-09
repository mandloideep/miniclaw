package gmailoauth

import (
	"context"
	"net/http"

	"golang.org/x/oauth2"
)

// oauth2HTTPClient is a thin shim that returns an http.Client whose Transport
// uses src to auto-attach Authorization headers and refresh as needed.
func oauth2HTTPClient(ctx context.Context, src oauth2.TokenSource) *http.Client {
	return oauth2.NewClient(ctx, src)
}

// AuthedHTTPClient is the exported alias other packages (planner's calendar
// push/pull) use to build an authed client without having to know the
// oauth2 plumbing.
func AuthedHTTPClient(ctx context.Context, src oauth2.TokenSource) *http.Client {
	return oauth2HTTPClient(ctx, src)
}
