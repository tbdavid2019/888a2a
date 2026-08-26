//go:build release

package cmd

import (
	"github.com/tbdavid2019/888a2a/backend/common"
	"github.com/tbdavid2019/888a2a/backend/manager/config"
)

func activeProfile(dataDir string) *config.Profile {
	p := getBaseProfile(dataDir)
	p.Mode = common.ReleaseModeProd
	return p
}
