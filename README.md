# pathsurfer

A tiny terminal utility for navigating through directories more quickly.

<p align="center">
  <img src="https://github.com/bnuredini/pathsurfer/blob/master/docs/imgs/screenshot1.png" alt="pathsurfer screenshot" width="80%">
</p>

* [Screenshots](#screenshots)
* [Features](#features)
* [Platforms](#platforms)
* [Installing](#installing)
* [Building locally](#building-locally)
* [Keybindings](#keybindings)
* [License](#license)

## Features

> ⚠️ **WARNING**
>
> This is **pre-alpha software**. It's under active development and behavior may change at any time.

* Directory navigation
* Fuzzy finding
* Vi-like keybindings
* Configurable settings
* Integration with bash, zsh, and fish to change your current directory

## Platforms

* Linux
* macOS
* Planned: Windows

## Installing

```bash
curl -sL https://github.com/bnuredini/pathsurfer/releases/latest/download/pathsurfer-linux-amd64.tar.gz | tar xz
```

## Building locally

### Option #1: Building locally with `make`

From the project's root, run the following:

```bash
make build
sudo make install
```

Depending on which shell you use, you might want to run one of the following commands to integrate
pathsurfer with your shell.

* If you use bash, run `make integrate/bash`
* If you use zsh, run `make integrate/zsh`
* If you use fish, run `make integrate/fish`

### Option #2: Building locally with `go`

If you don't have `make` installed in your system, you can build this project by using the Go
toolchain directly.

From project root, run the following:

```bash
go build ./cmd/pathsurfer
sudo install -m 644 ./pathsurfer /usr/bin/pathsurfer
```

If you use bash or zsh, add this line to your `.bashrc`/`.zshrc`:

```bash
source <path-to-this-repo>/scripts/psurf.sh
```

If you use fish, run:

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
