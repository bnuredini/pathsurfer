#!/bin/bash

psurf() {
    local app_path="/usr/bin/pathsurfer"
    local target_dir

    if [ ! -x "$app_path" ]; then
        echo "error: pathsurfer not found or isn't executable at $app_path"
        return 1
    fi

    target_dir=$("$app_path")

    if [ -n "$target_dir" ] && [ -d "$target_dir" ]; then
        cd "$target_dir"
    fi
}
