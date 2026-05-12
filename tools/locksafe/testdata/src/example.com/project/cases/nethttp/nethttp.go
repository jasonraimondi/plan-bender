package nethttp

import (
	"net/http"

	"example.com/project/internal/planrepo"
)

func PackageHTTPWhileSessionLive() error {
	sess := planrepo.Open()
	defer sess.Close()
	_, err := http.Get("https://example.com") // want "PlanSession sess is live across net/http call http.Get"
	return err
}

func ClientHTTPWhileSessionLive(client *http.Client, req *http.Request) error {
	sess := planrepo.Open()
	defer sess.Close()
	_, err := client.Do(req) // want "PlanSession sess is live across net/http call client.Do"
	return err
}

func ClientHTTPAfterClose(client *http.Client, req *http.Request) error {
	sess := planrepo.Open()
	if err := sess.Close(); err != nil {
		return err
	}
	_, err := client.Do(req)
	return err
}
