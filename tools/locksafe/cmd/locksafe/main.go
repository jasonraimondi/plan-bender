package main

import (
	"github.com/jasonraimondi/plan-bender/tools/locksafe"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(locksafe.Analyzer)
}
