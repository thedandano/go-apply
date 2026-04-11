package port

import "context"

type JDFetcher interface {
	Fetch(ctx context.Context, url string) (string, error)
}
