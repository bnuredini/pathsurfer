# To-do list

* Bugs
    - Term `ns` matches `clones` ahead of `ns`
* Improvements
    - When in search mode, ignore subsequent presses of the "/" key
* Features
    - Clipboard support for Linux
    - Git info for repositories
    - Useful directory info (size, file count, directory count)
    - Colorthemes
    - Text file preview
    - Popup for keybinding hints listing possible continuations after hitting a key
    - Keybindings for outputting selected files/directories to stdout
    - Upon encountering a bookmark conflict, prompt the user if they want to do an override
* Add support for Windows
    - Install commands
    - Clipboard support
* Integrating
    - Write install script so users can get started quickly using the `curl <url> | sh` pattern. This
      script should automatically check which shells are installed.
    - Add subcommands: `psurf integrate`, `psurf check-integration`, `psurf wizard` (where to install,
      for which shell to integrate, should the shell keybindings be set)
* Add a section to README comparing this with nnn, ranger, lf, yazi, & fzf
