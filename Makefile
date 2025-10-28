binary_name = pathsurfer
binary_path = ./bin/${binary_name}
main_package_path = .

curr_time = $(shell date --iso-8601=seconds)
git_description = $(shell git describe --always --dirty)
linker_flags = '-s -X main.buildTime=${curr_time} -X main.version=${git_description}'

install_path = /usr/bin/pathsurfer
script_install_dir_for_bash = $(HOME)/.local/share/pathsurfer/functions
script_install_dir_for_fish = $(HOME)/.config/fish/conf.d
bashrc = $(HOME)/.bashrc

## build: build the application
.PHONY: build
build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags=${linker_flags} -o=${binary_path} ${main_package_path}

## install: install the application
.PHONY: install
install:
	@echo "Installing the binary to $(install_path)..."
	install -m 755 $(binary_path) $(install_path)
	@echo "Installed $(install_path)"

## install/fish: install the binary stored in <project-path>/bin/ for fish
.PHONY: install/fish
install/fish:
	@echo "Installing the psurf script to $(script_install_dir_for_fish)/psurf.fish"
	mkdir -p $(script_install_dir_for_fish)
	install -m 644 scripts/psurf.fish $(script_install_dir_for_fish)
	@echo "Installation complete. Run 'source $(script_install_dir_for_fish)/psurf.fish' or restart your shell to use psurf."

## install/bash: install the binary stored in <project-path>/bin/ for bash
.PHONY: install/bash
install/bash:
	@echo "Installing the psurf script to $(script_install_dir_for_bash)..."
	mkdir -p $(script_install_dir_for_bash)
	install -m 644 scripts/psurf.sh $(script_install_dir_for_bash)/psurf.sh

	@echo "Adding the source line to $(bashrc) if missing..."
	@grep -qxF "source $(script_install_dir_for_bash)/psurf.sh" $(bashrc) || \
	{ echo ""; echo "# Load the psurf shell function"; echo "source $(script_install_dir_for_bash)/psurf.sh"; } >> $(bashrc)

	@echo "Installation complete. Run 'source $(bashrc)' or restart your shell to use psurf."

## uninstall: remove the application
.PHONY: uninstall
uninstall:
	@echo "Removing $(install_path)..."
	rm -f $(install_path)
	@echo "Uninstallation completed"

## uninstall/bash: remove the psurf shell script for bash
.PHONY: uninstall/bash
uninstall/bash:
	@echo "Removing $(script_install_dir_for_bash)/psurf.sh..."
	rm -f $(script_install_dir_for_bash)/psurf.sh
	sed -i '\|# Load psurf shell function|d' $(bashrc) || true
	sed -i '\|source $(script_install_dir_for_bash)/psurf.sh|d' $(bashrc) || true
	@echo "Uninstallation completed"

## uninstall/fish: install the binary stored in <project-path>/bin/ for fish
.PHONY: uninstall/fish
uninstall/fish:
	@echo "Removing $(script_install_dir_for_fish)/psurf.fish..."
	rm -f $(script_install_dir_for_fish)/psurf.fish
	@echo "Uninstallation completed"
