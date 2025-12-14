# pathsurfer

A tiny terminal utility for navigating through directiories more quickly.

* [Screenshots](#screenshots)
* [Installing](#installing)
* [License](#license)

## Screenshots

![screenshot1.png](./docs/imgs/screenshot1.png)

## Installing

### Building locally with `make`

Clone the repository:

```
git clone https://github.com/bnuredini/pathsurfer
```

and run:

```
make build
sudo make install
```

The binary should be installed at `/usr/bin/pathsurfer`.

Finally, make sure add the `psurf` function to your shell. Depending on which shell you're using,
you should run one of the following commands:

```
make install/fish
```

```
make install/bash
```

### Building locally with the `go` toolchain

If you don't have `make` installed in your system, you can build this project by using the Go
toolchain directly.

Clone the repository:

```
git clone https://github.com/bnuredini/pathsurfer
```

and run:

```
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o=./bin/pathsurfer ./cmd/pathsurfer
mkdir -p /usr/bin
sudo install -m 644 ./bin/pathsurfer /usr/bin/pathsurfer
```

To include the `psurf.sh` script into your Bash installation, make sure to add this line your `.bashrc`:

```
source <path-to-this-repo>/scripts/psurf.sh
```

To include the `psurf.fish` script into your Fish installation, run:

```
install -m 644 <path-to-this-repo>/scripts/psurf.fish ~/.config/fish/conf.d/psurf.fish
```

## License

This project is released under the MIT license. See the [LICENSE](./LICENSE) file for more 
information.
