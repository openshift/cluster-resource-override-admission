package main

import (
	"github.com/openshift/generic-admission-server/pkg/cmd"
)

func main() {
	cmd.RunAdmissionServer(&podSvtRelabel{})
}
