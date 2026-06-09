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
