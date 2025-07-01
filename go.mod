// go/go.mod
module github.com/hypernetix/hyperspot

go 1.23.0

toolchain go1.23.5

replace (
        github.com/hypernetix/hyperspot/libs => ./libs
        github.com/hypernetix/hyperspot/modules => ./modules
)
