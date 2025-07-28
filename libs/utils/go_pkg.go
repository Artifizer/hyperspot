package utils

import (
	"runtime"
	"strings"
)

// CallerPackage returns the package path of the function that called it
// depth = 0: current function (e.g., GoCallerPackageName)
// depth = 1: direct caller
// depth = 2: caller of the caller, etc.
func GoCallerPackageName(depth int) string {
	pc, _, _, ok := runtime.Caller(depth)
	if !ok {
		return ""
	}

	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return ""
	}

	fullName := fn.Name() // e.g., "github.com/user/repo/pkg.Func"

	// return the first .-separated part of the last /-separated part of the full name
	lastSlash := strings.LastIndex(fullName, "/")
	if lastSlash == -1 {
		return ""
	}

	pkgPath := fullName[lastSlash+1:]
	firstDot := strings.Index(pkgPath, ".")
	if firstDot == -1 {
		return ""
	}

	return pkgPath[:firstDot]
}
