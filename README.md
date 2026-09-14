# pathsurfer

A tiny terminal utility for navigating through directories more quickly.

<p align="center">
  <img src="https://github.com/bnuredini/pathsurfer/blob/master/docs/imgs/screenshot1.png" alt="pathsurfer screenshot" width="80%">
</p>

---

## Features

* Directory navigation
* Fuzzy finding
* Vi-like keybindings
* Configurable settings
* Integration with bash, zsh, and fish to change your current directory

## Installing

> ⚠️ **Warning.** This is **pre-alpha software**. It's under active development
> and behavior may change at any time.

```bash
curl -sL https://github.com/bnuredini/pathsurfer/releases/latest/download/pathsurfer-linux-amd64.tar.gz | tar xz
```

## Building locally

Building and integrating pathsurfer is easy: using Make, you'll just need to run one command to
build the binary and two more commands for installing and integrating with your shell.

### Option #1: Building locally with `make`

From the project's root, run the following:

```bash
make install
```

Depending on which shell you use, run one of the following to integrate pathsurfer with your shell:

- If you use bash, run `make integrate/bash`
- If you use zsh, run `make integrate/zsh`
- If you use fish, run `make integrate/fish`

Shell integration is what allows you to change directories when quitting the program.

### Option #2: Building locally with `go`

If you don't have `make` in your system, you can build by using the Go toolchain directly:

```bash
go build ./cmd/pathsurfer
mkdir -p ~/.local/bin
install -m 644 ./bin/pathsurfer ~/.local/bin/pathsurfer
```

To integrate with bash or zsh, add this line to your `.bashrc`/`.zshrc`:

```
source <path-to-this-repo>/scripts/psurf.sh
```

To integrate with fish, run:

```bash
install -m 644 ./scripts/psurf.fish ~/.config/fish/conf.d/psurf.fish
```

## Keybindings

| Action              | Key            | Description                 |
|---------------------|----------------|-----------------------------|
| Move up             | <kbd>k</kbd>   | Move up in the file list    |
| Move down           | <kbd>j</kbd>   | Move down in the file list  |
| Go back             | <kbd>h</kbd>   | Go back one directory       |
| Go forward          | <kbd>l</kbd>   | Change into a directory     |
| Search              | <kbd>/</kbd>   | Enter search mode           |
| Toggle hidden files | <kbd>.</kbd>   | Toggle hidden files in list |
| Quit                | <kbd>q</kbd>   | Quits the program           |
| Exit search         | <kbd>ESC</kbd> | Exists out of search mode   |

## License

This project is released under the MIT license. For more information, see the 
[LICENSE](./LICENSE) file.
