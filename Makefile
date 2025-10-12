binary_name = pathsurfer
binary_path = .bin/${bin_name}
main_package_path = .

## build: build the application
.PHONY: build
build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o=${binary_path} ${main_package_path}
