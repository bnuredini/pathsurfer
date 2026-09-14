# To-do list

* Bugs
    - Tab is buggy
* Improvements
    - Add a "Searched" line in the search bar
    - Respect position history when using Shift-TAB
    - When in search mode, ignore subsequent presses of the "/" key
* Features
  - Bookmarks
  - Git info for repositories
  - Useful directory info (size, file count, directory count)
  - Colorthemes
  - Text file preview
  - Popup for keybinding hints listing possible continuations after hitting a key
  - Keybindings for outputting selected files/directories to stdout
  - Keybindings for copying the current directory path to the clipboard
* Add support for Windows
* Integrating
  - Write install script so users can get started quickly using the `curl <url> | sh` pattern. This
    script should automatically check which shells are installed.
  - Add subcommands: `psurf integrate`, `psurf check-integration`, `psurf wizard` (where to install,
    for which shell to integrate, should the shell keybindings be set)
* Add a section to README comparing this with nnn, ranger, lf, yazi, & fzf
