//go:build embed_machine

package server

import (
	"embed"
	"io/fs"
)

//go:embed embedded_machine
var embeddedMachine embed.FS

// machineManifest returns the embedded machine manifest.json bytes.
func machineManifest() ([]byte, error) {
	return embeddedMachine.ReadFile("embedded_machine/manifest.json")
}

// openMachineGz opens the gzipped machine binary for the given target
// (e.g. "linux-x64").
func openMachineGz(target string) (fs.File, error) {
	if f, err := embeddedMachine.Open("embedded_machine/888a2a-machine-" + target + ".gz"); err == nil {
		return f, nil
	}
	return embeddedMachine.Open("embedded_machine/laelia-machine-" + target + ".gz")
}
