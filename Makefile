binary_name       = pathsurfer
binary_path       = ./build/${binary_name}
main_package_path = ./cmd/pathsurfer

curr_time 		= $(shell date -Iseconds)
git_description = $(shell git describe --always --dirty)
linker_flags    = '-s -X github.com/bnuredini/pathsurfer/internal/conf.buildTime=${curr_time} -X github.com/bnuredini/pathsurfer/internal/conf.version=${git_description}'

GOOS := $(shell go env GOOS)

ifeq ($(GOOS),darwin)
	install_path := $(HOME)/.local/bin/pathsurfer
else ifeq ($(GOOS),linux)
	install_path := $(HOME)/.local/bin/pathsurfer
else ifeq ($(GOOS),windows)
	install_path := $(USERPROFILE)/bin/pathsurfer
	binary_extension = .exe
else
	$(error Unsupported OS: $(GOOS))
endif

script_install_dir_for_fish = $(HOME)/.config/fish/conf.d
script_install_dir          = $(HOME)/.local/share/pathsurfer/functions

bashrc = $(HOME)/.bashrc
zshrc  = $(HOME)/.zshrc

## build: build pathsurfer 
.PHONY: build
build:
	CGO_ENABLED=0 go build -ldflags=${linker_flags} -o=${binary_path}${binary_extension} ${main_package_path}

## install: install pathsurfer
.PHONY: install
install: build
	@echo "Installing the binary to $(install_path)..."
	mkdir -p "$(HOME)/.local/bin"
	install -m 755 $(binary_path) $(install_path)
	@echo "Installed $(install_path)"

## integrate/fish: integrate the binary stored in <project-path>/build/ for fish
.PHONY: integrate/fish
integrate/fish:
	@echo "Installing psurf scripts to $(script_install_dir_for_fish)/psurf.fish..."
	mkdir -p $(script_install_dir_for_fish)
	install -m 644 scripts/psurf.fish $(script_install_dir_for_fish)
	install -m 644 scripts/psurf_keybindings.fish $(script_install_dir_for_fish)
	@echo ""
	@echo "Installation complete. Run the following commands or restart your shell to use psurf:"
	@echo ""
	@echo '```'
	@echo "source $(script_install_dir_for_fish)/psurf.fish"
	@echo "source $(script_install_dir_for_fish)/psurf_keybindings.fish"
	@echo '```'

## integrate/bash: integrate the binary stored in <project-path>/build/ for bash
.PHONY: integrate/bash
integrate/bash:
	@echo "Installing the psurf script to $(script_install_dir)..."
	mkdir -p $(script_install_dir)
	install -m 644 scripts/psurf.sh $(script_install_dir)/psurf.sh

	@echo "Adding the source line to $(bashrc) if missing..."
	@grep -qxF "source $(script_install_dir)/psurf.sh" $(bashrc) || \
	{ echo ""; echo "# Load the psurf shell function"; echo "source $(script_install_dir)/psurf.sh"; } >> $(bashrc)

	@echo ""
	@echo "Installation complete. Run 'source $(bashrc)' or restart your shell to use psurf."

## integrate/zsh: integrate the binary stored in <project-path>/build/ for zsh
.PHONY: integrate/zsh
integrate/zsh:
	@echo "Installing the psurf script to $(script_install_dir)..."
	mkdir -p $(script_install_dir)
	install -m 644 scripts/psurf.sh $(script_install_dir)/psurf.sh

	@echo "Adding the source line to $(zshrc) if missing..."
	@grep -qxF "source $(script_install_dir)/psurf.sh" $(zshrc) || \
	{ echo ""; echo "# Load the psurf shell function"; echo "source $(script_install_dir)/psurf.sh"; } >> $(zshrc)

	@printf "\nInstallation complete. Run 'source $(zshrc)' or restart your shell to use psurf."

## uninstall: remove the application
.PHONY: uninstall
uninstall:
	@echo "Removing $(install_path)..."
	rm -f $(install_path)
	@echo "Uninstallation completed"

## uninstall/fish: remove fish integration
.PHONY: uninstall/fish
uninstall/fish:
	@echo "Removing $(script_install_dir_for_fish)/psurf.fish..."
	rm -f $(script_install_dir_for_fish)/psurf.fish
	rm -f $(script_install_dir_for_fish)/psurf_keybindings.fish
	@printf "\nUninstallation completed. Close your shell session and open it back again."

## uninstall/bash: remove the psurf shell script for bash
.PHONY: uninstall/bash
uninstall/bash:
	@echo "Removing $(script_install_dir)/psurf.sh..."
	rm -f $(script_install_dir)/psurf.sh
	sed -i '\|# Load psurf shell function|d' $(bashrc) || true
	sed -i '\|source $(script_install_dir)/psurf.sh|d' $(bashrc) || true
	@printf "\nUninstallation completed. Close your shell session and open it back again."

## uninstall/zsh: remove the psurf shell script for zsh
.PHONY: uninstall/zsh
uninstall/zsh:
	@echo "Removing $(script_install_dir)/psurf.sh..."
	rm -f $(script_install_dir)/psurf.sh
	sed -i '\|# Load psurf shell function|d' $(zshrc) || true
	sed -i '\|source $(script_install_dir)/psurf.sh|d' $(zshrc) || true
	@printf "\nUninstallation completed. Close your shell session and open it back again."

## run: run the binary
.PHONY: run
run:
	${binary_path}${binary_extension}

## run/live: run the application with reloading on file changes
.PHONY: run/live
run/live:
	watchexec \
		--restart \
		--clear \
		--wrap-process=none \
		--watch cmd \
		--watch internal \
		--watch go.mod \
		--watch go.sum \
		--exts go -- "make build && ${binary_path}${binary_extension}"

## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'
