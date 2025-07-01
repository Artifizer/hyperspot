package test_module

import "github.com/hypernetix/hyperspot/libs/core"

func InitModule() {
	core.RegisterModule(&core.Module{
		Name:          "test_module",
		InitAPIRoutes: registerTestModuleAPIRoutes,
		InitMain:      InitTestModuleJobs,
	})
}
