package spoofplanrepo

import "net/http"

type PlanSession struct{}

func Open() *PlanSession {
	return &PlanSession{}
}

func (s *PlanSession) Close() error {
	return nil
}

func NotTheRealPlanSession() error {
	sess := Open()
	defer sess.Close()
	_, err := http.Get("https://example.com")
	return err
}
