package planrepo

import "net/http"

type PlanSession struct{}

func Open() *PlanSession {
	return &PlanSession{}
}

func (s *PlanSession) Close() error {
	return nil
}

type Holder struct {
	Sess *PlanSession
}

func RemoteWhileLiveInPlanrepo(sess *PlanSession) error {
	_, err := http.Get("https://example.com") // want "PlanSession sess is live across net/http call http.Get"
	return err
}
