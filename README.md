# pathsurfer

A tiny terminal utility for navigating through directiories more quickly.

* [Screenshots](#screenshots)
* [Features](#features)
* [Platforms](#platforms)
* [Installing](#installing)
* [Keybindings](#keybindings)
* [License](#license)

## Screenshots

![screenshot1.png](./docs/imgs/screenshot1.png)

## Features

* Directory navigation
* Fuzzy finding
* Vi-like keybindings
* Configurable settings
* Integration with bash, zsh, and fish

## Platforms

* Linux
* macOS
* Planned: Windows

## Installing

### Option #1: Building locally with `make`

Clone the repository:

```bash
git clone https://github.com/bnuredini/pathsurfer
```

and run:

```bash
make build
sudo make install
```

The binary should be installed at `/usr/bin/pathsurfer`.

Finally, make sure add the `psurf` function to your shell. Depending on which shell you're using,
you should run one of the following commands:

```bash
make install/fish
```

```bash
make install/bash
```

### Option #2: Building locally with `go`

If you don't have `make` installed in your system, you can build this project by using the Go
toolchain directly.

Clone the repository:

```bash
git clone https://github.com/bnuredini/pathsurfer
```

and run:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o=./bin/pathsurfer ./cmd/pathsurfer
mkdir -p /usr/bin
sudo install -m 644 ./bin/pathsurfer /usr/bin/pathsurfer
```

If you use bash, add this line to your `.bashrc`:

```bash
source <path-to-this-repo>/scripts/psurf.sh
```

If you use fish, run:

```bash
install -m 644 <path-to-this-repo>/scripts/psurf.fish ~/.config/fish/conf.d/psurf.fish
```

## Keybindings

| Action              | Key | Description                 |
|---------------------|-----|-----------------------------|
| Move up             | k   | Move up in the file list    |
| Move down           | j   | Move down in the file list  |
| Go back             | h   | Go back one directory       |
| Go forward          | l   | Change into a directory     |
| Search              | /   | Enter search mode           |
| Toggle hidden files | .   | Toggle hidden files in list |
| Quit                | q   | Quits the program           |

## License

This project is released under the MIT license. For more information, see the 
[LICENSE](./LICENSE) file.

