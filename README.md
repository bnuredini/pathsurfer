# pathsurfer

A tiny terminal utility for fast directory navigating.

Built to work out of the box with sensible defaults, support for fuzzy finding, Vi-like
keybindings, and shell integration.

<div align="center">
  <img src="https://github.com/bnuredini/pathsurfer/blob/master/docs/imgs/screenshot1.png" alt="pathsurfer screenshot" width="80%">
  <p><i>this is <b>pre-alpha software</b>: expect active development and behavior change at any time.</i></p>
</div>

---

## Features

* Directory navigation
* Fuzzy finding
* Vi-like keybindings
* Configurable settings
* Integration with bash, zsh, and fish for fast directory navigation

<!--
## Quickstart

```bash
curl -fsSL https://github.com/bnuredini/pathsurfer/releases/latest/download/install.sh  | sh
```

## Installing

Linux:

```bash
curl -sL https://github.com/bnuredini/pathsurfer/releases/latest/download/pathsurfer-linux-amd64.tar.gz | tar xz
./pathsurfer-linux-amd64.tar.gz wizard
```

macOS:

```bash
curl -sL https://github.com/bnuredini/pathsurfer/releases/latest/download/pathsurfer-darwin-arm64.tar.gz | tar xz
./pathsurfer-darwin-arm64.tar.gz wizard
```

Windows:

```bash
curl -sL https://github.com/bnuredini/pathsurfer/releases/latest/download/pathsurfer-windows-amd64-.tar.gz | tar xz
./pathsurfer-windows-amd64.tar.gz wizard
```
-->

## Keybindings

### Default mode

| Action              | Key                              | Description                                                   |
| ------------------- | -------------------------------- | ------------------------------------------------------------- |
| Move down           | <kbd>j</kbd> / <kbd>&darr;</kbd> | Move down in the file list                                    |
| Move up             | <kbd>k</kbd> / <kbd>&uarr;</kbd>  | Move up in the file list                                      |
| Go back             | <kbd>h</kbd> / <kbd>&larr;</kbd> | Go back one directory                                         |
| Go forward          | <kbd>l</kbd> / <kbd>&rarr;</kbd> | Change into a directory                                       |
| Search              | <kbd>/</kbd>                     | Enter search mode                                             |
| Toggle hidden files | <kbd>.</kbd>                     | Toggle hidden files in list                                   |
| Quit                | <kbd>q</kbd>                     | Quits the program                                             |
| Record bookamark    | <kbd>m</kbd>                     | Prompts for a key to be associated with the current directory |
| Go to bookamark     | <kbd>'</kbd>                     | Prompts for a bookmarked key                                  |
| Go to top           | <kbd>gg</kbd>                    | Move the cursor at the top of the list                        |
| Go to bottom        | <kbd>G</kbd>                     | Move the cursor at the bottom of the list                     |
| Big move up         | <kbd>CTRL+u</kbd>                | Move the cursor 22 rows up                                    |
| Big move down       | <kbd>CTRL+d</kbd>                | Move the cursor 22 rows down                                  |

### Search mode

| Action              | Key                  | Description                                                          |
| ------------------- | -------------------- | -------------------------------------------------------------        |
| Confirm search      | <kbd>Enter</kbd>     | Escape out of search mode and view the results of the search pattern |
| Enter top directory | <kbd>TAB</kbd>       | Enter in to the directory showing up as the top search result        |
| Go back             | <kbd>Shift+TAB</kbd> | Go back one directory                                                |
| Escape              | <kbd>ESC</kbd>       | Escape out of search mode                                            |

## Building locally

Building and integrating pathsurfer is easy: using Make, you'll just need to run one command to
build the binary and one command to integrate with your shell.

### Option #1: Building locally with `make`

To build and install, run:

```bash
make install
```

(If you just need to build the binary, run `make build`.)

Depending on which shell you use, run one of the following to integrate pathsurfer with your shell:

| Shell | Command               |
| ----- | --------------------- |
| bash  | `make integrate/bash` |
| zsh   | `make integrate/zsh`  |
| fish  | `make integrate/fish` |

Shell integration is what allows you to change directories when quitting the program.

### Option #2: Building locally with `go`

If you don't have `make` in your system, you can build by using the Go toolchain directly:

```bash
go build ./cmd/pathsurfer
mkdir -p ~/.local/bin
install -m 744 ./build/pathsurfer ~/.local/bin/pathsurfer
```

To integrate with bash or zsh, add this line to your `.bashrc`/`.zshrc`:

```
source <path-to-this-repo>/scripts/psurf.sh
```

To integrate with fish, run:

```bash
install -m 644 ./scripts/psurf.fish ~/.config/fish/conf.d/psurf.fish
```

<details>
<summary>Cleaning up</summary>

Run:

```
make uninstall
```

and `make disintegrate/bash`, `make disintegrate/zsh`, or `make disintegrate/fish` depending on
which shell integration you installed.
</details>

## Contributions

Pull requests aren't reviewed due to time constraints, but feel free to open issues though.

## License

The source code is released under the MIT license.

---

_Everything hand-written: code, docs, and typos._
