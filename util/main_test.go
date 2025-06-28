package util

import (
	"testing"

	"github.com/rah-0/testmark/testutil"

	"github.com/rah-0/margo/conf"
)

func TestMain(m *testing.M) {
	testutil.TestMainWrapper(testutil.TestConfig{
		M: m,
		LoadResources: func() error {
			conf.CheckFlags()
			return nil
		},
		UnloadResources: func() error {
			return nil
		},
	})
}
