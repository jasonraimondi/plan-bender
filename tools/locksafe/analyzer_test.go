package locksafe_test

import (
	"testing"

	"github.com/jasonraimondi/plan-bender/tools/locksafe"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, locksafe.Analyzer,
		"example.com/project/cases/backendlive",
		"example.com/project/cases/nethttp",
		"example.com/project/cases/subprocess",
		"example.com/project/cases/containment",
		"example.com/project/spoofplanrepo",
		"example.com/project/internal/dispatch",
		"example.com/project/internal/planrepo",
	)
}
