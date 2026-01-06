main_package_path = ./cmd/pathsurfer

binary_name = pathsurfer
binary_path = ./bin/${binary_name}
binary_ext =
ifeq ($(GOOS),windows)
	binary_ext = .exe
endif

curr_time = $(shell date --iso-8601=seconds)
git_description = $(shell git describe --always --dirty)
linker_flags = '-s -X github.com/bnuredini/pathsurfer/internal/conf.buildTime=${curr_time} -X github.com/bnuredini/pathsurfer/internal/conf.version=${git_description}'

install_path = /usr/bin/pathsurfer
script_install_dir_for_fish = $(HOME)/.config/fish/conf.d
script_install_dir = $(HOME)/.local/share/pathsurfer/functions
bashrc = $(HOME)/.bashrc
zshrc = $(HOME)/.zshrc

## build: build the application
.PHONY: build
build:
	CGO_ENABLED=0 go build -ldflags=${linker_flags} -o=${binary_path}${binary_ext} ${main_package_path}

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
	@echo -e "\nInstallation complete. Run 'source $(script_install_dir_for_fish)/psurf.fish' or restart your shell to use psurf."

## install/bash: install the binary stored in <project-path>/bin/ for bash
.PHONY: install/bash
install/bash:
	@echo "Installing the psurf script to $(script_install_dir)..."
	mkdir -p $(script_install_dir)
	install -m 644 scripts/psurf.sh $(script_install_dir)/psurf.sh

	@echo "Adding the source line to $(bashrc) if missing..."
	@grep -qxF "source $(script_install_dir)/psurf.sh" $(bashrc) || \
	{ echo ""; echo "# Load the psurf shell function"; echo "source $(script_install_dir)/psurf.sh"; } >> $(bashrc)

	@echo -e "\nInstallation complete. Run 'source $(bashrc)' or restart your shell to use psurf."

## install/zsh: install the binary stored in <project-path>/bin/ for zsh
.PHONY: install/zsh
install/zsh:
	@echo "Installing the psurf script to $(script_install_dir)..."
	mkdir -p $(script_install_dir)
	install -m 644 scripts/psurf.sh $(script_install_dir)/psurf.sh

	@echo "Adding the source line to $(zshrc) if missing..."
	@grep -qxF "source $(script_install_dir)/psurf.sh" $(zshrc) || \
	{ echo ""; echo "# Load the psurf shell function"; echo "source $(script_install_dir)/psurf.sh"; } >> $(zshrc)

	@echo -e "\nInstallation complete. Run 'source $(zshrc)' or restart your shell to use psurf."

## uninstall: remove the application
.PHONY: uninstall
uninstall:
	@echo "Removing $(install_path)..."
	rm -f $(install_path)
	@echo "Uninstallation completed"

## uninstall/fish: install the binary stored in <project-path>/bin/ for fish
.PHONY: uninstall/fish
uninstall/fish:
	@echo "Removing $(script_install_dir_for_fish)/psurf.fish..."
	rm -f $(script_install_dir_for_fish)/psurf.fish
	@echo -e "\nUninstallation completed"

## uninstall/bash: remove the psurf shell script for bash
.PHONY: uninstall/bash
uninstall/bash:
	@echo "Removing $(script_install_dir)/psurf.sh..."
	rm -f $(script_install_dir)/psurf.sh
	sed -i '\|# Load psurf shell function|d' $(bashrc) || true
	sed -i '\|source $(script_install_dir)/psurf.sh|d' $(bashrc) || true
	@echo -e "\nUninstallation completed"

## uninstall/zsh: remove the psurf shell script for zsh
.PHONY: uninstall/zsh
uninstall/zsh:
	@echo "Removing $(script_install_dir)/psurf.sh..."
	rm -f $(script_install_dir)/psurf.sh
	sed -i '\|# Load psurf shell function|d' $(zshrc) || true
	sed -i '\|source $(script_install_dir)/psurf.sh|d' $(zshrc) || true
	@echo -e "\nUninstallation completed"

## run: run the binary
.PHONY: run
run:
	${binary_path}

## run/live: run the application with reloading on file changes
.PHONY: run/live
run/live:
	go run github.com/cosmtrek/air@v1.52.0 \
		--build.cmd "make build" \
		--build.bin "${binary_path}" \
		--build.delay "100" \
		--build.exclude_dir "" \
		--misc.clean_on_exit "true"
