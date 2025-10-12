binary_name = pathsurfer
binary_path = ./bin/${binary_name}
main_package_path = .

curr_time = $(shell date --iso-8601=seconds)
git_description = $(shell git describe --always --dirty)
linker_flags = '-s -X main.buildTime=${curr_time} -X main.version=${git_description}'

## build: build the application
.PHONY: build
build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags=${linker_flags} -o=${binary_path} ${main_package_path}
