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

## Keybindings

| Action              | Key               | Description                                                   |
| ------------------- | --------------    | ------------------------------------------------------------- |
| Move up             | <kbd>k</kbd>      | Move up in the file list                                      |
| Move down           | <kbd>j</kbd>      | Move down in the file list                                    |
| Go back             | <kbd>h</kbd>      | Go back one directory                                         |
| Go forward          | <kbd>l</kbd>      | Change into a directory                                       |
| Search              | <kbd>/</kbd>      | Enter search mode                                             |
| Toggle hidden files | <kbd>.</kbd>      | Toggle hidden files in list                                   |
| Quit                | <kbd>q</kbd>      | Quits the program                                             |
| Exit search         | <kbd>ESC</kbd>    | Exists out of search mode                                     |
| Record bookamark    | <kbd>m</kbd>      | Prompts for a key to be associated with the current directory |
| Go to bookamark     | <kbd>'</kbd>      | Prompts for a bookmarked key                                  |
| Go to top           | <kbd>gg</kbd>     | Move the cursor at the top of the list                        |
| Big move up         | <kbd>CTRL+u</kbd> | Move the cursor 22 rows up                                    |
| Big move down       | <kbd>CTRL+d</kbd> | Move the cursor 22 rows down                                  |

## Search mode

| Action              | Key                  | Description                                                   |
| ------------------- | --------------       | ------------------------------------------------------------- |
| Enter directory     | <kbd>TAB</kbd>       | Enter in to the directory showing up as the top search result |
| Go back directory   | <kbd>Shift+TAB</kbd> | Go back one directory                                         |
| Escape              | <kbd>ESC</kbd>       | Escape out of search mode                                     |

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
