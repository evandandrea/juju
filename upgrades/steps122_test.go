// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type steps122Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps122Suite{})

func (s *steps122Suite) TestStateStepsFor122(c *gc.C) {
	expected := []string{
		// Environment UUID related migrations should come first as
		// other upgrade steps may rely on them.
		"prepend the environment UUID to the ID of all settings docs",
		"prepend the environment UUID to the ID of all settingsRefs docs",
		"prepend the environment UUID to the ID of all networks docs",
		"prepend the environment UUID to the ID of all requestedNetworks docs",
		"prepend the environment UUID to the ID of all networkInterfaces docs",
		"prepend the environment UUID to the ID of all statuses docs",
		"prepend the environment UUID to the ID of all annotations docs",
		"prepend the environment UUID to the ID of all constraints docs",
	}
	assertStateSteps(c, version.MustParse("1.22.0"), expected)
}

func (s *steps122Suite) TestStepsFor122(c *gc.C) {
	assertSteps(c, version.MustParse("1.22.0"), []string{})
}
