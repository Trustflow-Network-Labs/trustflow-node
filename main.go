package main

import (
	"fmt"

	"github.com/adgsm/trustflow-node/cmd"
	"github.com/adgsm/trustflow-node/dependencies"
)

func main() {
	dependencies.CheckAndInstallDependencies()
	fmt.Println("\n🚀 Dependencies checked. Continuing to start the app...")

	cmd.Execute()
}
