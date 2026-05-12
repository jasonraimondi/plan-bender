package containment

import "example.com/project/internal/planrepo"

type Holder struct {
	Sess *planrepo.PlanSession // want "struct field type recursively contains"
}

type NestedHolder struct {
	Sessions []*planrepo.PlanSession // want "struct field type recursively contains"
}

func TakesSession(sess *planrepo.PlanSession) { // want "function parameter type recursively contains"
	_ = sess
}

func ReturnsSession() *planrepo.PlanSession { // want "function return type recursively contains"
	return nil
}
