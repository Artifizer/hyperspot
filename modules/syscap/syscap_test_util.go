package syscap

// This file is not part of the main package and is only used for testing.
// It is named with a `_test.go` suffix to ensure it's only included in test builds.

// resetTestRegistry clears the global capability registry for testing purposes.
func resetTestRegistry() {
	sysCapMutex.Lock()
	defer sysCapMutex.Unlock()
	sysCapRegistry = make(map[SysCapKey]*SysCap)
}
