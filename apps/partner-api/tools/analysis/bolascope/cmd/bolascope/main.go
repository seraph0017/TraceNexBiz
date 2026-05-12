package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/seraph0017/tracenexbiz/apps/partner-api/tools/analysis/bolascope"
)

func main() { singlechecker.Main(bolascope.Analyzer) }
