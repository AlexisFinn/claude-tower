//go:build production

package main

// productionBuild reports a build made by wails build, which adds the
// production tag, as opposed to wails dev and tests.
const productionBuild = true
